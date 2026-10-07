# Managing Processes - Library and Kernel Reference

This document introduces the Go packages and Linux kernel interfaces you will use to build the container runtime. Read it before starting the implementation steps. Each section describes what the library does, which parts you will use, and where to find the official documentation.

It is important to notice that if you are programming in other environment than Linux, the packages may have different structures and function signatures than the ones described here. 

Here we are working exclusively with a dockerized linux application.

---

## Table of Contents

- [The Basics]
- [The Go `syscall` Package](#the-go-syscall-package)
- [The `golang.org/x/sys/unix` Package](#the-golangorgxsysunix-package)
- [The `os/exec` Package](#the-osexec-package)
- [Linux `ptrace(2)`](#linux-ptrace2)
- [Linux Namespaces](#linux-namespaces)
- [Linux `clone(2)`](#linux-clone2)
- [cgroups v2](#cgroups-v2)

---

## The Basics

### Hostname

In this context, hostname is not related to the network machine name or DNS resolution. Instead, it is a label string used by the kernel to identify a **Unix Timeshare System (UTS)**  Namespace. Tools like ps, shell prompt, log formaters will read it making it easyer to identify the application.

Its a large subject, so if you want to know more, check [the Wikipedia Linux namespaces](https://en.wikipedia.org/wiki/Linux_namespaces).

So now is a bit clearer that:

```go
    cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWNS,
    }
```

- CLONE_NEWPI : will provide a new PID for the process
- CLONE_NEWUTS : will assing a new UTS Namespace label for the process

---

## The Go `syscall` Package

**Official documentation:** https://pkg.go.dev/syscall

The `syscall` package exposes the raw operating system interface, the layer just above the kernel. It is part of the Go standard library but is **frozen**: no new functionality is added. For new systems code, prefer `golang.org/x/sys/unix` (covered next), which is actively maintained and has broader coverage. You will still use `syscall` for the types it defines, particularly `SysProcAttr`, because `os/exec` references those types directly.

### `SysProcAttr`

This struct is the bridge between Go's process-spawning APIs and the Linux kernel's process-creation primitives. You attach it to a command before starting it, and the Go runtime passes its fields to `clone(2)` on your behalf.

On Linux, this structure looks like:

```go
type SysProcAttr struct {
    Chroot      string      // chroot into this path before exec
    Ptrace      bool        // start child under ptrace
    Cloneflags  uintptr     // clone(2) flags - namespace bits go here
    // ... other fields
}
```

The field you will use most is `Cloneflags`. It accepts a bitmask of `CLONE_NEW*` constants that tell the kernel which namespaces to create for the child process:

```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWNS,
    Ptrace:     true,
}
```

Setting `Ptrace: true` is equivalent to the child calling `PTRACE_TRACEME` - it tells the kernel to stop the child immediately after `exec` and signal the parent, which is exactly how you attach your syscall tracer.

### Key functions

`syscall.Sethostname(p []byte)` - sets the hostname of the current UTS namespace. Requires `CAP_SYS_ADMIN` (or ownership of the UTS namespace). Note it takes a `[]byte`, not a `string`:

```go
syscall.Sethostname([]byte("mycontainer"))
```

`syscall.Chroot(path string)` - changes the filesystem root for the calling process. After this call, `/` resolves to `path`. Always follow it with `os.Chdir("/")` to move the working directory inside the new root:

```go
syscall.Chroot("/tmp/container-root")
os.Chdir("/")
```

`syscall.Mount(source, target, fstype string, flags uintptr, data string)` - mounts a filesystem. You will use this to mount `/proc` inside the container so tools like `ps` work:

```go
syscall.Mount("proc", "/proc", "proc", 0, "")
```

`syscall.Exec(argv0 string, argv []string, envv []string)` - replaces the current process image with a new one (wraps `execve(2)`). It does not return on success. Use this inside the child after namespaces and rootfs are set up:

```go
syscall.Exec("/bin/sh", []string{"/bin/sh"}, os.Environ())
```

---

## The `golang.org/x/sys/unix` Package

**Official documentation:** https://pkg.go.dev/golang.org/x/sys/unix

**Install:** `go get golang.org/x/sys/unix`

This is the package you will use for everything ptrace-related. It provides direct, architecture-aware wrappers around Linux syscalls that the standard library either does not expose or exposes poorly. Unlike `syscall`, it is actively maintained and tracks new kernel interfaces.

### ptrace functions

`unix.PtraceSyscall(pid int, signal int)` - resumes a stopped tracee with the `PTRACE_SYSCALL` request. The tracee runs until the next syscall boundary (entry or exit), then stops again and sends `SIGTRAP` to the tracer. Pass `signal = 0` to resume without delivering any signal to the tracee:

```go
unix.PtraceSyscall(pid, 0)
```

`unix.PtraceGetRegs(pid int, regsout *unix.PtraceRegs)` - reads the current CPU registers of the tracee into `regsout`. Call this after the tracee has stopped at a syscall boundary to inspect which syscall is being made and what its arguments are.

`unix.PtracePeekData(pid int, addr uintptr, out []byte)` - reads bytes from an arbitrary address in the tracee's memory. You will use this to read filename strings passed to `open` and `openat`, since the register holds a pointer into the tracee's address space, not the string itself:

```go
// Read a null-terminated C string from the tracee
func readString(pid int, addr uintptr) string {
    var buf [256]byte
    unix.PtracePeekData(pid, addr, buf[:])
    return string(buf[:bytes.IndexByte(buf[:], 0)])
}
```

`unix.Wait4(pid int, wstatus *unix.WaitStatus, options int, rusage *unix.Rusage)` - blocks until a child process changes state. Returns the PID of the changed child. The `WaitStatus` value tells you why it stopped:

```go
var ws unix.WaitStatus
unix.Wait4(pid, &ws, 0, nil)

if ws.Stopped() && ws.StopSignal() == unix.SIGTRAP {
    // tracee hit a syscall boundary - inspect registers
}
```

### `PtraceRegs` and the x86-64 register layout

On x86-64 Linux, `PtraceRegs` maps to the kernel's `user_regs_struct`. The fields you need for syscall inspection:

| Field | At syscall entry | At syscall exit |
|---|---|---|
| `Orig_rax` | Syscall number | Syscall number (unchanged) |
| `Rdi` | Argument 1 | Argument 1 |
| `Rsi` | Argument 2 | Argument 2 |
| `Rdx` | Argument 3 | Argument 3 |
| `Rax` | Syscall number (clobbered) | Return value |

The key insight: **use `Orig_rax` to identify the syscall** - the kernel saves the original `rax` before overwriting it with the return value.

### x86-64 syscall numbers (the ones you will intercept)

| Number | Name | Arguments |
|---|---|---|
| 0 | `read` | fd, buf*, count |
| 1 | `write` | fd, buf*, count |
| 2 | `open` | path*, flags, mode |
| 257 | `openat` | dirfd, path*, flags, mode |
| 59 | `execve` | path*, argv**, envp** |
| 60 | `exit` | status |
| 231 | `exit_group` | status |

Modern Linux programs use `openat` more often than `open`. Your tracer should intercept both.

---

## The `os/exec` Package

**Official documentation:** https://pkg.go.dev/os/exec

`os/exec` is how Go programs launch child processes at a higher level than raw `syscall`. It wraps `os.StartProcess` with a cleaner API. Critically for this project, it is the attachment point for `syscall.SysProcAttr` - which is how you pass namespace flags and ptrace options to the child process.

### The `Cmd` struct

```go
cmd := exec.Command("/bin/sh")
cmd.Stdin  = os.Stdin
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
cmd.SysProcAttr = &syscall.SysProcAttr{
    Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWUTS | syscall.CLONE_NEWNS,
}
cmd.Start()  // fork+exec; returns immediately
cmd.Wait()   // block until child exits
```

The `SysProcAttr` field is where all the low-level process setup lives. When you call `cmd.Start()`, Go internally calls `clone(2)` with the flags you set, then `execve` in the child.

### Re-executing the same binary

A common pattern in container runtimes (used by Docker, runc, and others) is to have the binary **re-execute itself** with a special argument to become the child:

```go
// Parent path
func runContainer() {
    cmd := exec.Command("/proc/self/exe", "child")
    cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: ...}
    cmd.Run()
}

// Child path
func runChild() {
    // We are now inside the namespaces - set up rootfs, exec payload
    syscall.Sethostname([]byte("mycontainer"))
    syscall.Chroot("/tmp/rootfs")
    syscall.Exec("/bin/sh", []string{"/bin/sh"}, os.Environ())
}
```

This avoids the need for a separate binary for the child and keeps all the logic in one file. `/proc/self/exe` is a Linux symlink to the currently running executable.

---

## Linux `ptrace(2)`

**Official man page:** https://man7.org/linux/man-pages/man2/ptrace.2.html

`ptrace` is the system call that makes debuggers and syscall tracers possible. It allows one process (the *tracer*) to observe and control another (the *tracee*): inspect and modify memory and registers, intercept system calls, and redirect signals.

### How it works for syscall interception

The lifecycle is:

1. **Child opts in.** The child calls `PTRACE_TRACEME` (or sets `SysProcAttr.Ptrace = true`) before `exec`. This marks it as traceable by its parent.
2. **Child stops after exec.** The kernel automatically stops the child right after `execve` and sends `SIGCHLD` to the parent.
3. **Parent waits.** The parent calls `Wait4` and receives the initial stop.
4. **Parent sets options.** The parent uses `PTRACE_SETOPTIONS` to request `PTRACE_O_TRACESYSGOOD`, which sets bit 7 of the stop signal for syscall-stops, making them easy to distinguish from regular signal-stops.
5. **Parent loops.** The parent calls `PtraceSyscall` to resume the child, then `Wait4` to wait for the next stop. The child stops at every syscall entry and exit.
6. **Parent inspects.** At each stop, the parent calls `PtraceGetRegs` to read the registers and determine what syscall is being made and what its arguments are.

```
[child]  PTRACE_TRACEME
[child]  exec → kernel stops child, sends SIGCHLD
[parent] Wait4 → receives stop
[parent] PtraceSyscall(pid, 0) → resumes child
[child]  calls open("/etc/hostname") → kernel stops child at syscall ENTRY
[parent] Wait4 → receives syscall-stop
[parent] PtraceGetRegs → reads Orig_rax=2 (open), Rdi=addr of "/etc/hostname"
[parent] PtraceSyscall(pid, 0) → resumes child
[child]  kernel executes open() → stops child at syscall EXIT
[parent] Wait4 → receives syscall-stop
[parent] PtraceGetRegs → reads Rax=3 (returned fd)
[parent] PtraceSyscall(pid, 0) → resumes child
... continues for every syscall
```

### The two-stop model

Every intercepted syscall causes **two stops**: one at entry (before the kernel executes the syscall) and one at exit (after). You must track which stop you are on. A common way is a boolean toggle that flips each time `Wait4` returns a syscall-stop.

At **entry**: registers hold the syscall number (`Orig_rax`) and arguments (`Rdi`, `Rsi`, `Rdx`, `R10`, `R8`, `R9`). The filename pointer in `open` is valid here - read it with `PtracePeekData`.

At **exit**: `Rax` holds the return value (file descriptor for `open`, byte count for `read`/`write`, or a negative errno on error). `Orig_rax` still holds the syscall number.

### Required privilege

`ptrace` requires `CAP_SYS_PTRACE`. Running the container inside Docker with `--privileged` satisfies this. Alternatively, Docker's `--cap-add SYS_PTRACE` grants only this capability without full privilege.

---

## Linux Namespaces

**Official man page:** https://man7.org/linux/man-pages/man7/namespaces.7.html

Namespaces are the kernel feature that makes containers possible. Each namespace type wraps a particular set of global resources so that processes within a namespace see their own independent copy. A process in a new PID namespace sees PIDs starting from 1; a process in a new UTS namespace can set a hostname without affecting the host.

### Namespace types

| Namespace | Clone flag | What it isolates |
|---|---|---|
| Mount | `CLONE_NEWNS` | Filesystem mount points |
| UTS | `CLONE_NEWUTS` | Hostname and NIS domain |
| IPC | `CLONE_NEWIPC` | System V IPC, POSIX message queues |
| PID | `CLONE_NEWPID` | Process IDs |
| Network | `CLONE_NEWNET` | Network devices, IP stacks, ports |
| User | `CLONE_NEWUSER` | UID/GID mappings |
| Cgroup | `CLONE_NEWCGROUP` | cgroup root |
| Time | `CLONE_NEWTIME` | Boot and monotonic clocks (Linux 5.6+) |

For this project, you will use **UTS**, **PID**, and **Mount**. Network and User namespaces add significant complexity and are out of scope.

### What each namespace gives you

**PID namespace (`CLONE_NEWPID`):** The first process created in a new PID namespace is assigned PID 1 inside that namespace. It sees only its own descendants. From the host, you can still see the container's processes with their host PIDs - namespaces are a view, not a wall.

**UTS namespace (`CLONE_NEWUTS`):** Lets the container set its own hostname without affecting the host. A small but important isolation: tools inside the container that read `/proc/sys/kernel/hostname` see the container's hostname, not the host's.

**Mount namespace (`CLONE_NEWNS`):** Gives the container its own copy of the mount table. You can mount and unmount filesystems (like `/proc`) inside the container without those changes propagating to the host. This is what enables `pivot_root` and the isolated rootfs in Step 3.

### Inspecting namespaces

Every process's namespace memberships are visible as symlinks:

```
/proc/<pid>/ns/pid    -> pid:[4026531836]
/proc/<pid>/ns/uts    -> uts:[4026531838]
/proc/<pid>/ns/mnt    -> mnt:[4026531840]
```

Two processes that share a namespace will have symlinks pointing to the same inode number. You can verify your namespace setup is working by comparing these values between the host shell and the container process.

---

# Linux `clone(2)`

**Official man page:** https://man7.org/linux/man-pages/man2/clone.2.html

`clone(2)` is the system call that creates a new process with fine-grained control over what it shares with or inherits from the parent. It is a superset of `fork(2)`. Linux threads are created with `clone` using sharing flags (`CLONE_VM`, `CLONE_FILES`, etc.); containers are created with `clone` using isolation flags (`CLONE_NEW*`).

In Go, you do not call `clone` directly. When you call `cmd.Start()` with a populated `SysProcAttr.Cloneflags`, the Go runtime calls `clone` with those flags on your behalf. Understanding `clone` helps you understand what `Cloneflags` actually does.

### Combining flags

Flags can be combined freely. The most common combination for a minimal container:

```
CLONE_NEWPID | CLONE_NEWUTS | CLONE_NEWNS
```

Adding `CLONE_NEWNET` gives network isolation but requires additional setup (a veth pair) to be useful. Adding `CLONE_NEWUSER` enables unprivileged containers but requires UID/GID mapping files to be written before the child starts.

### Privilege requirements

All `CLONE_NEW*` flags except `CLONE_NEWUSER` require `CAP_SYS_ADMIN`. Inside a Docker container with `--privileged`, this capability is available. Without privilege, only `CLONE_NEWUSER` works, and it is typically used first to gain a capability context for the others.

---

## cgroups v2

**Official kernel documentation:** https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html

Control Groups (cgroups) are the kernel mechanism for limiting, accounting, and isolating resource usage (CPU, memory, I/O, PIDs) of process groups. Version 2 introduced a unified hierarchy - a single tree under `/sys/fs/cgroup/` - replacing the per-subsystem trees of v1. It is the default on all modern Linux distributions (kernel 5.3+, systemd 243+).

### The filesystem interface

cgroups v2 uses a virtual filesystem. You manage cgroups by reading and writing files under `/sys/fs/cgroup/`. There is no special library or syscall - just file I/O:

```
/sys/fs/cgroup/
├── cgroup.controllers       # controllers available at root
├── cgroup.subtree_control   # controllers enabled for children
├── mycontainer/             # your cgroup (mkdir to create)
│   ├── cgroup.procs         # write a PID here to add a process
│   ├── memory.max           # hard memory limit in bytes
│   └── cpu.max              # CPU bandwidth: "<quota> <period>"
```

Creating a cgroup is as simple as `os.MkdirAll`. Adding a process is writing its PID to `cgroup.procs`. Setting a memory limit:

```go
os.WriteFile("/sys/fs/cgroup/mycontainer/memory.max", []byte("67108864"), 0700) // 64 MiB
```

Setting a CPU limit (50% of one CPU):

```go
os.WriteFile("/sys/fs/cgroup/mycontainer/cpu.max", []byte("50000 100000"), 0700)
```

Cleanup: remove the directory after the child exits. The directory must be empty (no processes) before `os.Remove` will succeed.

### Key interface files

| File | Purpose |
|---|---|
| `cgroup.procs` | PIDs of processes in this cgroup (write to migrate) |
| `cgroup.controllers` | Controllers available to this cgroup |
| `cgroup.subtree_control` | Enable controllers for child cgroups |
| `memory.max` | Hard memory limit (OOM kill if exceeded) |
| `memory.current` | Current memory usage |
| `cpu.max` | `<quota> <period>` in microseconds |
| `cpu.weight` | Relative scheduling weight (1–10000, default 100) |
| `pids.max` | Maximum number of PIDs (prevents fork bombs) |
| `pids.current` | Current PID count |

### The no-internal-process rule

A cgroup can either contain processes or have child cgroups - not both. This means you must place container processes in a leaf cgroup, not the root. Creating a subdirectory (`/sys/fs/cgroup/mycontainer/`) and using that as the leaf is the standard approach.

### Docker and cgroup delegation

When running inside Docker, the container runtime may restrict which cgroup subtree you can write to. Use `--cgroupns private` and mount `/sys/fs/cgroup` with write access, or run with `--privileged`. Check that `/sys/fs/cgroup/cgroup.controllers` is non-empty before implementing this step.

---

## Further Reading

| Topic | Resource |
|---|---|
| How containers work (deep dive) | [Linux containers from scratch](https://ericchiang.github.io/post/containers-from-scratch/) by Eric Chiang |
| Namespace internals | [LWN.net namespace series](https://lwn.net/Articles/531114/) (7-part series, 2013) |
| ptrace and strace internals | [How does strace work?](https://blog.packagecloud.io/how-does-strace-work/) - packagecloud blog |
| runc source code | https://github.com/opencontainers/runc - the reference OCI runtime |
| gVisor (ptrace-based sandbox) | https://github.com/google/gvisor - a production ptrace/KVM container runtime |
| OCI Runtime Specification | https://github.com/opencontainers/runtime-spec - the spec that Docker, Podman, and containerd implement |
