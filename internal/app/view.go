package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	accentStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	favoriteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
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
	result := tea.NewView(content)
	result.AltScreen = m.config.AltScreen
	return result
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
		return mutedStyle.Render("Select an eligible VM to inspect its Bastion route.")
	}
	route := target.Routes[0]
	lines := []string{
		accentStyle.Render(target.VM.Name),
		"",
		"Subscription  " + m.subscriptionName(target.VM.SubscriptionID),
		"Resource group " + target.VM.ResourceGroup,
		"OS            " + displayValue(target.VM.OSType),
		"",
		accentStyle.Render("Connection route"),
		"Bastion       " + route.Bastion.Name,
		"Bastion RG    " + route.Bastion.ResourceGroup,
		"VNet route    " + route.RouteType,
	}
	if len(target.Routes) > 1 {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("%d routes available; enter or b to choose", len(target.Routes))))
	} else {
		lines = append(lines, "", mutedStyle.Render("enter connect  shift+enter review command"))
	}
	return strings.Join(lines, "\n")
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
	m.vmList.SetSize(listWidth, max(8, m.height-8))
}

func displayValue(value string) string {
	if value == "" {
		return "Unknown"
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
