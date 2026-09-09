package app

import (
	"testing"

	"azssh/internal/inventory"

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
