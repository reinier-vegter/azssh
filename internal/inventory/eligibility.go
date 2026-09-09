package inventory

import (
	"sort"
	"strings"
)

// ResolveEligibleTargets returns VMs reachable from at least one Bastion. A
// peered route requires connected, VNet-access-enabled peerings in both
// directions; Azure VNet peering is not transitive.
func ResolveEligibleTargets(bastions []Bastion, peerings []VNetPeering, vms []VirtualMachine) []EligibleTarget {
	edges := make(map[string]map[string]bool)
	for _, peering := range peerings {
		local, remote := normalizedID(peering.LocalVNetID), normalizedID(peering.RemoteVNetID)
		if local == "" || remote == "" {
			continue
		}
		if edges[local] == nil {
			edges[local] = make(map[string]bool)
		}
		edges[local][remote] = true
	}

	targets := make([]EligibleTarget, 0, len(vms))
	for _, vm := range vms {
		if !strings.EqualFold(strings.TrimSpace(vm.OSType), "Linux") {
			continue
		}
		routes := routesForVM(vm, bastions, edges)
		if len(routes) > 0 {
			targets = append(targets, EligibleTarget{VM: vm, Routes: routes})
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		return strings.ToLower(targets[i].VM.Name) < strings.ToLower(targets[j].VM.Name)
	})
	return targets
}

func routesForVM(vm VirtualMachine, bastions []Bastion, edges map[string]map[string]bool) []BastionRoute {
	routes := make([]BastionRoute, 0, len(bastions))
	seen := make(map[string]bool)
	for _, bastion := range bastions {
		bastionVNet := normalizedID(bastion.VNetID)
		for _, vmVNet := range vm.VNetIDs {
			vmVNet = normalizedID(vmVNet)
			if vmVNet == "" {
				continue
			}

			routeType := ""
			switch {
			case vmVNet == bastionVNet:
				routeType = "same VNet"
			case edges[bastionVNet][vmVNet] && edges[vmVNet][bastionVNet]:
				routeType = "peered VNet"
			}
			if routeType == "" || seen[normalizedID(bastion.ID)] {
				continue
			}

			seen[normalizedID(bastion.ID)] = true
			routes = append(routes, BastionRoute{Bastion: bastion, RouteType: routeType})
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].RouteType != routes[j].RouteType {
			return routes[i].RouteType == "same VNet"
		}
		return strings.ToLower(routes[i].Bastion.Name) < strings.ToLower(routes[j].Bastion.Name)
	})
	return routes
}

func normalizedID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
