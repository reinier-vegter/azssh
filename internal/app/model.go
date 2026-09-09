// Package app contains the Bubble Tea application model and views.
package app

import (
	"sort"
	"strings"
	"time"

	"azssh/internal/azure"
	"azssh/internal/cache"
	"azssh/internal/inventory"
	"azssh/internal/shell"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
)

type view int

const (
	mainView view = iota
	subscriptionFilterView
	routeSelectorView
	helpView
)

// Config supplies the process options required for an interactive Bastion SSH
// command. Passwords are intentionally not represented here.
type Config struct {
	Authentication shell.Authentication
	AltScreen      bool
}

// Model is the root Bubble Tea application state.
type Model struct {
	client *azure.Client
	store  *cache.Store
	config Config
	keys   keyMap

	activeView   view
	previousView view
	width        int
	height       int

	topology              cache.TopologySnapshot
	targets               []inventory.EligibleTarget
	subscriptions         []inventory.Subscription
	hiddenSubscriptionIDs map[string]bool

	vmList      list.Model
	spinner     spinner.Model
	loading     bool
	status      string
	lastRefresh time.Time

	filterIndex int
	filterDraft map[string]bool

	routeTarget *inventory.EligibleTarget
	routeIndex  int

	reviewCommand []string
}

// ReviewCommand returns the requested Bastion command after the TUI exits.
func (m Model) ReviewCommand() []string {
	return append([]string(nil), m.reviewCommand...)
}

// NewModel starts from local cache data and schedules its refresh from Init.
func NewModel(client *azure.Client, store *cache.Store, config Config, topology cache.TopologySnapshot, vmInventory cache.VMInventorySnapshot, preferences cache.Preferences) Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetHeight(2)
	delegate.SetSpacing(0)
	vmList := list.New(nil, delegate, 48, 16)
	vmList.Title = "Eligible VMs"
	vmList.SetShowHelp(false)
	vmList.SetShowPagination(false)
	vmList.SetStatusBarItemName("VM", "VMs")
	vmList.FilterInput.Prompt = "Search > "
	vmList.FilterInput.Placeholder = "VM, subscription, resource group, Bastion"
	vmList.DisableQuitKeybindings()

	m := Model{
		client: client, store: store, config: config, keys: defaultKeyMap(),
		activeView: mainView, topology: topology, targets: vmInventory.Targets,
		subscriptions:         topology.Subscriptions,
		hiddenSubscriptionIDs: hiddenSet(preferences.HiddenSubscriptionIDs),
		vmList:                vmList,
		spinner:               spinner.New(spinner.WithSpinner(spinner.Dot)),
		loading:               true,
		lastRefresh:           vmInventory.FetchedAt,
	}
	sort.Slice(m.subscriptions, func(i, j int) bool {
		return strings.ToLower(m.subscriptions[i].Name) < strings.ToLower(m.subscriptions[j].Name)
	})
	m.rebuildList()
	return m
}

func hiddenSet(ids []string) map[string]bool {
	hidden := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			hidden[id] = true
		}
	}
	return hidden
}

func (m *Model) rebuildList() {
	filtered := inventory.FilterTargets(m.targets, m.hiddenSubscriptionIDs, "")
	items := make([]list.Item, 0, len(filtered))
	for _, target := range filtered {
		items = append(items, targetItem{target: target, subscriptionName: m.subscriptionName(target.VM.SubscriptionID)})
	}
	_ = m.vmList.SetItems(items)
}

func (m Model) subscriptionName(id string) string {
	for _, subscription := range m.subscriptions {
		if strings.EqualFold(subscription.ID, id) {
			return subscription.Name
		}
	}
	return id
}

func (m *Model) openSubscriptionFilter() {
	m.filterDraft = make(map[string]bool, len(m.subscriptions))
	for id, hidden := range m.hiddenSubscriptionIDs {
		m.filterDraft[id] = hidden
	}
	m.filterIndex = 0
	m.activeView = subscriptionFilterView
}

func (m *Model) applySubscriptionFilter() error {
	m.hiddenSubscriptionIDs = m.filterDraft
	ids := make([]string, 0, len(m.hiddenSubscriptionIDs))
	for id, hidden := range m.hiddenSubscriptionIDs {
		if hidden {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if err := m.store.SavePreferences(cache.Preferences{HiddenSubscriptionIDs: ids}); err != nil {
		return err
	}
	m.rebuildList()
	m.vmList.ResetSelected()
	m.activeView = mainView
	m.status = "Subscription filter applied"
	return nil
}

func (m *Model) selectedTarget() *inventory.EligibleTarget {
	item, ok := m.vmList.SelectedItem().(targetItem)
	if !ok {
		return nil
	}
	target := item.target
	return &target
}

type targetItem struct {
	target           inventory.EligibleTarget
	subscriptionName string
}

func (i targetItem) Title() string { return i.target.VM.Name }

func (i targetItem) Description() string {
	route := i.target.Routes[0]
	return i.subscriptionName + " / " + i.target.VM.ResourceGroup + "  via " + route.Bastion.Name
}

func (i targetItem) FilterValue() string {
	parts := []string{i.target.VM.Name, i.subscriptionName, i.target.VM.ResourceGroup, i.target.VM.SubscriptionID}
	for _, route := range i.target.Routes {
		parts = append(parts, route.Bastion.Name, route.Bastion.ResourceGroup)
	}
	return strings.Join(parts, " ")
}
