package app

import (
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type targetDelegate struct {
	list.DefaultDelegate
	useUnicode bool
}

func newTargetDelegate(useUnicode bool) targetDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetHeight(2)
	delegate.SetSpacing(0)
	delegate.Styles.NormalTitle = lipgloss.NewStyle().PaddingLeft(2)
	delegate.Styles.NormalDesc = mutedStyle.PaddingLeft(2)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().Bold(true).
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderLeftForeground(lipgloss.Color("39")).PaddingLeft(1)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedTitle.Bold(false).Foreground(lipgloss.Color("245"))
	delegate.Styles.DimmedTitle = mutedStyle.PaddingLeft(2)
	delegate.Styles.DimmedDesc = mutedStyle.PaddingLeft(2)
	return targetDelegate{DefaultDelegate: delegate, useUnicode: useUnicode}
}

func (d targetDelegate) Height() int {
	return d.DefaultDelegate.Height() + 1
}

func (d targetDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	d.DefaultDelegate.Render(w, m, index, item)
	fmt.Fprint(w, "\n")
	if d.isFavoriteBoundary(m.VisibleItems(), index) {
		line := "-"
		if d.useUnicode {
			line = "─"
		}
		fmt.Fprint(w, mutedStyle.Render(strings.Repeat(line, max(1, m.Width()-4))))
	}
}

func (d targetDelegate) isFavoriteBoundary(items []list.Item, index int) bool {
	current, ok := items[index].(targetItem)
	if !ok || !current.favorite || index+1 >= len(items) {
		return false
	}
	next, ok := items[index+1].(targetItem)
	return ok && !next.favorite
}

func favoriteMarker(useUnicode bool) string {
	if useUnicode {
		return favoriteStyle.Render("★")
	}
	return favoriteStyle.Render("*")
}

func (d targetDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return d.DefaultDelegate.Update(msg, m)
}
