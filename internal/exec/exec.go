package exec

import (
	"os"
	"syscall"
)

func Exec(cmd string, args []string) error {
	argv := append([]string{cmd}, args...)
	return syscall.Exec(cmd, argv, os.Environ())
}
