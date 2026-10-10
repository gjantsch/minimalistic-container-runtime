package container

import (
	"fmt"
	"os"
	"syscall"

	"github.com/gjantsch/container/internal/exec"
	"github.com/gjantsch/container/internal/namespace"
	"github.com/gjantsch/container/internal/rootfs"
	"github.com/gjantsch/container/internal/tracer"
)

// runParent initializes the parent process and starts the child process.
func RunParent(args []string) {
	fmt.Printf("[parent] PID: %d\n", os.Getpid())
	cmd := namespace.Namespace(args)
	tracer.Run(cmd)
	fmt.Printf("[parent] process state: %v\n", cmd.ProcessState)
	fmt.Printf("[parent] finished\n")
}

// runChild sets up the child process environment and executes the specified command.
func RunChild(args []string) {
	fmt.Printf("[child] PID: %d\n", os.Getpid())

	// set up the root filesystem for the container
	containerRoot := "/tmp/container-root"
	fmt.Printf("[child] setup filesystem at %s\n", containerRoot)
	err := rootfs.Setup(containerRoot)
	if err != nil {
		fmt.Printf("[child] error setting up rootfs: %v\n", err)
		return
	}

	// set hostname for the container
	// in this context, the hostname is a string label that the kernel
	// stores per UTS (Unix Timesharing System) namespace.
	hostname := "child-container"
	syscall.Sethostname([]byte(hostname))
	fmt.Printf("[child] hostname: %s\n", hostname)
	fmt.Printf("[child] PID: %d\n", os.Getpid())

	// check args
	if len(args) == 0 {
		fmt.Printf("[child] no command provided\n")
		return
	}

	// enable ptrace for the child process

	// execute command, is important to notice that
	// Exec replaces the current process with the specified command.
	// So any code after this point will not be executed if Exec is successful.
	fmt.Printf("[child] exec: %s %v\n", args[0], args[1:])
	err = exec.Exec(args[0], args[1:])
	if err != nil {
		fmt.Printf("[child] error executing %v: %v\n", args, err)
	}
}
