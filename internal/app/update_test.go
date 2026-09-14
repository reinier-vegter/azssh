package app

import (
	"strings"
	"testing"

	"azssh/internal/cache"
	"azssh/internal/inventory"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

func TestSubscriptionFilterTogglesWithSpace(t *testing.T) {
	model := Model{
		keys:          defaultKeyMap(),
		subscriptions: []inventory.Subscription{{ID: "subscription-a", Name: "Subscription A"}},
		filterDraft:   map[string]bool{},
	}

	updated, _ := model.updateSubscriptionFilter(tea.KeyPressMsg(tea.Key{Code: tea.KeySpace}))
	result := updated.(Model)
	if !result.filterDraft["subscription-a"] {
		t.Fatal("space should hide the selected subscription")
	}
}

func TestTransferRequestsSelectedRouteAndRequiresAAD(t *testing.T) {
	target := inventory.EligibleTarget{VM: inventory.VirtualMachine{ID: "vm-1", Name: "api-01", OSType: "Linux"}, Routes: []inventory.BastionRoute{
		{Bastion: inventory.Bastion{Name: "preferred"}},
		{Bastion: inventory.Bastion{Name: "alternate"}},
	}}
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{Targets: []inventory.EligibleTarget{target}}, cache.Preferences{})
	updated, _ := model.updateKey(tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
	result := updated.(Model)
	if result.activeView != routeSelectorView || result.routeAction != routeTransfer {
		t.Fatalf("transfer did not open route selector: %#v", result)
	}
	updated, _ = result.updateRouteSelector(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	route, vm, requested := updated.(Model).TransferRequest()
	if !requested || route.Bastion.Name != "preferred" || vm.Name != "api-01" {
		t.Fatalf("transfer request = %#v, %#v, %v", route, vm, requested)
	}

	model.config.Authentication.Type = "ssh-key"
	updated, _ = model.updateKey(tea.KeyPressMsg(tea.Key{Code: 't', Text: "t"}))
	result = updated.(Model)
	if !strings.Contains(result.status, "requires AAD") || result.activeView != mainView {
		t.Fatalf("non-AAD transfer result = status %q, view %v", result.status, result.activeView)
	}
}

func TestMountPromptsForPathAndRequiresAAD(t *testing.T) {
	target := inventory.EligibleTarget{VM: inventory.VirtualMachine{ID: "vm-1", Name: "api-01", OSType: "Linux"}, Routes: []inventory.BastionRoute{
		{Bastion: inventory.Bastion{Name: "preferred"}},
		{Bastion: inventory.Bastion{Name: "alternate"}},
	}}
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{Targets: []inventory.EligibleTarget{target}}, cache.Preferences{})
	updated, _ := model.updateKey(tea.KeyPressMsg(tea.Key{Code: 'm', Text: "m"}))
	result := updated.(Model)
	if result.activeView != routeSelectorView || result.routeAction != routeMount {
		t.Fatalf("mount did not open route selector: %#v", result)
	}
	updated, _ = result.updateRouteSelector(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	result = updated.(Model)
	if result.activeView != mountPathView || result.mountPath.Value() != "." {
		t.Fatalf("mount path prompt = view %v, path %q", result.activeView, result.mountPath.Value())
	}
	result.mountPath.SetValue("/srv/data")
	updated, _ = result.updateMountPath(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	route, vm, remotePath, requested := updated.(Model).MountRequest()
	if !requested || route.Bastion.Name != "preferred" || vm.Name != "api-01" || remotePath != "/srv/data" {
		t.Fatalf("mount request = %#v, %#v, %q, %v", route, vm, remotePath, requested)
	}

	model.config.Authentication.Type = "ssh-key"
	updated, _ = model.updateKey(tea.KeyPressMsg(tea.Key{Code: 'm', Text: "m"}))
	result = updated.(Model)
	if !strings.Contains(result.status, "requires AAD") || result.activeView != mainView {
		t.Fatalf("non-AAD mount result = status %q, view %v", result.status, result.activeView)
	}
}

func TestMountPathCancelHasNoRequest(t *testing.T) {
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{}, cache.Preferences{})
	model.openMountPath(inventory.BastionRoute{}, inventory.VirtualMachine{Name: "api-01"})
	updated, _ := model.updateMountPath(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	result := updated.(Model)
	if result.activeView != mainView || result.mountRequest != nil {
		t.Fatalf("cancelled mount = view %v, request %#v", result.activeView, result.mountRequest)
	}
}

func TestToggleFavoritePersistsAndPrioritizesVM(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.NewStore("test-account")
	if err != nil {
		t.Fatal(err)
	}
	model := NewModel(nil, store, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{Targets: []inventory.EligibleTarget{
		{VM: inventory.VirtualMachine{ID: "vm-one", Name: "one", OSType: "Linux"}, Routes: []inventory.BastionRoute{{}}},
		{VM: inventory.VirtualMachine{ID: "vm-two", Name: "two", OSType: "Linux"}, Routes: []inventory.BastionRoute{{}}},
	}}, cache.Preferences{})
	model.useUnicode = false
	model.vmList.Select(1)

	updated, _ := model.updateKey(tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"}))
	result := updated.(Model)
	if !result.favoriteVMIDs["vm-two"] {
		t.Fatal("x should favorite the selected VM")
	}
	if item := result.vmList.Items()[0].(targetItem); item.target.VM.ID != "vm-two" || item.Title() != favoriteMarker(false)+" two" {
		t.Fatalf("favorite should be marked and listed first: %#v", item)
	}
	preferences, err := store.LoadPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if len(preferences.FavoriteVMIDs) != 1 || preferences.FavoriteVMIDs[0] != "vm-two" {
		t.Fatalf("saved favorites = %#v", preferences.FavoriteVMIDs)
	}
}

func TestAllTermsFilter(t *testing.T) {
	ranks := allTermsFilter("prod api", []string{"api production", "api test", "production worker"})
	if len(ranks) != 1 || ranks[0].Index != 0 {
		t.Fatalf("all terms filter ranks = %#v", ranks)
	}
}

func TestTargetFilterValueIncludesResourceGroupAndTagValues(t *testing.T) {
	item := targetItem{target: inventory.EligibleTarget{
		VM: inventory.VirtualMachine{
			Name: "api-01", ResourceGroup: "payments-prod-rg",
			Tags: map[string]string{"Environment": "production", "Ignore": "not searchable"},
		},
		Routes: []inventory.BastionRoute{{}},
	}, subscriptionName: "Production"}

	for _, term := range []string{"payments-prod-rg", "production", "payments-prod-rg production"} {
		if ranks := allTermsFilter(term, []string{item.FilterValue()}); len(ranks) != 1 {
			t.Fatalf("search for %q ranks = %#v", term, ranks)
		}
	}
	for _, term := range []string{"not searchable", "environment", "environment:production"} {
		if ranks := allTermsFilter(term, []string{item.FilterValue()}); len(ranks) != 0 {
			t.Fatalf("search for %q ranks = %#v", term, ranks)
		}
	}
}

func TestTargetDescriptionDoesNotIncludeDetailMetadata(t *testing.T) {
	item := targetItem{target: inventory.EligibleTarget{
		VM:     inventory.VirtualMachine{ResourceGroup: "platform-rg", Location: "westeurope", Size: "Standard_D4s_v5", PrivateIPs: []string{"10.0.0.4"}},
		Routes: []inventory.BastionRoute{{Bastion: inventory.Bastion{Name: "bastion-prod"}}},
	}, subscriptionName: "Production"}
	if got, want := item.Description(), "Production / platform-rg  via bastion-prod"; got != want {
		t.Fatalf("Description() = %q, want %q", got, want)
	}
}

func TestRefreshStatusReportsVisibleCountAndActiveRefresh(t *testing.T) {
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{}, cache.Preferences{HiddenSubscriptionIDs: []string{"sub-2"}})
	updated, _ := model.Update(refreshSucceededMsg{
		topology: cache.TopologySnapshot{Subscriptions: []inventory.Subscription{{ID: "sub-1", Name: "One"}, {ID: "sub-2", Name: "Two"}}},
		vmInventory: cache.VMInventorySnapshot{Targets: []inventory.EligibleTarget{
			{VM: inventory.VirtualMachine{ID: "vm-1", OSType: "Linux", SubscriptionID: "sub-1"}, Routes: []inventory.BastionRoute{{}}},
			{VM: inventory.VirtualMachine{ID: "vm-2", OSType: "Linux", SubscriptionID: "sub-2"}, Routes: []inventory.BastionRoute{{}}},
		}},
	})
	result := updated.(Model)
	if result.status != "Loaded 2 eligible VMs (1 shown)" {
		t.Fatalf("refresh status = %q", result.status)
	}
	result.loading = true
	updated, _ = result.updateKey(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	if got := updated.(Model).status; got != "Refresh already in progress" {
		t.Fatalf("active refresh status = %q", got)
	}
}

func TestRefreshStatusRetainsSearchMatchCount(t *testing.T) {
	targets := []inventory.EligibleTarget{
		{VM: inventory.VirtualMachine{ID: "vm-1", Name: "api", OSType: "Linux", SubscriptionID: "sub-1"}, Routes: []inventory.BastionRoute{{}}},
		{VM: inventory.VirtualMachine{ID: "vm-2", Name: "worker", OSType: "Linux", SubscriptionID: "sub-1"}, Routes: []inventory.BastionRoute{{}}},
	}
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{Subscriptions: []inventory.Subscription{{ID: "sub-1", Name: "One"}}}, cache.VMInventorySnapshot{Targets: targets}, cache.Preferences{})
	model.vmList.FilterInput.SetValue("api")
	model.vmList.SetFilterState(list.Filtering)
	model.rebuildList()

	updated, _ := model.Update(refreshSucceededMsg{
		topology:    cache.TopologySnapshot{Subscriptions: []inventory.Subscription{{ID: "sub-1", Name: "One"}}},
		vmInventory: cache.VMInventorySnapshot{Targets: targets},
	})
	if got := updated.(Model).status; got != "Loaded 2 eligible VMs (2 shown); 1 matching" {
		t.Fatalf("refresh search status = %q", got)
	}
}

func TestSearchStatusUpdatesAfterFilterResults(t *testing.T) {
	targets := []inventory.EligibleTarget{
		{VM: inventory.VirtualMachine{ID: "vm-1", Name: "api", OSType: "Linux", SubscriptionID: "sub-1"}, Routes: []inventory.BastionRoute{{}}},
		{VM: inventory.VirtualMachine{ID: "vm-2", Name: "worker", OSType: "Linux", SubscriptionID: "sub-1"}, Routes: []inventory.BastionRoute{{}}},
	}
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{Subscriptions: []inventory.Subscription{{ID: "sub-1", Name: "One"}}}, cache.VMInventorySnapshot{Targets: targets}, cache.Preferences{})
	model.vmList.SetFilterState(list.Filtering)

	updated, cmd := model.updateKey(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
	if cmd == nil {
		t.Fatal("filter input did not return a command")
	}
	commands, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("filter input did not return a batch command")
	}
	for _, command := range commands {
		if msg := command(); msg != nil {
			if _, ok := msg.(list.FilterMatchesMsg); ok {
				updated, _ = updated.(Model).Update(msg)
			}
		}
	}
	if got := updated.(Model).status; got != "1 matching VMs" {
		t.Fatalf("filter result status = %q", got)
	}
}

func TestSearchStatusClearsAfterCancel(t *testing.T) {
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{}, cache.Preferences{})
	model.status = "0 matching VMs"
	model.vmList.SetFilterState(list.Filtering)

	updated, _ := model.updateKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	result := updated.(Model)
	if result.vmList.SettingFilter() {
		t.Fatal("search remains active after escape")
	}
	if result.status != "" {
		t.Fatalf("search status = %q, want empty", result.status)
	}
}

func TestRefreshSearchStatusClearsAfterCancel(t *testing.T) {
	targets := []inventory.EligibleTarget{
		{VM: inventory.VirtualMachine{ID: "vm-1", Name: "api", OSType: "Linux", SubscriptionID: "sub-1"}, Routes: []inventory.BastionRoute{{}}},
		{VM: inventory.VirtualMachine{ID: "vm-2", Name: "worker", OSType: "Linux", SubscriptionID: "sub-1"}, Routes: []inventory.BastionRoute{{}}},
	}
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{Subscriptions: []inventory.Subscription{{ID: "sub-1", Name: "One"}}}, cache.VMInventorySnapshot{Targets: targets}, cache.Preferences{})
	model.vmList.FilterInput.SetValue("api")
	model.vmList.SetFilterState(list.Filtering)
	model.rebuildList()

	updated, _ := model.Update(refreshSucceededMsg{
		topology:    cache.TopologySnapshot{Subscriptions: []inventory.Subscription{{ID: "sub-1", Name: "One"}}},
		vmInventory: cache.VMInventorySnapshot{Targets: targets},
	})
	updated, _ = updated.(Model).updateKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if got := updated.(Model).status; got != "Loaded 2 eligible VMs (2 shown)" {
		t.Fatalf("cancelled refresh search status = %q", got)
	}
}

func TestUnicodeFromEnv(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "UTF-8 language", env: map[string]string{"LANG": "en_US.UTF-8"}, want: true},
		{name: "UTF8 language", env: map[string]string{"LANG": "en_US.utf8"}, want: true},
		{name: "LC_ALL overrides language", env: map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, want: false},
		{name: "unknown language", env: map[string]string{"LANG": "C"}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := unicodeFromEnv(func(name string) string { return test.env[name] }); got != test.want {
				t.Fatalf("unicodeFromEnv() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTargetDelegateFavoriteBoundary(t *testing.T) {
	delegate := newTargetDelegate(true)
	items := []list.Item{
		targetItem{favorite: true},
		targetItem{favorite: false},
	}
	if !delegate.isFavoriteBoundary(items, 0) {
		t.Fatal("expected a separator after the final favorite")
	}
	if delegate.isFavoriteBoundary(items, 1) {
		t.Fatal("did not expect a separator after a non-favorite")
	}
}

func TestUpdateCheckShowsBanner(t *testing.T) {
	model := Model{config: Config{Version: "v0.0.1"}, width: 100}
	updated, _ := model.Update(updateCheckSucceededMsg{latestVersion: "v0.0.2"})
	result := updated.(Model)
	if got := result.availableUpdate; got != "v0.0.2" {
		t.Fatalf("available update = %q", got)
	}
	if !strings.Contains(result.updateBanner(), "https://github.com/reinier-vegter/azssh/releases") {
		t.Fatal("update banner does not include the releases URL")
	}
}
