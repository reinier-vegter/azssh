package inventory

import "strings"

// FilterTargets applies the saved subscription visibility choices and a
// case-insensitive text query without making a network request.
func FilterTargets(targets []EligibleTarget, hiddenSubscriptionIDs map[string]bool, query string) []EligibleTarget {
	query = strings.ToLower(strings.TrimSpace(query))
	filtered := make([]EligibleTarget, 0, len(targets))
	for _, target := range targets {
		if !strings.EqualFold(strings.TrimSpace(target.VM.OSType), "Linux") {
			continue
		}
		if hiddenSubscriptionIDs[normalizedID(target.VM.SubscriptionID)] {
			continue
		}
		if query != "" && !targetMatches(target, query) {
			continue
		}
		filtered = append(filtered, target)
	}
	return filtered
}

func targetMatches(target EligibleTarget, query string) bool {
	if strings.Contains(strings.ToLower(target.VM.Name), query) ||
		strings.Contains(strings.ToLower(target.VM.ResourceGroup), query) ||
		strings.Contains(strings.ToLower(target.VM.SubscriptionID), query) {
		return true
	}
	for _, route := range target.Routes {
		if strings.Contains(strings.ToLower(route.Bastion.Name), query) ||
			strings.Contains(strings.ToLower(route.Bastion.ResourceGroup), query) {
			return true
		}
	}
	return false
}
