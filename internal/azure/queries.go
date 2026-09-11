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
| extend vmSize = tostring(properties.hardwareProfile.vmSize)
| extend osDiskSizeGB = toint(properties.storageProfile.osDisk.diskSizeGB)
| extend osDiskStorageType = tostring(properties.storageProfile.osDisk.managedDisk.storageAccountType)
| extend dataDiskCount = array_length(properties.storageProfile.dataDisks)
| extend vmTags = tags
| project vmId, vmResourceId = id, vmName = name, resourceGroup,
          subscriptionId = tolower(subscriptionId), location, vmSize,
          osDiskSizeGB, osDiskStorageType, dataDiskCount, vmTags,
          osType = tostring(properties.storageProfile.osDisk.osType)
| join kind=inner (
    Resources
    | where type =~ 'microsoft.network/networkinterfaces'
    | where isnotempty(properties.virtualMachine.id)
    | mv-expand ipConfiguration = properties.ipConfigurations
    | extend vmId = tolower(tostring(properties.virtualMachine.id))
    | extend subnetId = tolower(tostring(ipConfiguration.properties.subnet.id))
    | extend vnetId = tostring(split(subnetId, '/subnets/')[0])
    | extend privateIp = tostring(ipConfiguration.properties.privateIPAddress)
    | where isnotempty(vmId) and isnotempty(vnetId)
    | project vmId, vnetId, subnetId, privateIp
) on vmId
| summarize vnetIds = make_set(vnetId), subnetIds = make_set(subnetId),
            privateIPs = make_set(privateIp),
            vmResourceId = take_any(vmResourceId), vmName = take_any(vmName),
            resourceGroup = take_any(resourceGroup),
            subscriptionId = take_any(subscriptionId), location = take_any(location),
            vmSize = take_any(vmSize), osDiskSizeGB = take_any(osDiskSizeGB),
            osDiskStorageType = take_any(osDiskStorageType),
            dataDiskCount = take_any(dataDiskCount), vmTags = take_any(vmTags),
            osType = take_any(osType) by vmId`
