package shell

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"azssh/internal/inventory"

	"golang.org/x/crypto/ssh"
)

const transferTunnelTimeout = 20 * time.Second

// StartTransferShell opens a temporary Bash session for Entra-authenticated scp
// transfers to or from the selected VM. It removes all temporary state on exit.
func StartTransferShell(route inventory.BastionRoute, vm inventory.VirtualMachine, authentication Authentication) error {
	if !usesAAD(authentication) {
		return fmt.Errorf("file transfer requires AAD authentication")
	}
	for _, executable := range []string{"az", "ssh-keygen", "scp", "bash"} {
		if _, err := exec.LookPath(executable); err != nil {
			return fmt.Errorf("%s is required for file transfer: %w", executable, err)
		}
	}

	session, err := startEntraSession(route, vm, "transfer")
	if err != nil {
		return err
	}
	defer session.Close()
	rcPath := filepath.Join(session.directory, "bashrc")
	if err := os.WriteFile(rcPath, []byte(transferRC(vm.Name, session.username, session.keyPath, session.certificatePath, session.port, route)), 0o600); err != nil {
		return fmt.Errorf("write transfer shell configuration: %w", err)
	}
	bash := exec.Command("bash", "--noprofile", "--rcfile", rcPath, "-i")
	bash.Stdin, bash.Stdout, bash.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := bash.Run(); err != nil {
		return fmt.Errorf("transfer shell exited: %w", err)
	}
	return nil
}

type entraSession struct {
	directory       string
	keyPath         string
	certificatePath string
	username        string
	port            int
	tunnel          *exec.Cmd
	tunnelDone      <-chan error
}

func startEntraSession(route inventory.BastionRoute, vm inventory.VirtualMachine, purpose string) (*entraSession, error) {
	directory, err := os.MkdirTemp("", "azssh-"+purpose+"-")
	if err != nil {
		return nil, fmt.Errorf("create temporary %s directory: %w", purpose, err)
	}
	cleanup := func(err error) (*entraSession, error) {
		_ = os.RemoveAll(directory)
		return nil, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return cleanup(fmt.Errorf("secure temporary %s directory: %w", purpose, err))
	}
	keyPath := filepath.Join(directory, "identity")
	if output, err := exec.Command("ssh-keygen", "-q", "-t", "rsa", "-b", "4096", "-N", "", "-f", keyPath).CombinedOutput(); err != nil {
		return cleanup(fmt.Errorf("create temporary SSH key: %w: %s", err, strings.TrimSpace(string(output))))
	}
	certificatePath := filepath.Join(directory, "identity-cert.pub")
	if output, err := exec.Command("az", "ssh", "cert", "--public-key-file", keyPath+".pub", "--file", certificatePath).CombinedOutput(); err != nil {
		return cleanup(fmt.Errorf("request Entra SSH certificate: %w: %s", err, strings.TrimSpace(string(output))))
	}
	username, err := certificatePrincipal(certificatePath)
	if err != nil {
		return cleanup(err)
	}
	port, err := availableLoopbackPort()
	if err != nil {
		return cleanup(err)
	}
	tunnel := exec.Command("az", "network", "bastion", "tunnel",
		"--subscription", route.Bastion.SubscriptionID,
		"--name", route.Bastion.Name,
		"--resource-group", route.Bastion.ResourceGroup,
		"--target-resource-id", vm.ID,
		"--resource-port", "22",
		"--port", strconv.Itoa(port),
	)
	tunnel.Stdout, tunnel.Stderr = os.Stdout, os.Stderr
	if err := tunnel.Start(); err != nil {
		return cleanup(fmt.Errorf("start Bastion tunnel: %w", err))
	}
	tunnelDone := make(chan error, 1)
	go func() { tunnelDone <- tunnel.Wait() }()
	if err := waitForTunnel(port, tunnelDone); err != nil {
		stopTunnel(tunnel, tunnelDone)
		return cleanup(err)
	}
	return &entraSession{directory: directory, keyPath: keyPath, certificatePath: certificatePath, username: username, port: port, tunnel: tunnel, tunnelDone: tunnelDone}, nil
}

func (s *entraSession) Close() {
	stopTunnel(s.tunnel, s.tunnelDone)
	_ = os.RemoveAll(s.directory)
}

func certificatePrincipal(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Entra SSH certificate: %w", err)
	}
	publicKey, _, _, _, err := ssh.ParseAuthorizedKey(contents)
	if err != nil {
		return "", fmt.Errorf("parse Entra SSH certificate: %w", err)
	}
	certificate, ok := publicKey.(*ssh.Certificate)
	if !ok || len(certificate.ValidPrincipals) == 0 || strings.TrimSpace(certificate.ValidPrincipals[0]) == "" {
		return "", fmt.Errorf("Entra SSH certificate has no valid principal")
	}
	return certificate.ValidPrincipals[0], nil
}

func availableLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve local tunnel port: %w", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func waitForTunnel(port int, done <-chan error) error {
	deadline := time.Now().Add(transferTunnelTimeout)
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil {
			connection.Close()
			return nil
		}
		select {
		case err := <-done:
			return fmt.Errorf("Bastion tunnel exited before becoming ready: %w", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("Bastion tunnel did not become ready within %s", transferTunnelTimeout)
}

func stopTunnel(tunnel *exec.Cmd, done <-chan error) {
	select {
	case <-done:
		return
	default:
	}
	if tunnel.Process != nil {
		_ = tunnel.Process.Kill()
	}
	<-done
}

func transferRC(vmName, username, keyPath, certificatePath string, port int, route inventory.BastionRoute) string {
	return fmt.Sprintf(`
vm_alias=%s
printf 'Azure Bastion transfer shell\n\n'
printf 'VM: %%s\nBastion: %%s (%%s / %%s)\n\n' %s %s %s %s
printf 'Use: scp ./report.csv %%s:/home/%%s/\n' "$vm_alias" %s
printf '     scp %%s:/var/log/app.log .\n\n' "$vm_alias"

scp() {
  local argument flag flags index remote_count=0
  local -a arguments=()
  local options=1
  for argument in "$@"; do
    if (( options )); then
      case "$argument" in
        --) options=0; arguments+=("$argument"); continue ;;
        -*)
          flags="${argument#-}"
          for ((index = 0; index < ${#flags}; index++)); do
            flag="${flags:index:1}"
            case "$flag" in
              3|4|6|A|B|C|O|p|q|R|r|T|v) ;;
              *) printf 'azssh: scp connection override or unsupported option: %%s\n' "$argument" >&2; return 2 ;;
            esac
          done
          arguments+=("$argument"); continue ;;
      esac
    fi
    if [[ "$argument" == "$vm_alias:"* ]]; then
      ((remote_count++))
    elif [[ "$argument" == *:* ]]; then
      printf 'azssh: only %%s:<path> is permitted as a remote endpoint\n' "$vm_alias" >&2
      return 2
    fi
    arguments+=("$argument")
  done
  if (( remote_count != 1 )); then
    printf 'azssh: exactly one %%s:<path> endpoint is required\n' "$vm_alias" >&2
    return 2
  fi
  command scp -F /dev/null -o HostName=127.0.0.1 -o Port=%d -o User=%s -o IdentitiesOnly=yes -o IdentityFile=%s -o CertificateFile=%s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "${arguments[@]}"
}
`, quoteShell(vmName), quoteShell(vmName), quoteShell(route.Bastion.Name), quoteShell(route.Bastion.SubscriptionID), quoteShell(route.Bastion.ResourceGroup), quoteShell(username), port, quoteShell(username), quoteShell(keyPath), quoteShell(certificatePath))
}
