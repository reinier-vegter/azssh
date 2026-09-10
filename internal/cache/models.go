// Package cache persists local Azure inventory snapshots and user preferences.
package cache

import (
	"time"

	"azssh/internal/inventory"
)

const schemaVersion = 1

// TopologySnapshot is the cached Azure network topology for an account.
type TopologySnapshot struct {
	FetchedAt       time.Time
	SubscriptionIDs []string
	Subscriptions   []inventory.Subscription
	Bastions        []inventory.Bastion
	Peerings        []inventory.VNetPeering
}

// VMInventorySnapshot is the cached set of eligible VM targets for an account.
type VMInventorySnapshot struct {
	FetchedAt       time.Time
	SubscriptionIDs []string
	Targets         []inventory.EligibleTarget
}

// UpdateCheck records the most recent GitHub release lookup shared by all
// Azure account namespaces on this machine.
type UpdateCheck struct {
	CheckedAt     time.Time `json:"checkedAt"`
	LatestVersion string    `json:"latestVersion"`
}

type topologyEnvelope struct {
	SchemaVersion int              `json:"schemaVersion"`
	Data          TopologySnapshot `json:"data"`
}

type vmInventoryEnvelope struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Data          VMInventorySnapshot `json:"data"`
}

type updateCheckEnvelope struct {
	SchemaVersion int         `json:"schemaVersion"`
	Data          UpdateCheck `json:"data"`
}
