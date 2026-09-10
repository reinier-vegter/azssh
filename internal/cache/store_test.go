package cache

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"azssh/internal/inventory"
)

func TestTopologyRoundTrip(t *testing.T) {
	store := testStore(t)
	snapshot := TopologySnapshot{
		FetchedAt:       time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		SubscriptionIDs: []string{"sub-1"},
		Subscriptions:   []inventory.Subscription{{ID: "sub-1", Name: "Production"}},
		Bastions:        []inventory.Bastion{{ID: "bastion-1", Name: "hub"}},
		Peerings:        []inventory.VNetPeering{{LocalVNetID: "vnet-a", RemoteVNetID: "vnet-b"}},
	}
	if err := store.SaveTopology(snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadTopology()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, snapshot) {
		t.Fatalf("loaded topology = %#v, want %#v", got, snapshot)
	}
}

func TestVMInventoryAndPreferencesRoundTrip(t *testing.T) {
	store := testStore(t)
	inventorySnapshot := VMInventorySnapshot{FetchedAt: time.Now().UTC().Round(0), Targets: []inventory.EligibleTarget{{VM: inventory.VirtualMachine{ID: "vm-1", VNetIDs: []string{"vnet-1"}}}}}
	if err := store.SaveVMInventory(inventorySnapshot); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadVMInventory(); err != nil || !reflect.DeepEqual(got, inventorySnapshot) {
		t.Fatalf("loaded VM inventory = %#v, %v", got, err)
	}

	preferences := Preferences{HiddenSubscriptionIDs: []string{"sub-2"}, FavoriteVMIDs: []string{"vm-2"}}
	if err := store.SavePreferences(preferences); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadPreferences(); err != nil || !reflect.DeepEqual(got, preferences) {
		t.Fatalf("loaded preferences = %#v, %v", got, err)
	}
}

func TestLoadDistinguishesNotFoundAndUnsupportedSchema(t *testing.T) {
	store := testStore(t)
	if _, err := store.LoadTopology(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing topology error = %v, want ErrNotFound", err)
	}
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.directory, "topology.json"), []byte(`{"schemaVersion":99,"data":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadTopology(); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("unsupported schema error = %v, want ErrUnsupportedSchema", err)
	}
}

func TestIsTopologyFresh(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name      string
		fetchedAt time.Time
		want      bool
	}{
		{"within TTL", now.Add(-topologyTTL), true},
		{"expired", now.Add(-topologyTTL - time.Nanosecond), false},
		{"future", now.Add(time.Nanosecond), false},
		{"zero", time.Time{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := IsTopologyFresh(TopologySnapshot{FetchedAt: test.fetchedAt}, now); got != test.want {
				t.Fatalf("IsTopologyFresh() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestAccountDirectory(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	directory, err := AccountDirectory(filepath.Join("tenant", "principal"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "azssh", "tenant", "principal")
	if directory != want {
		t.Fatalf("AccountDirectory() = %q, want %q", directory, want)
	}
	if _, err := AccountDirectory("../other-account"); err == nil {
		t.Fatal("AccountDirectory accepted a namespace escaping the cache root")
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	return &Store{directory: t.TempDir()}
}
