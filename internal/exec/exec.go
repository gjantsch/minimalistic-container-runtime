package exec

import (
	"os"
	"syscall"
)

// Exec replaces the current process with the specified command.
// cmd parameter specifies the command to execute
// args parameter specifies the arguments to pass to the command
func Exec(cmd string, args []string) error {
	argv := append([]string{cmd}, args...)
	return syscall.Exec(cmd, argv, os.Environ())
}
