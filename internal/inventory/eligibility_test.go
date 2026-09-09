package inventory

import "testing"

func TestResolveEligibleTargets(t *testing.T) {
	bastions := []Bastion{
		{ID: "/bastions/one", Name: "one", VNetID: "/vnets/hub"},
		{ID: "/bastions/two", Name: "two", VNetID: "/vnets/spoke"},
	}
	peerings := []VNetPeering{
		{LocalVNetID: "/vnets/hub", RemoteVNetID: "/vnets/spoke"},
		{LocalVNetID: "/vnets/spoke", RemoteVNetID: "/vnets/hub"},
		{LocalVNetID: "/vnets/hub", RemoteVNetID: "/vnets/one-way"},
	}
	vms := []VirtualMachine{
		{ID: "/vms/hub", Name: "hub-vm", OSType: "Linux", VNetIDs: []string{"/vnets/hub"}},
		{ID: "/vms/spoke", Name: "spoke-vm", OSType: "Linux", VNetIDs: []string{"/vnets/spoke"}},
		{ID: "/vms/one-way", Name: "one-way-vm", OSType: "Linux", VNetIDs: []string{"/vnets/one-way"}},
		{ID: "/vms/none", Name: "none-vm", OSType: "Linux", VNetIDs: []string{"/vnets/none"}},
		{ID: "/vms/windows", Name: "windows-vm", OSType: "Windows", VNetIDs: []string{"/vnets/hub"}},
	}

	targets := ResolveEligibleTargets(bastions, peerings, vms)
	if len(targets) != 2 {
		t.Fatalf("expected 2 eligible targets, got %d", len(targets))
	}

	byName := make(map[string]EligibleTarget, len(targets))
	for _, target := range targets {
		byName[target.VM.Name] = target
	}

	hub := byName["hub-vm"]
	if len(hub.Routes) != 2 {
		t.Fatalf("unexpected hub routes: %#v", hub.Routes)
	}
	if hub.Routes[0].Bastion.Name != "one" || hub.Routes[0].RouteType != "same VNet" {
		t.Fatalf("expected same-VNet route first, got %#v", hub.Routes)
	}
	if hub.Routes[1].Bastion.Name != "two" || hub.Routes[1].RouteType != "peered VNet" {
		t.Fatalf("expected peered route second, got %#v", hub.Routes)
	}

	spoke := byName["spoke-vm"]
	if len(spoke.Routes) != 2 {
		t.Fatalf("expected two spoke routes, got %#v", spoke.Routes)
	}
	if spoke.Routes[0].Bastion.Name != "two" || spoke.Routes[0].RouteType != "same VNet" {
		t.Fatalf("expected same-VNet route first, got %#v", spoke.Routes)
	}
	if spoke.Routes[1].Bastion.Name != "one" || spoke.Routes[1].RouteType != "peered VNet" {
		t.Fatalf("expected peered route second, got %#v", spoke.Routes)
	}

	if _, found := byName["one-way-vm"]; found {
		t.Fatal("one-way peering must not produce an eligible route")
	}
	if _, found := byName["none-vm"]; found {
		t.Fatal("unrelated VNet must not produce an eligible route")
	}
	if _, found := byName["windows-vm"]; found {
		t.Fatal("Windows VMs must not produce an eligible target")
	}
}
