// Package inventory contains the application-level Azure resource inventory.
package inventory

import (
	"strings"
	"unicode"
)

var displayTagTerms = []string{"env", "environment", "owner", "team", "cost", "project", "service", "application", "workload", "department"}

// Subscription is an Azure subscription visible to the signed-in user.
type Subscription struct {
	ID   string
	Name string
}

// Bastion is a native-client-capable Azure Bastion host.
type Bastion struct {
	ID             string
	Name           string
	ResourceGroup  string
	SubscriptionID string
	VNetID         string
}

// VNetPeering is a directed connected VNet peering which allows virtual
// network access.
type VNetPeering struct {
	LocalVNetID  string
	RemoteVNetID string
}

// VirtualMachine contains the VM fields needed to resolve a Bastion connection
// and verify the selected target in the TUI.
type VirtualMachine struct {
	ID                string
	Name              string
	ResourceGroup     string
	SubscriptionID    string
	OSType            string
	Location          string
	Size              string
	VNetIDs           []string
	SubnetIDs         []string
	PrivateIPs        []string
	OSDiskSizeGB      int
	OSDiskStorageType string
	DataDiskCount     int
	Tags              map[string]string
}

// DisplayTags returns the supported, nonempty tags to retain and show.
func DisplayTags(tags map[string]string) map[string]string {
	result := make(map[string]string)
	for key, value := range tags {
		if key == "" || value == "" || hasControlCharacter(key) || hasControlCharacter(value) || !matchesDisplayTag(key) {
			continue
		}
		result[key] = value
	}
	return result
}

func hasControlCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func matchesDisplayTag(key string) bool {
	key = strings.ToLower(key)
	for _, term := range displayTagTerms {
		if strings.Contains(key, term) {
			return true
		}
	}
	return false
}

// BastionRoute describes a Bastion path to a VM.
type BastionRoute struct {
	Bastion   Bastion
	RouteType string
}

// EligibleTarget is a VM with at least one usable Bastion route.
type EligibleTarget struct {
	VM     VirtualMachine
	Routes []BastionRoute
}
