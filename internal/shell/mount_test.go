package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateMountInputs(t *testing.T) {
	for _, test := range []struct {
		vmName     string
		remotePath string
		wantErr    bool
	}{
		{vmName: "api-01", remotePath: "."},
		{vmName: "api-01", remotePath: "/srv/data"},
		{vmName: "../api", remotePath: ".", wantErr: true},
		{vmName: "api-01", remotePath: "", wantErr: true},
		{vmName: "api-01", remotePath: "data\nother", wantErr: true},
	} {
		err := validateMountInputs(test.vmName, test.remotePath)
		if (err != nil) != test.wantErr {
			t.Errorf("validateMountInputs(%q, %q) error = %v, want error %v", test.vmName, test.remotePath, err, test.wantErr)
		}
	}
}

func TestMountpointForVMCreatesAndRejectsNonemptyDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mountpoint, err := mountpointForVM("api-01")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "azssh", "mnt", "api-01"); mountpoint != want {
		t.Fatalf("mountpoint = %q, want %q", mountpoint, want)
	}
	if err := os.WriteFile(filepath.Join(mountpoint, "occupied"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mountpointForVM("api-01"); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("non-empty mountpoint error = %v", err)
	}
}

func TestSSHFSArgsUseOnlyTemporaryConnectionSettings(t *testing.T) {
	got := sshfsArgs("entra-user", "/srv/data", "/home/user/azssh/mnt/api-01", "/tmp/key", "/tmp/cert", 22022)
	want := []string{
		"entra-user@127.0.0.1:/srv/data", "/home/user/azssh/mnt/api-01",
		"-f", "-F", "/dev/null",
		"-o", "HostName=127.0.0.1",
		"-o", "Port=22022",
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityFile=/tmp/key",
		"-o", "CertificateFile=/tmp/cert",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sshfsArgs() = %#v, want %#v", got, want)
	}
	if strings.Contains(strings.Join(got, " "), ".ssh") {
		t.Fatal("sshfs arguments must not refer to ~/.ssh")
	}
}

func TestMountStartedMessageIncludesMountpoint(t *testing.T) {
	if got, want := mountStartedMessage("/home/user/azssh/mnt/api-01"), "SSHFS mounted at /home/user/azssh/mnt/api-01. Press Ctrl-C to unmount."; got != want {
		t.Fatalf("mountStartedMessage() = %q, want %q", got, want)
	}
}
