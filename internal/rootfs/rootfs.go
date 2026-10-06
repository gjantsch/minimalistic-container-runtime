package rootfs

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

func Setup(rootFSPath string) error {
	// create the root path
	err := os.MkdirAll(rootFSPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create rootFS path: %w", err)
	}

	// basic directory structure
	subDirs := []string{"bin", "proc", "tmp"}
	for _, dir := range subDirs {
		err := os.MkdirAll(rootFSPath+"/"+dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create subdirectory %s: %w", dir, err)
		}
	}

	// copy the BusyBox binary into the container's root filesystem
	err = copyBusyBox("/bin/busybox", rootFSPath+"/bin/busybox")
	if err != nil {
		return fmt.Errorf("failed to copy BusyBox binary: %w", err)
	}

	// create symlinks for common binaries to BusyBox
	binaries := []string{"bin/sh", "bin/ls", "bin/mkdir", "bin/rm", "bin/ps", "bin/cat", "bin/echo"}
	for _, bin := range binaries {
		err := os.Symlink(rootFSPath+"/bin/busybox", rootFSPath+"/"+bin)
		if err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", bin, err)
		}
	}

	// change the root filesystem to the container's root
	err = syscall.Chroot(rootFSPath)
	if err != nil {
		return fmt.Errorf("failed to chroot to %s: %w", rootFSPath, err)
	}

	// change the current working directory to the new root
	err = os.Chdir("/")
	if err != nil {
		return fmt.Errorf("failed to change directory to /: %w", err)
	}

	// mount the proc filesystem to /proc within the container's root filesystem
	err = syscall.Mount("proc", "/proc", "proc", 0, "")
	if err != nil {
		return fmt.Errorf("failed to mount proc filesystem: %w", err)
	}
	return nil
}

// a custom copy function for BusyBox binary
// to keep things low level
func copyBusyBox(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
