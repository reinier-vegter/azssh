// Package app contains the Bubble Tea application model and views.
package app

import (
	"os"
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
	Version        string
	UpdateStore    *cache.Store
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
	favoriteVMIDs         map[string]bool

	vmList          list.Model
	spinner         spinner.Model
	loading         bool
	status          string
	lastRefresh     time.Time
	useUnicode      bool
	availableUpdate string

	filterIndex int
	filterDraft map[string]bool

	routeTarget   *inventory.EligibleTarget
	routeIndex    int
	routeTransfer bool

	reviewCommand   []string
	transferRequest *transferRequest
}

type transferRequest struct {
	route inventory.BastionRoute
	vm    inventory.VirtualMachine
}

// ReviewCommand returns the requested Bastion command after the TUI exits.
func (m Model) ReviewCommand() []string {
	return append([]string(nil), m.reviewCommand...)
}

// TransferRequest returns the selected Entra transfer route after the TUI exits.
func (m Model) TransferRequest() (inventory.BastionRoute, inventory.VirtualMachine, bool) {
	if m.transferRequest == nil {
		return inventory.BastionRoute{}, inventory.VirtualMachine{}, false
	}
	return m.transferRequest.route, m.transferRequest.vm, true
}

// NewModel starts from local cache data and schedules its refresh from Init.
func NewModel(client *azure.Client, store *cache.Store, config Config, topology cache.TopologySnapshot, vmInventory cache.VMInventorySnapshot, preferences cache.Preferences) Model {
	useUnicode := unicodeFromEnv(os.Getenv)
	vmList := list.New(nil, newTargetDelegate(useUnicode), 48, 16)
	vmList.Title = "Eligible VMs"
	vmList.SetShowHelp(false)
	vmList.SetShowPagination(false)
	vmList.SetStatusBarItemName("VM", "VMs")
	vmList.FilterInput.Prompt = "Search > "
	vmList.FilterInput.Placeholder = "VM, subscription, resource group, Bastion"
	vmList.Filter = allTermsFilter
	vmList.DisableQuitKeybindings()

	m := Model{
		client: client, store: store, config: config, keys: defaultKeyMap(),
		activeView: mainView, topology: topology, targets: vmInventory.Targets,
		subscriptions:         topology.Subscriptions,
		hiddenSubscriptionIDs: hiddenSet(preferences.HiddenSubscriptionIDs),
		favoriteVMIDs:         favoriteSet(preferences.FavoriteVMIDs),
		vmList:                vmList,
		spinner:               spinner.New(spinner.WithSpinner(spinner.Dot)),
		loading:               true,
		lastRefresh:           vmInventory.FetchedAt,
		useUnicode:            useUnicode,
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

func favoriteSet(ids []string) map[string]bool {
	favorites := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			favorites[id] = true
		}
	}
	return favorites
}

func (m *Model) rebuildList() {
	filtered := inventory.FilterTargets(m.targets, m.hiddenSubscriptionIDs, "")
	sort.SliceStable(filtered, func(i, j int) bool {
		return m.favoriteVMIDs[normalizedID(filtered[i].VM.ID)] && !m.favoriteVMIDs[normalizedID(filtered[j].VM.ID)]
	})
	items := make([]list.Item, 0, len(filtered))
	for _, target := range filtered {
		items = append(items, targetItem{target: target, subscriptionName: m.subscriptionName(target.VM.SubscriptionID), favorite: m.favoriteVMIDs[normalizedID(target.VM.ID)], useUnicode: m.useUnicode})
	}
	if cmd := m.vmList.SetItems(items); cmd != nil {
		m.vmList, _ = m.vmList.Update(cmd())
	}
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
	if err := m.store.SavePreferences(cache.Preferences{HiddenSubscriptionIDs: ids, FavoriteVMIDs: m.favoriteIDs()}); err != nil {
		return err
	}
	m.rebuildList()
	m.vmList.ResetSelected()
	m.activeView = mainView
	m.status = "Subscription filter applied"
	return nil
}

func (m *Model) toggleFavorite() error {
	target := m.selectedTarget()
	if target == nil {
		return nil
	}
	id := normalizedID(target.VM.ID)
	if id == "" {
		return nil
	}
	wasFavorite := m.favoriteVMIDs[id]
	if wasFavorite {
		delete(m.favoriteVMIDs, id)
		m.status = "Removed " + target.VM.Name + " from favorites"
	} else {
		m.favoriteVMIDs[id] = true
		m.status = "Added " + target.VM.Name + " to favorites"
	}
	if err := m.store.SavePreferences(cache.Preferences{HiddenSubscriptionIDs: m.hiddenIDs(), FavoriteVMIDs: m.favoriteIDs()}); err != nil {
		if wasFavorite {
			m.favoriteVMIDs[id] = true
		} else {
			delete(m.favoriteVMIDs, id)
		}
		return err
	}
	m.rebuildList()
	return nil
}

func (m Model) hiddenIDs() []string {
	return enabledIDs(m.hiddenSubscriptionIDs)
}

func (m Model) favoriteIDs() []string {
	return enabledIDs(m.favoriteVMIDs)
}

func enabledIDs(ids map[string]bool) []string {
	result := make([]string, 0, len(ids))
	for id, enabled := range ids {
		if enabled {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func normalizedID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func unicodeFromEnv(getenv func(string) string) bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		locale := getenv(name)
		if locale == "" {
			continue
		}
		locale = strings.ToLower(locale)
		return strings.Contains(locale, ".utf-8") || strings.Contains(locale, ".utf8")
	}
	return false
}

func allTermsFilter(term string, targets []string) []list.Rank {
	terms := strings.Fields(strings.ToLower(term))
	ranks := make([]list.Rank, 0, len(targets))
	for index, target := range targets {
		target = strings.ToLower(target)
		matches := true
		for _, term := range terms {
			if !strings.Contains(target, term) {
				matches = false
				break
			}
		}
		if matches {
			ranks = append(ranks, list.Rank{Index: index})
		}
	}
	return ranks
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
	favorite         bool
	useUnicode       bool
}

func (i targetItem) Title() string {
	if i.favorite {
		return favoriteMarker(i.useUnicode) + " " + i.target.VM.Name
	}
	return i.target.VM.Name
}

func (i targetItem) Description() string {
	route := i.target.Routes[0]
	return i.subscriptionName + " / " + i.target.VM.ResourceGroup + "  via " + route.Bastion.Name
}

func (i targetItem) FilterValue() string {
	parts := []string{i.target.VM.Name, i.subscriptionName, i.target.VM.ResourceGroup, i.target.VM.SubscriptionID}
	for _, value := range inventory.DisplayTags(i.target.VM.Tags) {
		parts = append(parts, value)
	}
	for _, route := range i.target.Routes {
		parts = append(parts, route.Bastion.Name, route.Bastion.ResourceGroup)
	}
	return strings.Join(parts, " ")
}
