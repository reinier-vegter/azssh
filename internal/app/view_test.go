package app

import (
	"strings"
	"testing"
	"time"

	"azssh/internal/cache"
	"azssh/internal/inventory"
)

func TestDetailViewRendersMetadataAndMatchingTags(t *testing.T) {
	target := inventory.EligibleTarget{
		VM: inventory.VirtualMachine{
			ID: "vm-1", Name: "api-01", SubscriptionID: "sub-1", ResourceGroup: "platform-rg", OSType: "Linux",
			Location: "westeurope", Size: "Standard_D4s_v5", VNetIDs: []string{"vnet-a"},
			SubnetIDs: []string{"/subscriptions/sub-1/subnets/apps"}, PrivateIPs: []string{"10.0.0.4", "10.0.0.5"},
			OSDiskSizeGB: 128, OSDiskStorageType: "Premium_LRS", DataDiskCount: 2,
			Tags: map[string]string{"Owner": "platform", "environment": "production", "ignore": "hidden"},
		},
		Routes: []inventory.BastionRoute{
			{Bastion: inventory.Bastion{Name: "bastion-prod", ResourceGroup: "network-rg"}, RouteType: "same VNet"},
			{Bastion: inventory.Bastion{Name: "bastion-shared", ResourceGroup: "shared-rg"}, RouteType: "peered VNet"},
		},
	}
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{Subscriptions: []inventory.Subscription{{ID: "sub-1", Name: "Production"}}}, cache.VMInventorySnapshot{Targets: []inventory.EligibleTarget{target}}, cache.Preferences{})

	view := model.detailView()
	for _, value := range []string{"Location", "westeurope", "Size", "Standard_D4s_v5", "Private IPs", "10.0.0.4", "Subnets", "VNets", "Storage", "128 GiB / Premium_LRS", "Data disks", "Tags", "environment", "production", "Owner", "platform", "same VNet (preferred)"} {
		if !strings.Contains(view, value) {
			t.Errorf("detail view does not contain %q:\n%s", value, view)
		}
	}
	if strings.Contains(view, "ignore") || strings.Contains(view, "hidden") {
		t.Fatalf("detail view includes an unmatched tag:\n%s", view)
	}
}

func TestDisplayTagsFiltersAndSorts(t *testing.T) {
	tags := displayTags(map[string]string{"Team": "core", "CostCenter": "123", "environment": "prod", "Ignore": "no", "owner": ""})
	if len(tags) != 3 {
		t.Fatalf("displayTags() = %#v, want 3 tags", tags)
	}
	got := []string{tags[0].Key, tags[1].Key, tags[2].Key}
	want := []string{"CostCenter", "environment", "Team"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tag order = %#v, want %#v", got, want)
	}
}

func TestEmptyDetailViewExplainsInventoryState(t *testing.T) {
	if view := (Model{}).emptyDetailView(); !strings.Contains(view, "No eligible VMs found") {
		t.Fatalf("empty inventory view = %q", view)
	}
	if view := (Model{status: "Refresh failed: unavailable"}).emptyDetailView(); !strings.Contains(view, "Azure inventory could not be loaded") {
		t.Fatalf("failed empty inventory view = %q", view)
	}
	if view := (Model{lastRefresh: time.Now(), status: "Refresh failed: unavailable"}).emptyDetailView(); !strings.Contains(view, "Azure inventory could not be loaded") {
		t.Fatalf("failed cached empty inventory view = %q", view)
	}

	target := inventory.EligibleTarget{VM: inventory.VirtualMachine{ID: "vm-1", OSType: "Linux"}, Routes: []inventory.BastionRoute{{}}}
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{Targets: []inventory.EligibleTarget{target}}, cache.Preferences{HiddenSubscriptionIDs: []string{""}})
	model.vmList.SetItems(nil)
	if view := model.emptyDetailView(); !strings.Contains(view, "Press f") {
		t.Fatalf("hidden inventory view = %q", view)
	}
}

func TestMountPathViewShowsSelectedMountpoint(t *testing.T) {
	model := NewModel(nil, nil, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{}, cache.Preferences{})
	model.openMountPath(inventory.BastionRoute{Bastion: inventory.Bastion{Name: "bastion-prod", ResourceGroup: "network-rg"}}, inventory.VirtualMachine{Name: "api-01"})
	view := model.mountPathView()
	for _, value := range []string{"Mount api-01", "~/azssh/mnt/api-01", "bastion-prod", "Remote path", "enter mount"} {
		if !strings.Contains(view, value) {
			t.Errorf("mount path view does not contain %q:\n%s", value, view)
		}
	}
}
