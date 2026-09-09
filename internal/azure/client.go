// Package azure provides Azure Resource Graph inventory queries.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"azssh/internal/inventory"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
)

// Client queries the signed-in user's Azure Resource Graph inventory.
type Client struct {
	resourceGraph *armresourcegraph.Client
}

// NewClient creates a Resource Graph client authenticated through the current
// Azure CLI session.
func NewClient() (*Client, error) {
	credential, err := azidentity.NewAzureCLICredential(nil)
	if err != nil {
		return nil, fmt.Errorf("create Azure CLI credential: %w", err)
	}
	resourceGraph, err := armresourcegraph.NewClientFactory(credential, nil)
	if err != nil {
		return nil, fmt.Errorf("create Resource Graph client: %w", err)
	}
	return &Client{resourceGraph: resourceGraph.NewClient()}, nil
}

// ListSubscriptions returns all subscriptions readable through Resource Graph.
func (c *Client) ListSubscriptions(ctx context.Context) ([]inventory.Subscription, error) {
	type row struct {
		ID   string `json:"subscriptionId"`
		Name string `json:"subscriptionName"`
	}
	rows, err := queryAll[row](ctx, c.resourceGraph, subscriptionsQuery, nil)
	if err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}

	subscriptions := make([]inventory.Subscription, 0, len(rows))
	for _, row := range rows {
		if row.ID != "" {
			subscriptions = append(subscriptions, inventory.Subscription{ID: row.ID, Name: row.Name})
		}
	}
	return subscriptions, nil
}

// ListBastions returns only Bastions usable by the native Azure CLI SSH flow.
func (c *Client) ListBastions(ctx context.Context, subscriptionIDs []string) ([]inventory.Bastion, error) {
	type row struct {
		ID             string `json:"bastionId"`
		Name           string `json:"bastionName"`
		ResourceGroup  string `json:"resourceGroup"`
		SubscriptionID string `json:"subscriptionId"`
		VNetID         string `json:"vnetId"`
	}
	rows, err := queryAll[row](ctx, c.resourceGraph, bastionsQuery, subscriptionIDs)
	if err != nil {
		return nil, fmt.Errorf("query Bastion hosts: %w", err)
	}

	bastions := make([]inventory.Bastion, 0, len(rows))
	for _, row := range rows {
		if row.ID != "" && row.VNetID != "" {
			bastions = append(bastions, inventory.Bastion{
				ID: row.ID, Name: row.Name, ResourceGroup: row.ResourceGroup,
				SubscriptionID: row.SubscriptionID, VNetID: row.VNetID,
			})
		}
	}
	return bastions, nil
}

// ListPeerings returns connected VNet peerings that permit VNet access.
func (c *Client) ListPeerings(ctx context.Context, subscriptionIDs []string) ([]inventory.VNetPeering, error) {
	type row struct {
		LocalVNetID  string `json:"localVnetId"`
		RemoteVNetID string `json:"remoteVnetId"`
	}
	rows, err := queryAll[row](ctx, c.resourceGraph, peeringsQuery, subscriptionIDs)
	if err != nil {
		return nil, fmt.Errorf("query VNet peerings: %w", err)
	}

	peerings := make([]inventory.VNetPeering, 0, len(rows))
	for _, row := range rows {
		if row.LocalVNetID != "" && row.RemoteVNetID != "" {
			peerings = append(peerings, inventory.VNetPeering{LocalVNetID: row.LocalVNetID, RemoteVNetID: row.RemoteVNetID})
		}
	}
	return peerings, nil
}

// ListVMs returns VM metadata joined to NIC-derived VNet IDs.
func (c *Client) ListVMs(ctx context.Context, subscriptionIDs []string) ([]inventory.VirtualMachine, error) {
	type row struct {
		ID             string   `json:"vmResourceId"`
		Name           string   `json:"vmName"`
		ResourceGroup  string   `json:"resourceGroup"`
		SubscriptionID string   `json:"subscriptionId"`
		OSType         string   `json:"osType"`
		VNetIDs        []string `json:"vnetIds"`
	}
	rows, err := queryAll[row](ctx, c.resourceGraph, virtualMachinesQuery, subscriptionIDs)
	if err != nil {
		return nil, fmt.Errorf("query virtual machines: %w", err)
	}

	vms := make([]inventory.VirtualMachine, 0, len(rows))
	for _, row := range rows {
		if row.ID != "" && len(row.VNetIDs) > 0 {
			vms = append(vms, inventory.VirtualMachine{
				ID: row.ID, Name: row.Name, ResourceGroup: row.ResourceGroup,
				SubscriptionID: row.SubscriptionID, OSType: row.OSType, VNetIDs: row.VNetIDs,
			})
		}
	}
	return vms, nil
}

func queryAll[T any](ctx context.Context, client *armresourcegraph.Client, query string, subscriptionIDs []string) ([]T, error) {
	subscriptions := make([]*string, 0, len(subscriptionIDs))
	for _, id := range subscriptionIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			subscriptions = append(subscriptions, to.Ptr(id))
		}
	}

	request := armresourcegraph.QueryRequest{
		Query:         to.Ptr(query),
		Subscriptions: subscriptions,
		Options: &armresourcegraph.QueryRequestOptions{
			ResultFormat: to.Ptr(armresourcegraph.ResultFormatObjectArray),
			Top:          to.Ptr(int32(1000)),
		},
	}

	var allRows []T
	for {
		response, err := client.Resources(ctx, request, nil)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(response.Data)
		if err != nil {
			return nil, fmt.Errorf("encode query result: %w", err)
		}
		var rows []T
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("decode query result: %w", err)
		}
		allRows = append(allRows, rows...)

		if response.SkipToken == nil || *response.SkipToken == "" {
			if response.ResultTruncated != nil && *response.ResultTruncated == armresourcegraph.ResultTruncatedTrue {
				return nil, fmt.Errorf("query result was truncated without a skip token")
			}
			return allRows, nil
		}
		request.Options.SkipToken = response.SkipToken
	}
}
