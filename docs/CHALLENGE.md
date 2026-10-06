# Container Runtime - Implementation Plan

## Goal

Build a minimalistic, Linux-compatible container runtime in Go (~500 lines) that:
- Isolates a process using Linux namespaces
- Provides a minimal filesystem root via chroot/pivot_root
- Intercepts `open`, `read`, and `write` syscalls via ptrace and logs them to stdout
- Runs entirely inside Docker for a controlled Linux environment

Each step produces a **runnable binary** with a concrete expected output, so progress is always visible.

---

## Repository Layout

The project grows incrementally — each step adds files to their designated place. Here is the full layout you will end up with by Step 7:

```
container/
├── main.go                        # entry point; routes "run" and "child" subcommands
├── go.mod
├── Makefile
├── Dockerfile
│
├── cmd/
│   └── container/
│       └── main.go                # Step 2: parent/child argument dispatch
│
└── internal/
    ├── namespace/
    │   └── namespace.go           # Step 2: clone flags, Sethostname
    ├── rootfs/
    │   └── rootfs.go              # Step 3: chroot/pivot_root, /proc mount
    ├── exec/
    │   └── exec.go                # Step 4: syscall.Exec into the payload command
    ├── tracer/
    │   ├── tracer.go              # Step 5: ptrace loop, wait4
    │   └── decode.go              # Step 6: register inspection, open/read/write decoding
    └── cgroup/
        └── cgroup.go              # Step 7 (optional): cgroup v2 resource limits
```

**Why `internal/`?** Go's `internal` package rule prevents code outside this module from importing these packages. For a study project it serves as a clear signal: everything under `internal/` is implementation detail, not a public API.

**Why `cmd/container/`?** The re-exec pattern means the binary spawns a copy of itself and passes `"child"` as an argument to take on the child role inside the namespaces. Separating this dispatch into `cmd/container/main.go` keeps the root `main.go` focused on the entry point and makes the two roles (parent supervisor vs. child inside namespaces) easy to find.

**One binary thOnroughout.** Despite the directory structure, `go build` produces a single binary. The `cmd/` and `internal/` split is organisational, not a multiple-binary layout.

---

## Step 1 - Project Scaffold + Dockerfile

### Goal
Set up the Go module, directory layout, and Docker build pipeline. By the end, you can build and run a "hello from inside Docker" binary.

### What to build
- `go.mod` with module `github.com/whatever...`
- `main.go` with a stub `main()` that prints a version string
- `Dockerfile` (multi-stage): builder image compiles a static binary; runner image is minimal Linux
- `Makefile` with `build` and `run` targets

### Technical knowledge needed
- Go modules (`go mod init`, `go build`)
- Docker multi-stage builds (`FROM golang:... AS builder`, `FROM scratch` or `FROM alpine`)
- Static linking in Go: `CGO_ENABLED=0 GOOS=linux go build`
- Docker `--privileged` flag (required for namespaces and ptrace in later steps)

### Expected output
```
$ make run
[container-runtime] v0.1.0 - ready
```

---

## Step 2 - PID and UTS Namespace Isolation

### Goal
Launch a child process inside new PID and UTS namespaces so it sees itself as PID 1 and has its own hostname. The parent process acts as the supervisor.

### How the two roles work

There is only one binary. It re-executes itself and uses the first CLI argument to decide which role to play:

```
$ container run   →  parent role: spawns a copy of itself as "child"
$ container child →  child role:  runs inside the namespaces
```

The parent uses `exec.Command("/proc/self/exe", "child")` to spawn itself. `/proc/self/exe` is a Linux symlink that always points to the currently running binary, so no hardcoded path is needed.

`cmd/container/main.go` reads `os.Args[1]` and routes to the correct function:

```go
switch os.Args[1] {
case "run":
    runParent()
case "child":
    runChild()
}
```

### What to build

**`cmd/container/main.go`** — argument dispatch as shown above, plus `runParent()` and `runChild()` stubs.

**`internal/namespace/namespace.go`** — a `Namespace` function that returns a configured `*exec.Cmd` pointing at `/proc/self/exe child` with the namespace flags set:

```go
func Namespace() *exec.Cmd {
    cmd := exec.Command("/proc/self/exe", "child")
    cmd.Stdin  = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    cmd.SysProcAttr = &syscall.SysProcAttr{
        Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS,
    }
    return cmd
}
```

The parent calls `namespace.Namespace()`, then `cmd.Run()`.

**Inside `runChild()`** — the child is already inside the new namespaces when it starts. It just needs to set its hostname and print its PID:

```go
func runChild() {
    syscall.Sethostname([]byte("mycontainer"))
    fmt.Printf("[child]  hostname: %s\n", getHostname())
    fmt.Printf("[child]  PID inside namespace: %d\n", os.Getpid())
}
```

No `ForcExec` or any other special flag is needed here. The namespace isolation is already in place before `runChild()` runs a single line of code — the kernel applied it during the `clone` call that spawned the child process.

### Technical knowledge needed
- Linux namespaces: what PID and UTS namespaces isolate
- `syscall.SysProcAttr.Cloneflags` — how `exec.Cmd` passes these to `clone(2)` under the hood
- `/proc/self/exe` — the Linux symlink to the running binary; used to re-exec without a hardcoded path
- `os.Getpid()` returns 1 inside the child because it is the first process in the new PID namespace
- Why the parent sees a different PID for the child than the child sees for itself

### Expected output
```
$ make run
[parent] supervisor PID: 4821
[child]  hostname: mycontainer
[child]  PID inside namespace: 1
```

---

## Step 3 - Mount Namespace + Rootfs Setup

### Goal
Give the container its own filesystem root so it sees an isolated directory tree instead of the host's `/`. The child process will `chroot` into a minimal directory you build at runtime.

### Where the rootfs comes from

No download needed. The outer Docker image is Alpine, and Alpine ships with busybox — a single static binary at `/bin/busybox` that provides `sh`, `ls`, `cat`, `ps`, and dozens of other tools via symlinks.

The rootfs setup creates a temporary directory (e.g. `/tmp/container-root`) and populates it by copying or bind-mounting just enough from the outer Alpine filesystem for a shell to work:

```
/tmp/container-root/
├── bin/
│   ├── busybox        ← copied from /bin/busybox
│   ├── sh             ← symlink → busybox
│   ├── ls             ← symlink → busybox
│   └── ...            ← other symlinks busybox provides
├── proc/              ← empty dir, /proc will be mounted here
└── tmp/               ← empty dir
```

`internal/rootfs/rootfs.go` is responsible for building this tree and calling `syscall.Chroot` to make it the child's root.

### What to build

**`internal/rootfs/rootfs.go`** — a `Setup(rootfsPath string)` function that:
1. Creates the directory tree above under `rootfsPath`
2. Copies `/bin/busybox` into `rootfsPath/bin/busybox`
3. Creates symlinks for the tools you want (`sh`, `ls`, `ps`, `cat`, `echo`)
4. Calls `syscall.Chroot(rootfsPath)` followed by `os.Chdir("/")`
5. Mounts `/proc` inside the new root: `syscall.Mount("proc", "/proc", "proc", 0, "")`

**`internal/namespace/namespace.go`** — add `CLONE_NEWNS` to the clone flags so the child gets its own mount namespace (required before remounting `/proc`):

```go
Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWNS,
```

**`cmd/container/main.go`** — `runChild()` calls `rootfs.Setup("/tmp/container-root")` before anything else.

### Why chroot and not pivot_root?

`pivot_root(2)` is the production approach — it fully replaces the root mount and makes the old root inaccessible, which is more secure. However it requires the new root to be a mount point itself, adding extra steps. `chroot(2)` is simpler: it just changes what `/` resolves to for the calling process. For a study project, `chroot` is the right starting point. The "Technical knowledge needed" section below explains the difference.

### Technical knowledge needed
- Mount namespace (`CLONE_NEWNS`): without it, mounting `/proc` inside the child would affect the host's `/proc` too — namespaces make the mount table private
- `chroot(2)`: changes the root directory for the calling process and all its children; always follow with `os.Chdir("/")` to move the working directory inside the new root
- `pivot_root(2)` vs `chroot(2)`: pivot_root replaces the root mount entirely (more secure, used by runc); chroot only changes path resolution (simpler, sufficient here)
- Mounting `/proc`: the proc filesystem is virtual — it must be explicitly mounted even inside a new root; without it, `ps`, `/proc/self/exe`, and PID inspection won't work
- Busybox symlinks: busybox reads `argv[0]` to decide which tool to run; a symlink named `ls` pointing to `busybox` makes busybox behave as `ls`

### Expected output
```
$ make run
[parent] supervisor PID: 4821
[child]  rootfs ready: /tmp/container-root
[child]  hostname: mycontainer  PID: 1
/ # ls /
bin   proc  tmp
/ # ps
PID   USER     COMMAND
1     root     sh
/ # exit
[parent] container exited: exit status 0
```

---

## Step 4 - Process Execution Inside the Container

### Goal
Instead of a hardcoded shell, accept a command from the host CLI and execute it inside the container. This is the `container run <cmd>` primitive.

### What to build
- CLI argument parsing: `os.Args` to extract the command to run (e.g., `./container run /bin/sh`)
- `internal/exec/exec.go`: `syscall.Exec` inside the child to replace the stub process with the target command
- Stdio passthrough so the user can interact with the process

### Technical knowledge needed
- `syscall.Exec` vs `exec.Command`: `Exec` replaces the current process image (no fork); `exec.Command` spawns a child
- How Go's `exec.Cmd` propagates stdin/stdout/stderr via `Cmd.Stdin`, `Cmd.Stdout`, `Cmd.Stderr`
- Why the child must call `exec` *after* namespace and rootfs setup, not before
- The two-process pattern in container runtimes: the parent sets up namespaces, the child calls `exec` into the payload

### Expected output
```
$ make run CMD="/bin/sh"
[parent] launching: /bin/sh
/ # echo hello from container
hello from container
/ # hostname
mycontainer
/ # exit
[parent] container exited: exit status 0
```

---

## Step 5 - ptrace Harness: Attach and Intercept Syscalls

### Goal
Attach a ptrace tracer to the container process. Stop at every syscall entry/exit and print the syscall number. This is the foundation for syscall monitoring - not yet decoded, just raw numbers.

### What to build
- `internal/tracer/tracer.go`: starts the child with `PTRACE_TRACEME`, then the parent loops with `PTRACE_SYSCALL` + `wait4`
- Print `SYSCALL ENTER: <number>` and `SYSCALL EXIT: <number>` for every syscall

### Technical knowledge needed
- ptrace fundamentals: `PTRACE_TRACEME` (child opts in), `PTRACE_SYSCALL` (resume until next syscall boundary), `wait4` (parent blocks until child stops)
- Syscall-stop vs signal-stop: how to distinguish them in `WaitStatus`
- The two-stop model: ptrace stops the tracee **twice** per syscall - entry (before kernel executes) and exit (after)
- `golang.org/x/sys/unix` package: `unix.PtraceGetRegs`, `unix.PtraceSyscall`, `unix.Wait4`
- Why ptrace requires `CAP_SYS_PTRACE` (satisfied by `--privileged` in Docker)

### Expected output
```
$ make run CMD="/bin/echo hi"
SYSCALL ENTER: 59   (execve)
SYSCALL EXIT:  59
SYSCALL ENTER: 12   (brk)
SYSCALL EXIT:  12
...
hi
SYSCALL ENTER: 60   (exit_group)
[parent] container exited: exit status 0
```

---

## Step 6 - Syscall Argument Inspection: open / read / write

### Goal
Decode the registers for `open` (or `openat`), `read`, and `write` syscalls and print the filename, file descriptor, and byte count. This is the core monitoring feature of the runtime.

### What to build
- `internal/tracer/decode.go`: a `decodeSyscall(pid, regs)` function that switches on `regs.Orig_rax`
- For `open`/`openat`: read the filename string from the tracee's memory via `unix.PtracePeekData`
- For `read`/`write`: extract fd (rdi), buffer address (rsi), count (rdx)
- Structured log line: `[OPEN]  /etc/passwd  flags=O_RDONLY` / `[WRITE] fd=1  bytes=12`

### Technical knowledge needed
- x86-64 Linux syscall ABI: `rax`=syscall number, `rdi`=arg1, `rsi`=arg2, `rdx`=arg3
- `unix.PtracePeekData`: reads one word at a time from tracee memory - need a loop to read a C string
- Syscall numbers on x86-64: `open`=2, `openat`=257, `read`=0, `write`=1
- Entry vs exit distinction: filename is valid at entry; return value (fd) is in `rax` at exit
- `O_RDONLY`, `O_WRONLY`, `O_RDWR` flag decoding

### Expected output
```
$ make run CMD="/bin/cat /etc/hostname"
[OPEN]   path=/etc/hostname  flags=O_RDONLY
[READ]   fd=3  bytes=12
[WRITE]  fd=1  bytes=12
mycontainer
[parent] container exited: exit status 0
```

---

## Step 7 - cgroup v2 Resource Limits (optional)

### Goal
Apply memory and CPU limits to the container process using cgroup v2, so resource exhaustion cannot affect the host. This step is optional - cut it if the line budget is tight.

### What to build
- `internal/cgroup/cgroup.go`: creates a cgroup under `/sys/fs/cgroup/<name>/`, writes `memory.max` and `cpu.max`, adds the child PID to `cgroup.procs`

### Technical knowledge needed
- cgroup v2 hierarchy: a single unified tree under `/sys/fs/cgroup`; resources set by writing to pseudo-files
- `memory.max`: hard limit in bytes; `cpu.max`: `<quota> <period>` in microseconds
- cgroup delegation: why Docker containers need `--cgroup-parent` or a writable cgroup mount to create sub-cgroups
- Cleanup: remove the cgroup directory after the child exits (must be empty first)

### Expected output
```
$ make run CMD="/bin/sh" MEM=64m CPU=0.5
[parent] cgroup: memory.max=67108864  cpu.max=50000 100000
[child]  hostname: mycontainer  PID: 1
/ # exit
[parent] cgroup cleaned up
[parent] container exited: exit status 0
```

---

## Summary

| Step | Feature | Runnable artifact |
|------|---------|------------------|
| 1 | Scaffold + Docker | Prints version string |
| 2 | PID + UTS namespaces | Child reports PID=1, custom hostname |
| 3 | Mount namespace + rootfs | Isolated `ls /` and `ps` |
| 4 | CLI command execution | Interactive shell inside container |
| 5 | ptrace raw syscall log | Raw syscall numbers printed |
| 6 | open/read/write decoding | Structured syscall log with filenames |
| 7 | cgroup v2 limits | Resource-bounded container |
