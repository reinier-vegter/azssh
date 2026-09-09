// Package inventory contains the application-level Azure resource inventory.
package inventory

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

// VirtualMachine contains only the VM fields needed to resolve and launch a
// Bastion connection.
type VirtualMachine struct {
	ID             string
	Name           string
	ResourceGroup  string
	SubscriptionID string
	OSType         string
	VNetIDs        []string
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
