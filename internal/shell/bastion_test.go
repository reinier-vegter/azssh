package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"azssh/internal/inventory"
)

func TestCommandForAAD(t *testing.T) {
	command, err := Command(inventory.BastionRoute{Bastion: inventory.Bastion{
		Name: "bastion", ResourceGroup: "network", SubscriptionID: "bastion-sub",
	}}, inventory.VirtualMachine{ID: "/subscriptions/vm-sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm"}, Authentication{})
	if err != nil {
		t.Fatal(err)
	}
	arguments := strings.Join(command.Args, " ")
	for _, expected := range []string{"network bastion ssh", "--subscription bastion-sub", "--name bastion", "--resource-group network", "--auth-type AAD"} {
		if !strings.Contains(arguments, expected) {
			t.Errorf("command %q does not contain %q", arguments, expected)
		}
	}
}

func TestCommandRequiresCredentialsForSSHKey(t *testing.T) {
	_, err := Command(inventory.BastionRoute{}, inventory.VirtualMachine{}, Authentication{Type: "ssh-key", Username: "user"})
	if err == nil || !strings.Contains(err.Error(), "--ssh-key") {
		t.Fatalf("expected ssh key validation error, got %v", err)
	}
}

func TestReviewCommandPreservesAzureArguments(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	review := ReviewCommand(exec.Command("az", "network", "bastion", "ssh", "--name", "bastion name"))
	if review.Args[0] != "/bin/sh" || review.Args[1] != "-ic" {
		t.Fatalf("unexpected review shell arguments: %#v", review.Args)
	}
	if !strings.Contains(review.Args[2], "exec \"$@\"") {
		t.Fatalf("review script does not execute the preserved argument vector: %q", review.Args[2])
	}
	if got := review.Args[len(review.Args)-1]; got != "bastion name" {
		t.Fatalf("review command lost an argument: %#v", review.Args)
	}
}

func TestQuoteCommand(t *testing.T) {
	if got, want := quoteCommand([]string{"az", "bastion name", "contains'quote"}), "az 'bastion name' 'contains'\\''quote'"; got != want {
		t.Fatalf("quoteCommand() = %q, want %q", got, want)
	}
}

func TestCopyableCommandQuotesValuesOnly(t *testing.T) {
	arguments := []string{"az", "network", "bastion", "ssh", "--subscription", "sub-id", "--name", "bastion name", "--auth-type", "AAD", "--username", "--looks-like-a-flag", "--ssh-key", ""}
	want := "az network bastion ssh --subscription 'sub-id' --name 'bastion name' --auth-type 'AAD' --username '--looks-like-a-flag' --ssh-key ''"
	if got := quoteCommand(arguments); got != want {
		t.Fatalf("display=%q, want %q", got, want)
	}
}

func TestCopyableCommandRoundTripsValues(t *testing.T) {
	arguments := []string{"printf", "--format", "name with spaces", "--quote", "O'Brien", "--path", `C:\folder\file`, "--syntax", "$(echo unsafe); * & |", "--empty", "", "--multiline", "line one\nline two"}
	// Parse the displayed command as shell words without executing it.
	parse := exec.Command("/bin/sh", "-c", "set -- "+quoteCommand(arguments)+"; printf '%s\\000' \"$@\"")
	output, err := parse.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00"); !slices.Equal(got, arguments) {
		t.Fatalf("parsed=%q, want %q", got, arguments)
	}
}

func TestFishCommandQuotesValuesForFish(t *testing.T) {
	arguments := []string{"az", "network", "bastion", "ssh", "--username", "O'Brien", "--ssh-key", `folder\file`}
	want := `az network bastion ssh --username 'O\'Brien' --ssh-key 'folder\\file'`
	if got := formatCommand(arguments, quoteFish); got != want {
		t.Fatalf("fish display=%q, want %q", got, want)
	}
}

func TestReviewOutputHasNoIndentationAndExecutesOriginalArguments(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "az")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf 'EXECUTED:'\nprintf '<%s>' \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	values := []string{"network", "bastion", "ssh", "--name", "O'Brien", "--ssh-key", `folder\file`, "--username", "$(not-executed)"}
	for _, shell := range []string{"/bin/sh", "bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			path, err := exec.LookPath(shell)
			if err != nil {
				t.Skip("shell not installed")
			}
			t.Setenv("SHELL", path)
			review := ReviewCommand(exec.Command(stub, values...))
			// Do not load interactive profiles in this deterministic test.
			review.Args[1] = "-c"
			review.Stdin = strings.NewReader("\n")
			output, err := review.CombinedOutput()
			if err != nil {
				t.Fatalf("review: %v\n%s", err, output)
			}
			quote := quoteShell
			if filepath.Base(path) == "fish" {
				quote = quoteFish
			}
			expected := "Review this command:\n\n" + formatCommand(append([]string{stub}, values...), quote) + "\n\n"
			if !strings.HasPrefix(string(output), expected) {
				t.Fatalf("unexpected review output: %q", output)
			}
			if !strings.Contains(string(output), "EXECUTED:<network><bastion><ssh><--name><O'Brien><--ssh-key><folder\\file><--username><$(not-executed)>") {
				t.Fatalf("arguments not preserved: %q", output)
			}
		})
	}
}

func TestReviewCommandPassesFishArgumentsWithoutPlaceholder(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	review := ReviewCommand(exec.Command("az", "network", "bastion", "ssh"))
	if got := review.Args[3:]; !slices.Equal(got, []string{"az", "network", "bastion", "ssh"}) {
		t.Fatalf("fish review command arguments = %#v", got)
	}
}
