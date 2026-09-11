package azure

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestVirtualMachinesFromRowsPreservesDetailMetadata(t *testing.T) {
	rows := []virtualMachineRow{
		{
			ID: "/vms/api-01", Name: "api-01", ResourceGroup: "platform-rg", SubscriptionID: "sub-1",
			OSType: "Linux", Location: "westeurope", Size: "Standard_D4s_v5",
			VNetIDs: []string{"vnet-b", "vnet-a", "vnet-a"}, SubnetIDs: []string{"subnet-b", "", "subnet-a"},
			PrivateIPs: []string{"10.0.0.5", "10.0.0.4", "10.0.0.5"}, OSDiskSizeGB: 128,
			OSDiskStorageType: "Premium_LRS", DataDiskCount: 2,
			Tags: map[string]string{"environment": "production", "owner": "platform", "Ignore": "not cached"},
		},
		{ID: "/vms/no-network", VNetIDs: []string{""}},
	}

	vms := virtualMachinesFromRows(rows)
	if len(vms) != 1 {
		t.Fatalf("virtualMachinesFromRows() returned %d VMs, want 1", len(vms))
	}
	vm := vms[0]
	if vm.Location != "westeurope" || vm.Size != "Standard_D4s_v5" || vm.OSDiskSizeGB != 128 || vm.OSDiskStorageType != "Premium_LRS" || vm.DataDiskCount != 2 {
		t.Fatalf("metadata = %#v", vm)
	}
	if !reflect.DeepEqual(vm.VNetIDs, []string{"vnet-a", "vnet-b"}) || !reflect.DeepEqual(vm.SubnetIDs, []string{"subnet-a", "subnet-b"}) || !reflect.DeepEqual(vm.PrivateIPs, []string{"10.0.0.4", "10.0.0.5"}) {
		t.Fatalf("network metadata = %#v", vm)
	}
	if want := map[string]string{"environment": "production", "owner": "platform"}; !reflect.DeepEqual(vm.Tags, want) {
		t.Fatalf("tags = %#v, want %#v", vm.Tags, want)
	}
}

func TestVirtualMachineRowsDecodeResourceGraphPayload(t *testing.T) {
	const payload = `[
		{
			"vmResourceId": "/vms/api-01",
			"vmName": "api-01",
			"resourceGroup": "platform-rg",
			"subscriptionId": "sub-1",
			"osType": "Linux",
			"location": "westeurope",
			"vmSize": "Standard_D4s_v5",
			"vnetIds": ["vnet-a"],
			"subnetIds": ["subnet-a"],
			"privateIPs": ["10.0.0.4"],
			"osDiskSizeGB": null,
			"dataDiskCount": null,
			"vmTags": {"Environment": "production", "Owner": "platform"}
		}
	]`

	var rows []virtualMachineRow
	if err := json.Unmarshal([]byte(payload), &rows); err != nil {
		t.Fatal(err)
	}
	vms := virtualMachinesFromRows(rows)
	if len(vms) != 1 {
		t.Fatalf("decoded VMs = %#v", vms)
	}
	vm := vms[0]
	if vm.OSDiskSizeGB != 0 || vm.DataDiskCount != 0 || vm.OSDiskStorageType != "" {
		t.Fatalf("missing optional metadata = %#v", vm)
	}
	if want := map[string]string{"Environment": "production", "Owner": "platform"}; !reflect.DeepEqual(vm.Tags, want) {
		t.Fatalf("decoded tags = %#v, want %#v", vm.Tags, want)
	}
}

func TestVirtualMachineQueryProjectsDetailMetadata(t *testing.T) {
	for _, field := range []string{"location", "vmSize", "osDiskSizeGB", "osDiskStorageType", "dataDiskCount", "vmTags", "subnetIds", "privateIPs"} {
		if !strings.Contains(virtualMachinesQuery, field) {
			t.Errorf("virtualMachinesQuery does not project %q", field)
		}
	}
}

func TestVirtualMachineQueryAggregatesDynamicTags(t *testing.T) {
	if !strings.Contains(virtualMachinesQuery, "vmTags = take_any(vmTags)") {
		t.Fatal("virtualMachinesQuery does not aggregate dynamic tags")
	}
	if !strings.Contains(virtualMachinesQuery, "osType = take_any(osType) by vmId") {
		t.Fatal("virtualMachinesQuery does not group by the stable VM ID")
	}
	if strings.Contains(virtualMachinesQuery, "dataDiskCount, vmTags, osType") {
		t.Fatal("virtualMachinesQuery groups by dynamic tag values")
	}
}
