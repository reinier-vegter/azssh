package app

import (
	"context"
	"fmt"
	"time"

	"azssh/internal/cache"
	"azssh/internal/inventory"

	tea "charm.land/bubbletea/v2"
)

func (m Model) refreshCmd() tea.Cmd {
	client := m.client
	store := m.store
	cachedTopology := m.topology
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()

		topology := cachedTopology
		if !cache.IsTopologyFresh(topology, time.Now()) {
			subscriptions, err := client.ListSubscriptions(ctx)
			if err != nil {
				return refreshFailedMsg{err: fmt.Errorf("discover subscriptions: %w", err)}
			}
			if len(subscriptions) == 0 {
				return refreshFailedMsg{err: fmt.Errorf("no subscriptions are visible to the current Azure CLI identity")}
			}
			subscriptionIDs := make([]string, 0, len(subscriptions))
			for _, subscription := range subscriptions {
				subscriptionIDs = append(subscriptionIDs, subscription.ID)
			}
			bastions, err := client.ListBastions(ctx, subscriptionIDs)
			if err != nil {
				return refreshFailedMsg{err: err}
			}
			peerings, err := client.ListPeerings(ctx, subscriptionIDs)
			if err != nil {
				return refreshFailedMsg{err: err}
			}
			topology = cache.TopologySnapshot{
				FetchedAt: time.Now(), SubscriptionIDs: subscriptionIDs, Subscriptions: subscriptions,
				Bastions: bastions, Peerings: peerings,
			}
			if err := store.SaveTopology(topology); err != nil {
				return refreshFailedMsg{err: fmt.Errorf("save Bastion topology cache: %w", err)}
			}
		}

		vms, err := client.ListVMs(ctx, topology.SubscriptionIDs)
		if err != nil {
			return refreshFailedMsg{err: err}
		}
		targets := inventory.ResolveEligibleTargets(topology.Bastions, topology.Peerings, vms)
		vmInventory := cache.VMInventorySnapshot{
			FetchedAt: time.Now(), SubscriptionIDs: topology.SubscriptionIDs, Targets: targets,
		}
		if err := store.SaveVMInventory(vmInventory); err != nil {
			return refreshFailedMsg{err: fmt.Errorf("save VM inventory cache: %w", err)}
		}
		return refreshSucceededMsg{topology: topology, vmInventory: vmInventory}
	}
}
