package tracer

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// x86-64 syscall numbers
const (
	sysRead   = 0
	sysWrite  = 1
	sysOpen   = 2
	sysOpenat = 257
)

func decodeSyscall(pid int, regs *unix.PtraceRegs) {
	switch regs.Orig_rax {
	case sysOpen:
		path := readString(pid, uintptr(regs.Rdi))
		fmt.Printf("[OPEN]   path=%s  flags=%s\n", path, decodeFlags(regs.Rsi))
	case sysOpenat:
		path := readString(pid, uintptr(regs.Rsi))
		fmt.Printf("[OPEN]   path=%s  flags=%s\n", path, decodeFlags(regs.Rdx))
	case sysRead:
		fmt.Printf("[READ]   fd=%d  count=%d\n", regs.Rdi, regs.Rdx)
	case sysWrite:
		fmt.Printf("[WRITE]  fd=%d  count=%d\n", regs.Rdi, regs.Rdx)
	}
}

func decodeSyscallExit(pid int, regs *unix.PtraceRegs) {
	switch regs.Orig_rax {
	case sysRead:
		if int64(regs.Rax) > 0 {
			fmt.Printf("[READ]   fd=%d  bytes=%d\n", regs.Rdi, regs.Rax)
		}
	case sysWrite:
		fmt.Printf("[WRITE]  fd=%d  bytes=%d\n", regs.Rdi, regs.Rax)
	}
}

func decodeFlags(flags uint64) string {
	switch flags & 3 {
	case unix.O_RDONLY:
		return "O_RDONLY"
	case unix.O_WRONLY:
		return "O_WRONLY"
	case unix.O_RDWR:
		return "O_RDWR"
	default:
		return fmt.Sprintf("0x%x", flags)
	}
}
