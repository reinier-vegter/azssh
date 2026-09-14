package shell

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"azssh/internal/inventory"
)

// StartMount creates a foreground SSHFS mount through the selected Bastion.
// It leaves an empty user-owned mountpoint behind but removes all credentials.
func StartMount(route inventory.BastionRoute, vm inventory.VirtualMachine, remotePath string, authentication Authentication) error {
	if !usesAAD(authentication) {
		return fmt.Errorf("mounting requires AAD authentication")
	}
	if err := validateMountInputs(vm.Name, remotePath); err != nil {
		return err
	}
	for _, executable := range []string{"az", "ssh-keygen", "sshfs"} {
		if _, err := exec.LookPath(executable); err != nil {
			if executable == "sshfs" {
				return fmt.Errorf("sshfs is required for mounting; install SSHFS with FUSE 3 on Linux or macFUSE/FUSE-T on macOS: %w", err)
			}
			return fmt.Errorf("%s is required for mounting: %w", executable, err)
		}
	}

	mountpoint, err := mountpointForVM(vm.Name)
	if err != nil {
		return err
	}
	session, err := startEntraSession(route, vm, "mount")
	if err != nil {
		return err
	}
	defer session.Close()
	return runSSHFS(session.username, remotePath, mountpoint, session.keyPath, session.certificatePath, session.port)
}

func usesAAD(authentication Authentication) bool {
	typeName := strings.TrimSpace(authentication.Type)
	return typeName == "" || strings.EqualFold(typeName, "AAD")
}

func validateMountInputs(vmName, remotePath string) error {
	if vmName == "" || vmName == "." || vmName == ".." || strings.ContainsAny(vmName, "/\\\x00") {
		return fmt.Errorf("VM name cannot be used as a mountpoint")
	}
	if remotePath = strings.TrimSpace(remotePath); remotePath == "" || strings.ContainsAny(remotePath, "\x00\r\n") {
		return fmt.Errorf("remote path must be a non-empty POSIX path")
	}
	return nil
}

func mountpointForVM(vmName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	mountpoint := filepath.Join(home, "azssh", "mnt", vmName)
	if err := os.MkdirAll(mountpoint, 0o700); err != nil {
		return "", fmt.Errorf("create mountpoint: %w", err)
	}
	info, err := os.Stat(mountpoint)
	if err != nil {
		return "", fmt.Errorf("inspect mountpoint: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("mountpoint is not a directory: %s", mountpoint)
	}
	if mounted, err := isMountpoint(mountpoint); err != nil {
		return "", err
	} else if mounted {
		return "", fmt.Errorf("mountpoint is already mounted: %s", mountpoint)
	}
	entries, err := os.ReadDir(mountpoint)
	if err != nil {
		return "", fmt.Errorf("inspect mountpoint contents: %w", err)
	}
	if len(entries) != 0 {
		return "", fmt.Errorf("mountpoint is not empty: %s", mountpoint)
	}
	return mountpoint, nil
}

func isMountpoint(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("inspect mountpoint: %w", err)
	}
	parent, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return false, fmt.Errorf("inspect mountpoint parent: %w", err)
	}
	pathStat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("inspect mountpoint filesystem")
	}
	parentStat, ok := parent.Sys().(*syscall.Stat_t)
	if !ok {
		return false, fmt.Errorf("inspect mountpoint parent filesystem")
	}
	return pathStat.Dev != parentStat.Dev, nil
}

func runSSHFS(username, remotePath, mountpoint, keyPath, certificatePath string, port int) error {
	command := exec.Command("sshfs", sshfsArgs(username, remotePath, mountpoint, keyPath, certificatePath, port)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start SSHFS: %w", err)
	}
	fmt.Fprintln(os.Stdout, mountStartedMessage(mountpoint))
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("SSHFS exited: %w", err)
		}
		return nil
	case <-signals:
		unmountErr := unmount(mountpoint)
		if mounted, err := isMountpoint(mountpoint); err == nil && mounted {
			if command.Process != nil {
				_ = command.Process.Kill()
			}
			<-done
			return unmountErr
		}
		<-done
		return nil
	}
}

func mountStartedMessage(mountpoint string) string {
	return "SSHFS mounted at " + mountpoint + ". Press Ctrl-C to unmount."
}

func sshfsArgs(username, remotePath, mountpoint, keyPath, certificatePath string, port int) []string {
	return []string{
		username + "@127.0.0.1:" + remotePath, mountpoint,
		"-f", "-F", "/dev/null",
		"-o", "HostName=127.0.0.1",
		"-o", "Port=" + strconv.Itoa(port),
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityFile=" + keyPath,
		"-o", "CertificateFile=" + certificatePath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}
}

func unmount(mountpoint string) error {
	commands := [][]string{{"umount", mountpoint}}
	if runtime.GOOS == "linux" {
		commands = [][]string{{"fusermount3", "-u", mountpoint}, {"fusermount", "-u", mountpoint}}
	}
	var failures []string
	for _, args := range commands {
		if _, err := exec.LookPath(args[0]); err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if output, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err == nil {
			return nil
		} else {
			failures = append(failures, strings.TrimSpace(string(output)))
		}
	}
	return fmt.Errorf("unmount %s: %s", mountpoint, strings.Join(failures, "; "))
}
