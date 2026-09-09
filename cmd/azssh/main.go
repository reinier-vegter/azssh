package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"

	"azssh/internal/app"
	"azssh/internal/azure"
	"azssh/internal/cache"
	"azssh/internal/shell"

	tea "charm.land/bubbletea/v2"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	authType := flag.String("auth-type", "AAD", "Bastion SSH authentication type (AAD, ssh-key, or password)")
	username := flag.String("username", "", "SSH username for ssh-key or password authentication")
	sshKey := flag.String("ssh-key", "", "SSH private key path for ssh-key authentication")
	noAltScreen := flag.Bool("no-alt-screen", false, "render in the current terminal screen")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	account, err := azure.ActiveAccount()
	if err != nil {
		exitf("Azure CLI authentication is unavailable; run az login: %v", err)
	}
	store, err := cache.NewStore(cacheNamespace(account))
	if err != nil {
		exitf("could not initialize local cache: %v", err)
	}
	client, err := azure.NewClient()
	if err != nil {
		exitf("Azure CLI authentication is unavailable; run az login: %v", err)
	}

	topology, _ := store.LoadTopology()
	vmInventory, _ := store.LoadVMInventory()
	preferences, _ := store.LoadPreferences()
	model := app.NewModel(client, store, app.Config{Authentication: shell.Authentication{
		Type: *authType, Username: *username, SSHKeyPath: *sshKey,
	}, AltScreen: !*noAltScreen}, topology, vmInventory, preferences)
	finalModel, err := tea.NewProgram(model).Run()
	if err != nil {
		exitf("TUI failed: %v", err)
	}
	if final, ok := finalModel.(app.Model); ok {
		if command := final.ReviewCommand(); len(command) > 0 {
			if err := shell.ReplaceWithReviewShell(command); err != nil {
				exitf("could not open the command review shell: %v", err)
			}
		}
	}
}

func cacheNamespace(account azure.Account) string {
	sum := sha256.Sum256([]byte(account.Cloud + "\x00" + account.TenantID + "\x00" + account.Principal))
	return fmt.Sprintf("account-%x", sum[:12])
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "azssh: "+format+"\n", args...)
	os.Exit(1)
}
