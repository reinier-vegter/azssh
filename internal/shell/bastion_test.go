package shell

import (
	"os/exec"
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
	if got, want := quoteCommand([]string{"az", "bastion name", "contains'quote"}), "'az' 'bastion name' 'contains'\\''quote'"; got != want {
		t.Fatalf("quoteCommand() = %q, want %q", got, want)
	}
}

func TestReviewCommandPassesFishArgumentsWithoutPlaceholder(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	review := ReviewCommand(exec.Command("az", "network", "bastion", "ssh"))
	if got := review.Args[3:]; !slices.Equal(got, []string{"az", "network", "bastion", "ssh"}) {
		t.Fatalf("fish review command arguments = %#v", got)
	}
}
