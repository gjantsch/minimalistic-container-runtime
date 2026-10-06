package rootfs

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

// list of common binaries implemented in the container
var binaries = []string{"sh", "ls", "mkdir", "rm", "ps", "cat", "echo", "cp", "mv", "pwd", "ln"}

// Setup the Root File System Path of the container
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
	err = copyFile("/bin/busybox.static", rootFSPath+"/bin/busybox")
	if err != nil {
		return fmt.Errorf("failed to copy BusyBox binary: %w", err)
	}

	// this is the start of the isolation
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

	// create symlinks for common binaries to BusyBox after
	// the container is closed and the root filesystem is set up
	// this simplifies how references are perceived within the container.
	for _, bin := range binaries {
		err := os.Symlink("/bin/busybox", "/bin/"+bin)
		if err != nil {
			return fmt.Errorf("failed to create symlink for %s: %w", bin, err)
		}
	}

	// mount the proc filesystem to /proc within the container's root filesystem
	// note that the /proc directory must exist
	err = syscall.Mount("proc", "/proc", "proc", 0, "")
	if err != nil {
		return fmt.Errorf("failed to mount proc filesystem: %w", err)
	}

	return nil
}

// a custom copy function for BusyBox binary
// to keep things low level
func copyFile(src, dst string) error {
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
