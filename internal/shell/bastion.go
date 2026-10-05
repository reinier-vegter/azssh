// Package shell builds interactive Azure Bastion shell commands.
package shell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"azssh/internal/inventory"
)

// Authentication contains non-secret Azure Bastion SSH options.
type Authentication struct {
	Type       string
	Username   string
	SSHKeyPath string
}

// Command builds an az network bastion ssh process for one selected route.
func Command(route inventory.BastionRoute, vm inventory.VirtualMachine, authentication Authentication) (*exec.Cmd, error) {
	authType := strings.TrimSpace(authentication.Type)
	if authType == "" {
		authType = "AAD"
	}
	arguments := []string{
		"network", "bastion", "ssh",
		"--subscription", route.Bastion.SubscriptionID,
		"--name", route.Bastion.Name,
		"--resource-group", route.Bastion.ResourceGroup,
		"--target-resource-id", vm.ID,
		"--auth-type", authType,
	}
	if username := strings.TrimSpace(authentication.Username); username != "" {
		arguments = append(arguments, "--username", username)
	}
	if keyPath := strings.TrimSpace(authentication.SSHKeyPath); keyPath != "" {
		arguments = append(arguments, "--ssh-key", keyPath)
	}
	if !strings.EqualFold(authType, "AAD") && strings.TrimSpace(authentication.Username) == "" {
		return nil, fmt.Errorf("--username is required for %s authentication", authType)
	}
	if strings.EqualFold(authType, "ssh-key") && strings.TrimSpace(authentication.SSHKeyPath) == "" {
		return nil, fmt.Errorf("--ssh-key is required for ssh-key authentication")
	}
	return exec.Command("az", arguments...), nil
}

// ReviewCommand opens the user's interactive shell, shows the command exactly
// as it will run, and waits for their confirmation before executing it.
func ReviewCommand(command *exec.Cmd) *exec.Cmd {
	shellPath := strings.TrimSpace(os.Getenv("SHELL"))
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	display := quoteCommand(command.Args)
	var script string
	arguments := []string{"-ic"}
	if filepath.Base(shellPath) == "fish" {
		display = formatCommand(command.Args, quoteFish)
		script = fmt.Sprintf("printf 'Review this command:\\n\\n%%s\\n\\n' %s; read -P 'Press Enter to run it (Ctrl-C to cancel): '; command $argv", quoteFish(display))
		arguments = append(arguments, script)
	} else {
		script = fmt.Sprintf("printf 'Review this command:\\n\\n%%s\\n\\n' %s; printf 'Press Enter to run it (Ctrl-C to cancel): '; IFS= read -r _; exec \"$@\"", quoteShell(display))
		arguments = append(arguments, script, "azssh-review")
	}
	arguments = append(arguments, command.Args...)
	return exec.Command(shellPath, arguments...)
}

// ReplaceWithReviewShell replaces the current process with the user's shell.
// The shell displays the command and waits for Enter, so azssh itself has
// already stopped before the user confirms execution.
func ReplaceWithReviewShell(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("cannot review an empty command")
	}
	review := ReviewCommand(exec.Command(arguments[0], arguments[1:]...))
	path, err := exec.LookPath(review.Path)
	if err != nil {
		return fmt.Errorf("resolve review shell: %w", err)
	}
	return syscall.Exec(path, review.Args, os.Environ())
}

func quoteCommand(arguments []string) string {
	return formatCommand(arguments, quoteShell)
}

func formatCommand(arguments []string, quote func(string) string) string {
	quoted := make([]string, len(arguments))
	commandWords, expectValue := true, false
	for index, argument := range arguments {
		switch {
		case expectValue:
			quoted[index], expectValue = quote(argument), false
		case strings.HasPrefix(argument, "--") && literalShellWord(argument):
			quoted[index], commandWords, expectValue = argument, false, true
		case commandWords && literalShellWord(argument):
			quoted[index] = argument
		default:
			quoted[index], commandWords = quote(argument), false
		}
	}
	return strings.Join(quoted, " ")
}

// Only command/flag tokens with no shell syntax can be displayed bare.
func literalShellWord(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("_-/.", char) {
			continue
		}
		return false
	}
	return true
}

func quoteFish(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return "'" + strings.ReplaceAll(value, "'", "\\'") + "'"
}

func quoteShell(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
