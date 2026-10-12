package container

import (
	"fmt"
	"os"
	"syscall"

	"github.com/gjantsch/container/internal/cgroup"
	containerexec "github.com/gjantsch/container/internal/exec"
	"github.com/gjantsch/container/internal/namespace"
	"github.com/gjantsch/container/internal/rootfs"
	"github.com/gjantsch/container/internal/tracer"
)

type Config struct {
	MemBytes  int64
	CPUQuota  int64
	CPUPeriod int64
	Cmd       []string
}

func RunParent(cfg Config) {
	fmt.Printf("[parent] supervisor PID: %d\n", os.Getpid())

	if cfg.MemBytes > 0 || cfg.CPUQuota > 0 {
		if err := cgroup.Setup("mycontainer", cfg.MemBytes, cfg.CPUQuota, cfg.CPUPeriod); err != nil {
			fmt.Printf("[parent] cgroup setup error: %v\n", err)
			return
		}
	}

	cmd := namespace.Namespace(cfg.Cmd)

	if err := tracer.Start(cmd); err != nil {
		fmt.Printf("[parent] start error: %v\n", err)
		return
	}

	if cfg.MemBytes > 0 || cfg.CPUQuota > 0 {
		if err := cgroup.AddPID("mycontainer", cmd.Process.Pid); err != nil {
			fmt.Printf("[parent] cgroup addpid error: %v\n", err)
		}
	}

	if err := tracer.Wait(cmd); err != nil {
		fmt.Printf("[parent] error: %v\n", err)
	}

	if cfg.MemBytes > 0 || cfg.CPUQuota > 0 {
		if err := cgroup.Cleanup("mycontainer"); err != nil {
			fmt.Printf("[parent] cgroup cleanup error: %v\n", err)
		} else {
			fmt.Printf("[parent] cgroup cleaned up\n")
		}
	}

	fmt.Printf("[parent] container exited: %v\n", cmd.ProcessState)
}

func RunChild(args []string) {
	containerRoot := "/tmp/container-root"
	fmt.Printf("[child] running\n")

	err := rootfs.Setup(containerRoot)
	if err != nil {
		fmt.Printf("[child] error setting up rootfs: %v\n", err)
		return
	}

	hostname := fmt.Sprintf("mycontainer-%d", os.Getpid())
	syscall.Sethostname([]byte(hostname))
	fmt.Printf("[child] hostname: %s  PID: %d\n", hostname, os.Getpid())

	if len(args) == 0 {
		fmt.Printf("[child] no command provided\n")
		return
	}

	fmt.Printf("[child] exec: %s %v\n", args[0], args[1:])
	err = containerexec.Exec(args[0], args[1:])
	if err != nil {
		fmt.Printf("[child] error executing %v: %v\n", args, err)
	}
}
