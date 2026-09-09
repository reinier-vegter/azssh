package azure

const subscriptionsQuery = `
ResourceContainers
| where type =~ 'microsoft.resources/subscriptions'
| project subscriptionId = tolower(subscriptionId), subscriptionName = name`

const bastionsQuery = `
Resources
| where type =~ 'microsoft.network/bastionhosts'
| where tostring(properties.provisioningState) =~ 'Succeeded'
| where tostring(sku.name) in~ ('Standard', 'Premium')
| where tostring(properties.enableTunneling) =~ 'true'
| mv-expand ipConfiguration = properties.ipConfigurations
| extend subnetId = tolower(tostring(ipConfiguration.properties.subnet.id))
| extend vnetId = tostring(split(subnetId, '/subnets/')[0])
| where isnotempty(vnetId)
| project bastionId = id, bastionName = name, resourceGroup,
          subscriptionId = tolower(subscriptionId), vnetId`

const peeringsQuery = `
Resources
| where type =~ 'microsoft.network/virtualnetworks/virtualnetworkpeerings'
| where tostring(properties.peeringState) =~ 'Connected'
| where tostring(properties.allowVirtualNetworkAccess) =~ 'true'
| extend localVnetId = tolower(tostring(split(id, '/virtualNetworkPeerings/')[0]))
| extend remoteVnetId = tolower(tostring(properties.remoteVirtualNetwork.id))
| where isnotempty(localVnetId) and isnotempty(remoteVnetId)
| project localVnetId, remoteVnetId`

const virtualMachinesQuery = `
Resources
| where type =~ 'microsoft.compute/virtualmachines'
| where tostring(properties.storageProfile.osDisk.osType) =~ 'Linux'
| extend vmId = tolower(id)
| project vmId, vmResourceId = id, vmName = name, resourceGroup,
          subscriptionId = tolower(subscriptionId),
          osType = tostring(properties.storageProfile.osDisk.osType)
| join kind=inner (
    Resources
    | where type =~ 'microsoft.network/networkinterfaces'
    | where isnotempty(properties.virtualMachine.id)
    | mv-expand ipConfiguration = properties.ipConfigurations
    | extend vmId = tolower(tostring(properties.virtualMachine.id))
    | extend subnetId = tolower(tostring(ipConfiguration.properties.subnet.id))
    | extend vnetId = tostring(split(subnetId, '/subnets/')[0])
    | where isnotempty(vmId) and isnotempty(vnetId)
    | project vmId, vnetId
) on vmId
| summarize vnetIds = make_set(vnetId) by
    vmResourceId, vmName, resourceGroup, subscriptionId, osType`
