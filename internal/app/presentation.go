package app

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

type shortcut struct{ key, action string }

func shortcutLabel(key, action string) string {
	return accentStyle.Render(key) + ": " + action
}

// Pack complete controls, never individual words or ANSI bytes.
func shortcutRows(width int, controls ...shortcut) string {
	var rows []string
	line := ""
	for _, control := range controls {
		label := shortcutLabel(control.key, control.action)
		separator := "  "
		if line == "" {
			separator = ""
		}
		if line != "" && lipgloss.Width(line+separator+label) > width {
			rows = append(rows, line)
			line, separator = "", ""
		}
		line += separator + label
	}
	if line != "" {
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (m Model) screenWidth() int {
	if m.width <= 0 {
		return 100
	}
	return m.width
}

func (m Model) screenHeight() int {
	if m.height <= 0 {
		return 30
	}
	return m.height
}

func (m Model) mainHeader() string {
	width := m.screenWidth()
	identity := accentStyle.Render("azssh")
	if m.config.Version != "" {
		identity += " " + mutedStyle.Render(m.config.Version)
	}
	cache := m.cacheStatus()
	notice := ""
	if m.availableUpdate != "" {
		notice = notificationStyle.Render(m.availableUpdate + " available")
		if !m.vmList.SettingFilter() {
			notice += "  " + shortcutLabel("U", "update")
		}
	}
	if notice != "" && lipgloss.Width(identity)+lipgloss.Width(notice)+lipgloss.Width(cache)+4 <= width {
		left := identity + "  " + notice
		return left + strings.Repeat(" ", width-lipgloss.Width(left)-lipgloss.Width(cache)) + cache
	}
	gap := width - lipgloss.Width(identity) - lipgloss.Width(cache)
	line := identity
	if gap >= 2 {
		line += strings.Repeat(" ", gap) + cache
	} else {
		line += "\n" + cache
	}
	if notice != "" {
		line += "\n" + notice
	}
	return line
}

func (m Model) mainControls(scroll bool) []shortcut {
	if m.vmList.SettingFilter() {
		return []shortcut{{"enter", "apply search"}, {"esc", "cancel search"}, {"ctrl+c", "quit"}}
	}
	controls := []shortcut{}
	if target := m.selectedTarget(); target != nil {
		controls = append(controls, shortcut{"enter", "connect"})
		if usesAAD(m.config.Authentication) {
			controls = append(controls, shortcut{"t", "transfer"}, shortcut{"m", "mount"})
		}
		controls = append(controls, shortcut{"shift+enter", "review"}, shortcut{"/", "search"}, shortcut{"x", "favorite"}, shortcut{"f", "filters"})
		if len(target.Routes) > 1 {
			controls = append(controls, shortcut{"b", "route"})
		}
		if len(target.VM.SubnetIDs)+len(target.VM.VNetIDs) > 0 {
			action := "full IDs"
			if m.fullNetworkIDs {
				action = "names"
			}
			controls = append(controls, shortcut{"d", action})
		}
	} else {
		controls = append(controls, shortcut{"/", "search"}, shortcut{"f", "filters"})
	}
	if scroll {
		controls = append(controls, shortcut{"pgup/pgdown", "details"})
	}
	if !m.loading {
		controls = append(controls, shortcut{"r", "refresh"})
	}
	if m.availableUpdate != "" {
		controls = append(controls, shortcut{"U", "update"})
	}
	return append(controls, shortcut{"?", "help"}, shortcut{"q", "quit"})
}

type finderLayout struct {
	header, footer                                 string
	leftWidth, rightWidth, leftHeight, rightHeight int
	stacked, tooSmall                              bool
}

func (m Model) finderLayout() finderLayout {
	width, height := m.screenWidth(), m.screenHeight()
	layout := finderLayout{header: m.mainHeader(), stacked: width < 86}
	if layout.stacked {
		layout.leftWidth, layout.rightWidth = width, width
	} else {
		layout.leftWidth = (width - 2) / 2
		layout.rightWidth = width - 2 - layout.leftWidth
	}
	scroll := false
	for pass := 0; pass < 2; pass++ {
		layout.footer = shortcutRows(width, m.mainControls(scroll)...)
		if m.status != "" {
			status := strings.Split(lipgloss.Wrap(m.status, width, " "), "\n")
			if len(status) > 2 {
				status = []string{status[0], "Full status in Help"}
			}
			layout.footer += "\n" + m.statusStyle().Render(strings.Join(status, "\n"))
		}
		bodyHeight := height - lipgloss.Height(layout.header) - 1 - lipgloss.Height(layout.footer) - 1
		layout.leftHeight, layout.rightHeight = bodyHeight, bodyHeight
		if layout.stacked {
			layout.leftHeight = bodyHeight / 2
			layout.rightHeight = bodyHeight - layout.leftHeight
		}
		scroll = lipgloss.Height(m.detailView()) > layout.rightHeight-2
	}
	layout.tooSmall = width < 32 || layout.leftHeight < 7 || layout.rightHeight < 5
	return layout
}

func viewportText(content string, width, height, offset int) string {
	view := viewport.New(viewport.WithWidth(max(1, width)), viewport.WithHeight(max(1, height)))
	view.SetContent(content)
	view.SetYOffset(offset)
	return view.View()
}

func renderPanel(content string, width, height int) string {
	return panelStyle.Width(width).Height(height).Render(content)
}

func (m Model) smallScreen() string {
	width, height := m.screenWidth(), m.screenHeight()
	quit := "q: quit"
	if m.activeView == mountPathView || (m.activeView == mainView && m.vmList.SettingFilter()) {
		quit = "ctrl+c: quit"
	}
	if m.activeView == selfUpdateView && m.updatePhase == "installing" {
		quit = "installation in progress"
	}
	text := lipgloss.Wrap("Terminal too small. Resize to show the finder and controls. "+quit, width, " ")
	if lipgloss.Height(text) > height {
		text = lipgloss.Wrap("Resize; "+quit, width, " ")
	}
	if lipgloss.Height(text) > height {
		text = lipgloss.Wrap(quit, width, " ")
		if lipgloss.Height(text) > height {
			text = "R"
		} // Too small even for a control: resize is the only useful instruction.
	}
	return text
}

// Secondary screens retain their controls while long content scrolls.
func (m Model) secondaryFrame(content string, controls []shortcut, focusLine int) (string, int) {
	width, height := m.screenWidth(), m.screenHeight()
	if width < 32 || height < 8 {
		return m.smallScreen(), 0
	}
	inner := width - 4
	content = lipgloss.Wrap(content, inner, " /,=")
	footer := shortcutRows(width, controls...)
	available := height - lipgloss.Height(footer) - 3
	if lipgloss.Height(content) > available {
		controls = append(controls, shortcut{"pgup/pgdown", "scroll"})
		footer = shortcutRows(width, controls...)
		available = height - lipgloss.Height(footer) - 3
	}
	if available < 2 {
		return m.smallScreen(), 0
	}
	offset := m.secondaryOffset
	if focusLine >= 0 && !m.secondaryManualScroll {
		if focusLine < offset {
			offset = focusLine
		}
		if focusLine >= offset+available {
			offset = focusLine - available + 1
		}
	}
	return renderPanel(viewportText(content, inner, available, offset), width, available+2) + "\n" + footer, max(0, lipgloss.Height(content)-available)
}

func (m Model) secondaryScrollLimit() int {
	var limit int
	switch m.activeView {
	case helpView:
		_, limit = m.renderHelp()
	case selfUpdateView:
		_, limit = m.renderSelfUpdate()
	case subscriptionFilterView:
		_, limit = m.renderSubscriptionFilter()
	case routeSelectorView:
		_, limit = m.renderRouteSelector()
	case mountPathView:
		_, limit = m.renderMountPath()
	}
	return limit
}

type detailRow struct{ label, value string }

func renderDetailRows(rows []detailRow, width int) []string {
	labelWidth := 0
	for _, row := range rows {
		if row.value != "" && lipgloss.Width(row.label) > labelWidth {
			labelWidth = lipgloss.Width(row.label)
		}
	}
	var lines []string
	for _, row := range rows {
		if row.value == "" {
			continue
		}
		if width-labelWidth-2 < 16 {
			lines = append(lines, mutedStyle.Render(lipgloss.Wrap(row.label, width, " ")))
			for _, value := range strings.Split(lipgloss.Wrap(row.value, max(1, width-2), " /,="), "\n") {
				lines = append(lines, "  "+value)
			}
			continue
		}
		prefix := row.label + strings.Repeat(" ", labelWidth-lipgloss.Width(row.label)+2)
		values := strings.Split(lipgloss.Wrap(row.value, width-labelWidth-2, " /,="), "\n")
		for index, value := range values {
			if index == 0 {
				lines = append(lines, mutedStyle.Render(prefix)+value)
			} else {
				lines = append(lines, strings.Repeat(" ", labelWidth+2)+value)
			}
		}
	}
	return lines
}

func resourcePart(id, kind string) string {
	parts := strings.Split(strings.Trim(id, "/"), "/")
	for index := 0; index+1 < len(parts); index++ {
		if strings.EqualFold(parts[index], kind) {
			return parts[index+1]
		}
	}
	return ""
}

func resourceName(id string) string {
	parts := strings.Split(strings.TrimRight(id, "/"), "/")
	return parts[len(parts)-1]
}

// Only shorten IDs when their displayed identity remains distinguishable.
func networkNames(ids []string, full bool) string {
	if full {
		return strings.Join(ids, ", ")
	}
	labels := make([]string, len(ids))
	counts := make(map[string]int)
	for _, id := range ids {
		counts[strings.ToLower(resourceName(id))]++
	}
	for index, id := range ids {
		name := resourceName(id)
		if counts[strings.ToLower(name)] > 1 {
			context := []string{}
			for _, kind := range []string{"virtualNetworks", "resourceGroups", "subscriptions"} {
				if value := resourcePart(id, kind); value != "" && !strings.EqualFold(value, name) {
					context = append(context, value)
				}
			}
			if len(context) == 0 {
				labels[index] = id
			} else {
				labels[index] = fmt.Sprintf("%s (%s)", name, strings.Join(context, " / "))
			}
		} else {
			labels[index] = name
		}
	}
	for index, label := range labels {
		for other := index + 1; other < len(labels); other++ {
			if strings.EqualFold(label, labels[other]) && ids[index] != ids[other] {
				labels[index], labels[other] = ids[index], ids[other]
			}
		}
	}
	return strings.Join(labels, ", ")
}
