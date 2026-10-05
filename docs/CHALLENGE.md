# Container Runtime - Implementation Plan

## Goal

Build a minimalistic, Linux-compatible container runtime in Go (~500 lines) that:
- Isolates a process using Linux namespaces
- Provides a minimal filesystem root via chroot/pivot_root
- Intercepts `open`, `read`, and `write` syscalls via ptrace and logs them to stdout
- Runs entirely inside Docker for a controlled Linux environment

Each step produces a **runnable binary** with a concrete expected output, so progress is always visible.

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

### What to build
- `cmd/container/main.go`: entry point that forks itself with `clone` flags
- `internal/namespace/namespace.go`: wraps `syscall.SysProcAttr` with `CLONE_NEWPID | CLONE_NEWUTS`
- Child process sets its hostname via `syscall.Sethostname` and prints its PID

### Technical knowledge needed
- Linux namespaces: what PID and UTS namespaces isolate
- `syscall.SysProcAttr.Cloneflags` in Go - how `exec.Cmd` uses it to pass clone flags to `clone(2)`
- `os.Getpid()` inside the child returns 1 because it's the first process in the new PID namespace
- Why the parent's PID and the child's PID differ

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
Give the container its own filesystem root using a minimal rootfs (busybox static binary) and `pivot_root` or `chroot`. The child process now lives in an isolated directory tree.

### What to build
- `internal/rootfs/rootfs.go`: downloads or unpacks a minimal rootfs (busybox tarball) into a temp directory; calls `syscall.Chroot` or `pivot_root`
- Add `CLONE_NEWNS` to the clone flags
- Mount `/proc` inside the new root so tools like `ps` work

### Technical knowledge needed
- Mount namespace: why you need `CLONE_NEWNS` before remounting `/proc`
- `pivot_root(2)` vs `chroot(2)`: pivot_root is the container-runtime way (fully replaces the root); chroot is simpler but leaves the old root accessible
- Bind mounts: `syscall.Mount` with `MS_BIND`
- `/proc` filesystem: why `ps` and `/proc/self` need it mounted
- Busybox as a minimal rootfs: one binary with symlinks for `sh`, `ls`, `cat`, etc.

### Expected output
```
$ make run
[parent] supervisor PID: 4821
[child]  rootfs: /tmp/container-root-382910
[child]  hostname: mycontainer  PID: 1
[child]  $ ls /
bin  dev  etc  proc  tmp  usr
[child]  $ ps
PID   USER     COMMAND
1     root     sh
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
