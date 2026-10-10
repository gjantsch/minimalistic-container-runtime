package tracer

import (
	"fmt"
	"os/exec"

	"golang.org/x/sys/unix"
)

func Run(cmd *exec.Cmd) error {
	cmd.SysProcAttr.Ptrace = true

	if err := cmd.Start(); err != nil {
		return err
	}

	pid := cmd.Process.Pid
	var ws unix.WaitStatus

	for {
		// block until the child stops for any reason
		_, err := unix.Wait4(pid, &ws, 0, nil)
		if err != nil {
			return err
		}

		// child exited or was killed — we are done
		if ws.Exited() || ws.Signaled() {
			break
		}

		// syscall stop: SIGTRAP is what ptrace delivers
		inSyscall := false
		if ws.StopSignal() == unix.SIGTRAP {
			var regs unix.PtraceRegs
			unix.PtraceGetRegs(pid, &regs)

			if !inSyscall {
				fmt.Printf("SYSCALL ENTER: %v\n", regs.Orig_rax)
				inSyscall = true
			} else {
				fmt.Printf("SYSCALL EXIT:  %v\n", regs.Orig_rax)
				inSyscall = false
			}

			// TODO Step 6: decode open/read/write
		}

		// resume the child until the next syscall boundary
		unix.PtraceSyscall(pid, 0)
	}

	return cmd.Wait()

}
