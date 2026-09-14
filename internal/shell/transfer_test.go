package shell

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"azssh/internal/inventory"

	"golang.org/x/crypto/ssh"
)

func TestCertificatePrincipal(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	certificate := &ssh.Certificate{Key: signer.PublicKey(), CertType: ssh.UserCert, ValidPrincipals: []string{"entra-user"}}
	if err := certificate.SignCert(rand.Reader, signer); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity-cert.pub")
	if err := os.WriteFile(path, ssh.MarshalAuthorizedKey(certificate), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := certificatePrincipal(path); err != nil || got != "entra-user" {
		t.Fatalf("certificatePrincipal() = %q, %v", got, err)
	}
}

func TestTransferRCRestrictsRemoteAlias(t *testing.T) {
	directory := t.TempDir()
	argumentsPath := filepath.Join(directory, "scp-arguments")
	fakeSCP := filepath.Join(directory, "scp")
	if err := os.WriteFile(fakeSCP, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$AZSSH_TEST_ARGUMENTS\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	rcPath := filepath.Join(directory, "rc")
	rc := transferRC("api-01", "entra-user", "/tmp/key", "/tmp/cert", 22022, inventory.BastionRoute{Bastion: inventory.Bastion{Name: "bastion", SubscriptionID: "sub", ResourceGroup: "rg"}})
	if err := os.WriteFile(rcPath, []byte(rc), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AZSSH_TEST_ARGUMENTS", argumentsPath)

	command := exec.Command("bash", "--noprofile", "--rcfile", rcPath, "-ic", "scp -r ./report.csv api-01:/home/entra-user/")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("allowed scp failed: %v\n%s", err, output)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"-F", "/dev/null", "HostName=127.0.0.1", "Port=22022", "User=entra-user", "IdentitiesOnly=yes", "api-01:/home/entra-user/"} {
		if !strings.Contains(string(arguments), expected) {
			t.Errorf("wrapped scp arguments do not contain %q:\n%s", expected, arguments)
		}
	}

	if err := os.Remove(argumentsPath); err != nil {
		t.Fatal(err)
	}
	command = exec.Command("bash", "--noprofile", "--rcfile", rcPath, "-ic", "scp other-host:/tmp/file .")
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "only api-01:<path>") {
		t.Fatalf("unexpected arbitrary-host result: %v\n%s", err, output)
	}
	if _, err := os.Stat(argumentsPath); !os.IsNotExist(err) {
		t.Fatalf("arbitrary host invoked real scp: %v", err)
	}
	command = exec.Command("bash", "--noprofile", "--rcfile", rcPath, "-ic", "scp -ro ProxyCommand=malicious ./report.csv api-01:/tmp/")
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "connection override") {
		t.Fatalf("unexpected connection-override result: %v\n%s", err, output)
	}
}
