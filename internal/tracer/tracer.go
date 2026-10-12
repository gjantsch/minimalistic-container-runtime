package tracer

import (
	"os/exec"

	"golang.org/x/sys/unix"
)

func Start(cmd *exec.Cmd) error {
	cmd.SysProcAttr.Ptrace = true
	return cmd.Start()
}

func Wait(cmd *exec.Cmd) error {
	pid := cmd.Process.Pid
	var ws unix.WaitStatus
	inSyscall := false

	for {
		_, err := unix.Wait4(pid, &ws, 0, nil)
		if err != nil {
			return err
		}

		if ws.Exited() || ws.Signaled() {
			break
		}

		if ws.StopSignal() == unix.SIGTRAP {
			var regs unix.PtraceRegs
			unix.PtraceGetRegs(pid, &regs)

			if !inSyscall {
				decodeSyscall(pid, &regs)
				inSyscall = true
			} else {
				decodeSyscallExit(pid, &regs)
				inSyscall = false
			}
		}

		unix.PtraceSyscall(pid, 0)
	}

	// Wait4 already reaped the child; cmd.Wait releases internal resources.
	cmd.Wait() //nolint
	return nil
}

func Run(cmd *exec.Cmd) error {
	if err := Start(cmd); err != nil {
		return err
	}
	return Wait(cmd)
}

func readString(pid int, addr uintptr) string {
	buf := make([]byte, 0, 256)
	word := make([]byte, 8)
	for {
		_, err := unix.PtracePeekData(pid, addr, word)
		if err != nil {
			break
		}
		for _, b := range word {
			if b == 0 {
				return string(buf)
			}
			buf = append(buf, b)
		}
		addr += 8
		if len(buf) > 512 {
			break
		}
	}
	return string(buf)
}
