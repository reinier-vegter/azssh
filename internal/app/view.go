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
	accentStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	favoriteStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	notificationStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	panelStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
)

// View renders the active application screen.
func (m Model) View() tea.View {
	var content string
	switch m.activeView {
	case subscriptionFilterView:
		content = m.subscriptionFilterView()
	case routeSelectorView:
		content = m.routeSelectorView()
	case mountPathView:
		content = m.mountPathView()
	case helpView:
		content = m.helpView()
	case selfUpdateView:
		content = m.selfUpdateScreen()
	default:
		content = m.mainScreen()
	}
	result := tea.NewView(content)
	result.AltScreen = m.config.AltScreen
	return result
}

func (m Model) mainScreen() string {
	layout := m.finderLayout()
	if layout.tooSmall {
		return m.smallScreen()
	}
	list := m.vmList
	list.SetSize(layout.leftWidth-4, layout.leftHeight-2)
	left := renderPanel(viewportText(list.View(), layout.leftWidth-4, layout.leftHeight-2, 0), layout.leftWidth, layout.leftHeight)
	right := renderPanel(viewportText(m.detailView(), layout.rightWidth-4, layout.rightHeight-2, m.detailOffset), layout.rightWidth, layout.rightHeight)
	content := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	if layout.stacked {
		content = lipgloss.JoinVertical(lipgloss.Left, left, right)
	}
	rule := "-"
	if m.useUnicode {
		rule = "─"
	}
	return layout.header + "\n" + mutedStyle.Render(strings.Repeat(rule, m.screenWidth())) + "\n" + content + "\n" + layout.footer
}

func (m Model) detailView() string {
	target := m.selectedTarget()
	if target == nil {
		return m.wrapDetail(m.emptyDetailView())
	}
	route := target.Routes[0]
	width := m.detailWidth()
	lines := []string{accentStyle.Render(lipgloss.Wrap(target.VM.Name, width, "-")), ""}
	lines = append(lines, renderDetailRows([]detailRow{
		{"Subscription", m.subscriptionName(target.VM.SubscriptionID)},
		{"Resource group", target.VM.ResourceGroup},
		{"Location", target.VM.Location}, {"Size", target.VM.Size}, {"OS", target.VM.OSType},
	}, width)...)
	if len(target.VM.PrivateIPs) > 0 || len(target.VM.SubnetIDs) > 0 || len(target.VM.VNetIDs) > 0 {
		lines = append(lines, "", accentStyle.Render("Network"))
		lines = append(lines, renderDetailRows([]detailRow{
			{"Private IPs", strings.Join(target.VM.PrivateIPs, ", ")},
			{"VNets", networkNames(target.VM.VNetIDs, m.fullNetworkIDs)},
			{"Subnets", networkNames(target.VM.SubnetIDs, m.fullNetworkIDs)},
		}, width)...)
	}
	if disk := diskDescription(target.VM.OSDiskSizeGB, target.VM.OSDiskStorageType); disk != "" || target.VM.DataDiskCount > 0 {
		lines = append(lines, "", accentStyle.Render("Storage"))
		rows := []detailRow{{"OS disk", disk}}
		if target.VM.DataDiskCount > 0 {
			rows = append(rows, detailRow{"Data disks", fmt.Sprintf("%d", target.VM.DataDiskCount)})
		}
		lines = append(lines, renderDetailRows(rows, width)...)
	}
	if tags := displayTags(target.VM.Tags); len(tags) > 0 {
		lines = append(lines, "", accentStyle.Render("Tags"))
		rows := make([]detailRow, 0, len(tags))
		for _, tag := range tags {
			rows = append(rows, detailRow{tag.Key, tag.Value})
		}
		lines = append(lines, renderDetailRows(rows, width)...)
	}
	lines = append(lines, "", accentStyle.Render("Connection route"))
	routeType := route.RouteType
	if len(target.Routes) > 1 {
		routeType += " (preferred)"
	}
	lines = append(lines, renderDetailRows([]detailRow{{"Bastion", route.Bastion.Name}, {"Bastion RG", route.Bastion.ResourceGroup}, {"VNet route", routeType}}, width)...)
	if len(target.Routes) > 1 {
		lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("%d eligible Bastion routes", len(target.Routes))))
	} else {
		lines = append(lines, "", mutedStyle.Render("One eligible Bastion route"))
	}
	return strings.Join(lines, "\n")
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
	width := m.screenWidth()
	if width < 86 {
		return max(1, width-4)
	}
	left := (width - 2) / 2
	return max(1, width-2-left-4)
}

func (m Model) subscriptionFilterView() string {
	content, _ := m.renderSubscriptionFilter()
	return content
}

func (m Model) renderSubscriptionFilter() (string, int) {
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
	lines = append(lines, "", fmt.Sprintf("%d of %d subscriptions shown", visible, len(m.subscriptions)))
	focus := -1
	if m.filterIndex < len(m.subscriptions) {
		focus = lipgloss.Height(lipgloss.Wrap(strings.Join(lines[:4+m.filterIndex], "\n"), m.screenWidth()-4, " /,="))
	}
	return m.secondaryFrame(strings.Join(lines, "\n"), []shortcut{{"space", "toggle"}, {"a", "all"}, {"n", "none"}, {"enter", "apply"}, {"esc", "cancel"}, {"?", "help"}, {"q", "quit"}}, focus)
}

func (m Model) routeSelectorView() string {
	content, _ := m.renderRouteSelector()
	return content
}

func (m Model) renderRouteSelector() (string, int) {
	if m.routeTarget == nil {
		return m.mainScreen(), 0
	}
	lines := []string{accentStyle.Render("Select Bastion for " + m.routeTarget.VM.Name), ""}
	for index, route := range m.routeTarget.Routes {
		cursor := " "
		if index == m.routeIndex {
			cursor = ">"
		}
		lines = append(lines, fmt.Sprintf("%s %-22s %s / %s   %s", cursor, route.Bastion.Name, m.subscriptionName(route.Bastion.SubscriptionID), route.Bastion.ResourceGroup, route.RouteType))
	}
	action := []string{"connect", "transfer", "mount"}[m.routeAction]
	focus := lipgloss.Height(lipgloss.Wrap(strings.Join(lines[:2+m.routeIndex], "\n"), m.screenWidth()-4, " /,="))
	return m.secondaryFrame(strings.Join(lines, "\n"), []shortcut{{"enter", action}, {"esc", "cancel"}, {"?", "help"}, {"q", "quit"}}, focus)
}

func (m Model) mountPathView() string {
	content, _ := m.renderMountPath()
	return content
}

func (m Model) renderMountPath() (string, int) {
	if m.routeTarget == nil || len(m.routeTarget.Routes) != 1 {
		return m.mainScreen(), 0
	}
	route := m.routeTarget.Routes[0]
	lines := []string{
		accentStyle.Render("Mount " + m.routeTarget.VM.Name),
		"",
		"Choose the remote directory to mount at " + "~/azssh/mnt/" + m.routeTarget.VM.Name + ".",
		"Bastion: " + route.Bastion.Name + " / " + route.Bastion.ResourceGroup,
		"",
		m.mountPath.View(),
		"",
	}
	if m.status != "" {
		lines = append(lines, m.statusStyle().Render(m.status))
	}
	focus := lipgloss.Height(lipgloss.Wrap(strings.Join(lines[:5], "\n"), max(1, m.screenWidth()-4), " /,="))
	return m.secondaryFrame(strings.Join(lines, "\n"), []shortcut{{"enter", "mount"}, {"esc", "cancel"}, {"ctrl+c", "quit"}}, focus)
}

func (m Model) helpView() string {
	content, _ := m.renderHelp()
	return content
}

func (m Model) renderHelp() (string, int) {
	lines := []string{
		accentStyle.Render("Help"),
		"",
		accentStyle.Render("Navigation"),
		shortcutLabel("up/k, down/j", "move selection"),
		shortcutLabel("/", "search VMs"),
		shortcutLabel("x", "toggle selected VM favorite"),
		shortcutLabel("f", "filter subscriptions"),
		shortcutLabel("?", "open or close help"),
		shortcutLabel("r", "refresh inventory"),
		shortcutLabel("U", "update when a newer release is available"),
		shortcutLabel("q, ctrl+c", "quit"),
		"",
		accentStyle.Render("Connection"),
		shortcutLabel("enter", "connect or select a Bastion route"),
		shortcutLabel("shift+enter", "review command in your shell"),
		shortcutLabel("b", "choose a Bastion route"),
		shortcutLabel("t", "open an Entra-only scp transfer shell"),
		shortcutLabel("m", "mount a directory with Entra SSHFS"),
		"", accentStyle.Render("Details"),
		shortcutLabel("d", "toggle full network IDs / readable names"),
		shortcutLabel("pgup/pgdown", "scroll details without moving VM selection"),
		"",
		accentStyle.Render("Filter subscriptions"),
		shortcutLabel("space", "toggle subscription visibility"),
		shortcutLabel("a / n", "show all / hide all"),
		shortcutLabel("enter", "apply filters"),
		shortcutLabel("esc", "cancel changes or close a view"),
	}
	if m.status != "" {
		lines = append(lines, "", accentStyle.Render("Current status"), m.status)
	}
	return m.secondaryFrame(strings.Join(lines, "\n"), []shortcut{{"esc", "back"}, {"?", "back"}, {"q", "quit"}}, -1)
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
	if m.vmList.Width() <= 0 {
		return
	}
	layout := m.finderLayout()
	m.vmList.SetSize(max(1, layout.leftWidth-4), max(1, layout.leftHeight-2))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
