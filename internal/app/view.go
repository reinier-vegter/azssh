package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"azssh/internal/inventory"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	favoriteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	updateStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("208")).Bold(true)
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
)

// View renders the active application screen.
func (m Model) View() tea.View {
	var content string
	switch m.activeView {
	case subscriptionFilterView:
		content = m.subscriptionFilterView()
	case routeSelectorView:
		content = m.routeSelectorView()
	case helpView:
		content = m.helpView()
	default:
		content = m.mainScreen()
	}
	if m.availableUpdate != "" {
		content = m.updateBanner() + "\n" + content
	}
	result := tea.NewView(content)
	result.AltScreen = m.config.AltScreen
	return result
}

func (m Model) updateBanner() string {
	banner := "Update available: " + m.availableUpdate + " (current: " + m.config.Version + ")  https://github.com/reinier-vegter/azssh/releases"
	if m.width > 0 {
		return updateStyle.Width(m.width).Render(banner)
	}
	return updateStyle.Render(banner)
}

func (m Model) mainScreen() string {
	header := accentStyle.Render("azssh") + strings.Repeat(" ", max(1, m.width-28)) + m.cacheStatus()
	left := m.vmList.View()
	right := m.detailView()
	content := lipgloss.JoinHorizontal(lipgloss.Top, panelStyle.Render(left), "  ", panelStyle.Render(right))
	if m.width > 0 && m.width < 86 {
		content = lipgloss.JoinVertical(lipgloss.Left, panelStyle.Render(left), panelStyle.Render(right))
	}
	footer := mutedStyle.Render("enter connect  shift+enter review  / search  x favorite  f filters  ? help  r refresh  q quit")
	if m.status != "" {
		footer = footer + "\n" + m.statusStyle().Render(m.status)
	}
	return header + "\n" + strings.Repeat("─", max(1, m.width)) + "\n" + content + "\n" + footer
}

func (m Model) detailView() string {
	target := m.selectedTarget()
	if target == nil {
		return m.emptyDetailView()
	}
	route := target.Routes[0]
	lines := []string{
		accentStyle.Render(target.VM.Name),
		"",
		detailLine("Subscription", m.subscriptionName(target.VM.SubscriptionID)),
		detailLine("Resource group", target.VM.ResourceGroup),
		"",
	}
	lines = appendDetailLine(lines, "Location", target.VM.Location)
	lines = appendDetailLine(lines, "Size", target.VM.Size)
	lines = appendDetailLine(lines, "OS", target.VM.OSType)
	if len(target.VM.PrivateIPs) > 0 || len(target.VM.SubnetIDs) > 0 || len(target.VM.VNetIDs) > 0 {
		lines = append(lines, "", accentStyle.Render("Network"))
		if len(target.VM.PrivateIPs) > 0 {
			lines = append(lines, detailLine("Private IPs", strings.Join(target.VM.PrivateIPs, ", ")))
		}
		if len(target.VM.SubnetIDs) > 0 {
			lines = append(lines, detailLine("Subnets", strings.Join(target.VM.SubnetIDs, ", ")))
		}
		if len(target.VM.VNetIDs) > 0 {
			lines = append(lines, detailLine("VNets", strings.Join(target.VM.VNetIDs, ", ")))
		}
	}
	if disk := diskDescription(target.VM.OSDiskSizeGB, target.VM.OSDiskStorageType); disk != "" || target.VM.DataDiskCount > 0 {
		lines = append(lines, "", accentStyle.Render("Storage"))
		if disk != "" {
			lines = append(lines, detailLine("OS disk", disk))
		}
		if target.VM.DataDiskCount > 0 {
			lines = append(lines, detailLine("Data disks", fmt.Sprintf("%d", target.VM.DataDiskCount)))
		}
	}
	if tags := displayTags(target.VM.Tags); len(tags) > 0 {
		lines = append(lines, "", accentStyle.Render("Tags"))
		for _, tag := range tags {
			lines = append(lines, detailLine(tag.Key, tag.Value))
		}
	}
	lines = append(lines, "", accentStyle.Render("Connection route"), detailLine("Bastion", route.Bastion.Name), detailLine("Bastion RG", route.Bastion.ResourceGroup))
	routeType := route.RouteType
	if len(target.Routes) > 1 {
		routeType += " (preferred)"
	}
	lines = append(lines, detailLine("VNet route", routeType))
	if len(target.Routes) > 1 {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("%d routes available; enter or b to choose", len(target.Routes))))
	} else {
		lines = append(lines, "", mutedStyle.Render("enter connect  shift+enter review command"))
	}
	return m.wrapDetail(strings.Join(lines, "\n"))
}

func (m Model) emptyDetailView() string {
	if len(m.targets) == 0 {
		if strings.HasPrefix(m.status, "Refresh failed") {
			return errorStyle.Render("Azure inventory could not be loaded. Check your Azure CLI session and refresh.")
		}
		return mutedStyle.Render("No eligible VMs found. Only Linux VMs with a discovered compatible Bastion route are shown.")
	}
	if len(m.vmList.Items()) == 0 {
		return mutedStyle.Render("No VMs are visible. Press f to change subscription visibility.")
	}
	if m.vmList.SettingFilter() && len(m.vmList.VisibleItems()) == 0 {
		return mutedStyle.Render("No VMs match the current search.")
	}
	return mutedStyle.Render("Select an eligible VM to inspect its Bastion route.")
}

func appendDetailLine(lines []string, label, value string) []string {
	if value != "" {
		return append(lines, detailLine(label, value))
	}
	return lines
}

func detailLine(label, value string) string {
	return fmt.Sprintf("%-14s %s", label, value)
}

func diskDescription(sizeGB int, storageType string) string {
	parts := make([]string, 0, 2)
	if sizeGB > 0 {
		parts = append(parts, fmt.Sprintf("%d GiB", sizeGB))
	}
	if storageType != "" {
		parts = append(parts, storageType)
	}
	return strings.Join(parts, " / ")
}

type displayTag struct {
	Key   string
	Value string
}

func displayTags(tags map[string]string) []displayTag {
	tags = inventory.DisplayTags(tags)
	result := make([]displayTag, 0, len(tags))
	for key, value := range tags {
		result = append(result, displayTag{Key: key, Value: value})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := strings.ToLower(result[i].Key), strings.ToLower(result[j].Key)
		if left == right {
			return result[i].Key < result[j].Key
		}
		return left < right
	})
	return result
}

func (m Model) wrapDetail(content string) string {
	if width := m.detailWidth(); width > 0 {
		return lipgloss.Wrap(content, width, " /,=")
	}
	return content
}

func (m Model) detailWidth() int {
	if m.width <= 0 {
		return 0
	}
	if m.width < 86 {
		return max(20, m.width-6)
	}
	return max(30, m.width-m.width/2-6)
}

func (m Model) subscriptionFilterView() string {
	lines := []string{
		accentStyle.Render("Filter subscriptions"),
		"",
		"Select the subscriptions whose VMs should be shown.",
		"",
	}
	visible := 0
	for index, subscription := range m.subscriptions {
		hidden := m.filterDraft[subscription.ID]
		if !hidden {
			visible++
		}
		cursor := " "
		if index == m.filterIndex {
			cursor = ">"
		}
		mark := " "
		if !hidden {
			mark = "x"
		}
		lines = append(lines, fmt.Sprintf("%s [%s] %s", cursor, mark, subscription.Name))
	}
	lines = append(lines, "", fmt.Sprintf("%d of %d subscriptions shown", visible, len(m.subscriptions)), "", mutedStyle.Render("space toggle  a all  n none  enter apply  esc cancel  ? help"))
	return panelStyle.Render(strings.Join(lines, "\n"))
}

func (m Model) routeSelectorView() string {
	if m.routeTarget == nil {
		return m.mainScreen()
	}
	lines := []string{accentStyle.Render("Select Bastion for " + m.routeTarget.VM.Name), ""}
	for index, route := range m.routeTarget.Routes {
		cursor := " "
		if index == m.routeIndex {
			cursor = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %-22s %s / %s   %s", cursor, route.Bastion.Name, m.subscriptionName(route.Bastion.SubscriptionID), route.Bastion.ResourceGroup, route.RouteType))
	}
	lines = append(lines, "", mutedStyle.Render("enter connect  shift+enter review command  esc cancel  ? help"))
	return panelStyle.Render(strings.Join(lines, "\n"))
}

func (m Model) helpView() string {
	lines := []string{
		accentStyle.Render("Help"),
		"",
		accentStyle.Render("Navigation"),
		"up/k, down/j     Move selection",
		"/                Focus VM search",
		"x                Toggle selected VM favorite",
		"f                Filter subscriptions",
		"?                Open or close help",
		"r                Refresh inventory",
		"q, ctrl+c        Quit",
		"",
		accentStyle.Render("Connection"),
		"enter            Connect, or select a Bastion route",
		"shift+enter      Review the command in your shell, then confirm",
		"b                Choose an eligible Bastion route",
		"",
		accentStyle.Render("Filter subscriptions"),
		"space            Toggle subscription visibility",
		"a / n            Show all / hide all",
		"enter            Apply filters",
		"esc              Cancel filter changes or close a view",
		"",
		mutedStyle.Render("esc, ?           Return"),
	}
	return panelStyle.Render(strings.Join(lines, "\n"))
}

func (m Model) cacheStatus() string {
	if m.loading {
		return m.spinner.View() + " syncing"
	}
	if m.lastRefresh.IsZero() {
		return mutedStyle.Render("cache: empty")
	}
	age := time.Since(m.lastRefresh).Round(time.Minute)
	if strings.HasPrefix(m.status, "Refresh failed") {
		return errorStyle.Render("cache: " + age.String() + " old (stale)")
	}
	return mutedStyle.Render("cache: " + age.String() + " old")
}

func (m Model) statusStyle() lipgloss.Style {
	if strings.HasPrefix(m.status, "Refresh failed") || strings.HasPrefix(m.status, "Cannot") || strings.HasPrefix(m.status, "Save") {
		return errorStyle
	}
	return mutedStyle
}

func (m *Model) resizeList() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	listWidth := max(30, m.width/2-4)
	if m.width < 86 {
		listWidth = max(30, m.width-6)
	}
	bannerHeight := 0
	if m.availableUpdate != "" {
		bannerHeight = 1
	}
	m.vmList.SetSize(listWidth, max(8, m.height-8-bannerHeight))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
