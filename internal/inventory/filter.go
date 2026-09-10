package inventory

import "strings"

// FilterTargets applies the saved subscription visibility choices and a
// case-insensitive text query without making a network request.
func FilterTargets(targets []EligibleTarget, hiddenSubscriptionIDs map[string]bool, query string) []EligibleTarget {
	terms := strings.Fields(strings.ToLower(query))
	filtered := make([]EligibleTarget, 0, len(targets))
	for _, target := range targets {
		if !strings.EqualFold(strings.TrimSpace(target.VM.OSType), "Linux") {
			continue
		}
		if hiddenSubscriptionIDs[normalizedID(target.VM.SubscriptionID)] {
			continue
		}
		if len(terms) > 0 && !targetMatches(target, terms) {
			continue
		}
		filtered = append(filtered, target)
	}
	return filtered
}

func targetMatches(target EligibleTarget, terms []string) bool {
	values := []string{target.VM.Name, target.VM.ResourceGroup, target.VM.SubscriptionID}
	for _, route := range target.Routes {
		values = append(values, route.Bastion.Name, route.Bastion.ResourceGroup)
	}
	searchable := strings.ToLower(strings.Join(values, " "))
	for _, term := range terms {
		if !strings.Contains(searchable, term) {
			return false
		}
	}
	return true
}
