package inventory

import "testing"

func TestFilterTargets(t *testing.T) {
	targets := []EligibleTarget{
		{VM: VirtualMachine{ID: "one", Name: "web-east", OSType: "Linux", SubscriptionID: "sub-one", ResourceGroup: "web"}, Routes: []BastionRoute{{Bastion: Bastion{Name: "east-bastion"}}}},
		{VM: VirtualMachine{ID: "two", Name: "data-west", OSType: "Linux", SubscriptionID: "sub-two", ResourceGroup: "data"}, Routes: []BastionRoute{{Bastion: Bastion{Name: "west-bastion"}}}},
		{VM: VirtualMachine{ID: "three", Name: "windows-east", OSType: "Windows", SubscriptionID: "sub-two", ResourceGroup: "web"}, Routes: []BastionRoute{{Bastion: Bastion{Name: "east-bastion"}}}},
	}

	visible := FilterTargets(targets, map[string]bool{"sub-one": true}, "")
	if len(visible) != 1 || visible[0].VM.ID != "two" {
		t.Fatalf("hidden subscription was not excluded: %#v", visible)
	}
	matched := FilterTargets(targets, nil, "west-bastion")
	if len(matched) != 1 || matched[0].VM.ID != "two" {
		t.Fatalf("Bastion query should match target: %#v", matched)
	}
	matched = FilterTargets(targets, nil, "data west")
	if len(matched) != 1 || matched[0].VM.ID != "two" {
		t.Fatalf("every query term should match: %#v", matched)
	}
	if matched := FilterTargets(targets, nil, "data east"); len(matched) != 0 {
		t.Fatalf("targets missing a query term should be excluded: %#v", matched)
	}
	if visible := FilterTargets(targets, nil, ""); len(visible) != 2 {
		t.Fatalf("Windows targets should be excluded from cached inventory: %#v", visible)
	}
}
