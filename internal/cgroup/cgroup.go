package cgroup

import (
	"fmt"
	"os"
	"path/filepath"
)

const cgroupMount = "/sys/fs/cgroup"

func Setup(name string, memBytes int64, cpuQuota int64, cpuPeriod int64) error {
	dir := filepath.Join(cgroupMount, name)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cgroup mkdir: %w", err)
	}

	// cgroup v2 "no internal processes" rule: move self into a leaf so the
	// parent dir becomes process-free and accepts subtree_control writes.
	selfLeaf := filepath.Join(dir, "supervisor")
	if err := os.MkdirAll(selfLeaf, 0755); err != nil {
		return fmt.Errorf("cgroup supervisor mkdir: %w", err)
	}
	if err := writeFile(selfLeaf, "cgroup.procs", fmt.Sprintf("%d\n", os.Getpid())); err != nil {
		return fmt.Errorf("cgroup move self: %w", err)
	}

	if err := writeFile(dir, "cgroup.subtree_control", "+memory +cpu\n"); err != nil {
		return fmt.Errorf("cgroup subtree_control: %w", err)
	}

	childDir := filepath.Join(dir, "payload")
	if err := os.MkdirAll(childDir, 0755); err != nil {
		return fmt.Errorf("cgroup payload mkdir: %w", err)
	}

	if memBytes > 0 {
		if err := writeFile(childDir, "memory.max", fmt.Sprintf("%d\n", memBytes)); err != nil {
			return err
		}
	}

	if cpuQuota > 0 {
		if err := writeFile(childDir, "cpu.max", fmt.Sprintf("%d %d\n", cpuQuota, cpuPeriod)); err != nil {
			return err
		}
	}

	fmt.Printf("[parent] cgroup: memory.max=%d  cpu.max=%d %d\n", memBytes, cpuQuota, cpuPeriod)
	return nil
}

func AddPID(name string, pid int) error {
	childDir := filepath.Join(cgroupMount, name, "payload")
	return writeFile(childDir, "cgroup.procs", fmt.Sprintf("%d\n", pid))
}

func Cleanup(name string) error {
	dir := filepath.Join(cgroupMount, name)

	// Move self back to the root cgroup so the supervisor leaf becomes empty
	// and can be rmdir'd.
	if err := writeFile(cgroupMount, "cgroup.procs", fmt.Sprintf("%d\n", os.Getpid())); err != nil {
		return fmt.Errorf("cgroup move self to root: %w", err)
	}

	for _, leaf := range []string{"payload", "supervisor"} {
		if err := os.Remove(filepath.Join(dir, leaf)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cgroup remove %s: %w", leaf, err)
		}
	}
	return os.Remove(dir)
}

func writeFile(dir, file, value string) error {
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(value), 0644); err != nil {
		return fmt.Errorf("cgroup write %s: %w", file, err)
	}
	return nil
}
