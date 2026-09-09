package app

import (
	"azssh/internal/cache"
	"azssh/internal/inventory"
)

type refreshSucceededMsg struct {
	topology    cache.TopologySnapshot
	vmInventory cache.VMInventorySnapshot
}

type refreshFailedMsg struct {
	err error
}

type shellFinishedMsg struct {
	err error
}

type targetsLoadedMsg struct {
	targets []inventory.EligibleTarget
}
