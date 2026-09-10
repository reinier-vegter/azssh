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
