package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"azssh/internal/inventory"
	"azssh/internal/release"
	"azssh/internal/shell"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Init begins cache validation after the first view is rendered.
func (m Model) Init() tea.Cmd {
	m.loading = true
	return tea.Batch(m.refreshCmd(), m.updateCheckCmd(), m.spinner.Tick)
}

// Update handles terminal input, background inventory refreshes, and shell
// completion messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.EnvMsg:
		useUnicode := unicodeFromEnv(msg.Getenv)
		if useUnicode != m.useUnicode {
			m.useUnicode = useUnicode
			m.vmList.SetDelegate(newTargetDelegate(useUnicode))
			m.rebuildList()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeList()
		return m, nil
	case refreshSucceededMsg:
		m.loading = false
		m.topology = msg.topology
		m.subscriptions = msg.topology.Subscriptions
		sort.Slice(m.subscriptions, func(i, j int) bool {
			return strings.ToLower(m.subscriptions[i].Name) < strings.ToLower(m.subscriptions[j].Name)
		})
		m.targets = msg.vmInventory.Targets
		m.lastRefresh = msg.vmInventory.FetchedAt
		m.rebuildList()
		m.status = fmt.Sprintf("Loaded %d eligible VMs (%d shown)", len(m.targets), len(m.vmList.Items()))
		if m.vmList.SettingFilter() {
			m.status += fmt.Sprintf("; %d matching", len(m.vmList.VisibleItems()))
		}
		return m, nil
	case refreshFailedMsg:
		m.loading = false
		m.status = "Refresh failed: " + msg.err.Error()
		return m, nil
	case shellFinishedMsg:
		if msg.err != nil {
			m.status = "Shell closed: " + msg.err.Error()
		} else {
			m.status = "Shell closed"
		}
		return m, nil
	case updateCheckSucceededMsg:
		m.availableUpdate = msg.latestVersion
		m.resizeList()
		return m, nil
	case tea.KeyPressMsg:
		return m.updateKey(msg)
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	if m.activeView == mainView {
		var cmd tea.Cmd
		m.vmList, cmd = m.vmList.Update(msg)
		m.updateSearchStatus()
		return m, cmd
	}
	return m, nil
}

func (m Model) updateCheckCmd() tea.Cmd {
	version := m.config.Version
	store := m.config.UpdateStore
	return func() tea.Msg {
		if store == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := release.CheckLatest(ctx, version, store)
		if err != nil || !result.Available {
			return nil
		}
		return updateCheckSucceededMsg{latestVersion: result.LatestVersion}
	}
}

func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.activeView {
	case helpView:
		if key.Matches(msg, m.keys.Help, m.keys.Cancel) {
			m.activeView = m.previousView
		}
		return m, nil
	case subscriptionFilterView:
		return m.updateSubscriptionFilter(msg)
	case routeSelectorView:
		return m.updateRouteSelector(msg)
	case mountPathView:
		return m.updateMountPath(msg)
	}

	if m.vmList.SettingFilter() {
		var cmd tea.Cmd
		m.vmList, cmd = m.vmList.Update(msg)
		m.updateSearchStatus()
		return m, cmd
	}
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if key.Matches(msg, m.keys.Help) {
		m.previousView, m.activeView = mainView, helpView
		return m, nil
	}
	if key.Matches(msg, m.keys.FilterSubscriptions) {
		m.openSubscriptionFilter()
		return m, nil
	}
	if key.Matches(msg, m.keys.Favorite) {
		if err := m.toggleFavorite(); err != nil {
			m.status = "Save favorites failed: " + err.Error()
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Refresh) {
		if m.loading {
			m.status = "Refresh already in progress"
			return m, nil
		}
		m.loading = true
		m.status = "Refreshing Azure inventory"
		return m, tea.Batch(m.refreshCmd(), m.spinner.Tick)
	}
	if key.Matches(msg, m.keys.Route) {
		if target := m.selectedTarget(); target != nil && len(target.Routes) > 1 {
			m.routeTarget, m.routeIndex, m.routeAction, m.activeView = target, 0, routeConnect, routeSelectorView
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Transfer) {
		if !strings.EqualFold(strings.TrimSpace(m.config.Authentication.Type), "AAD") && strings.TrimSpace(m.config.Authentication.Type) != "" {
			m.status = "File transfer requires AAD authentication"
			return m, nil
		}
		if target := m.selectedTarget(); target != nil {
			if len(target.Routes) > 1 {
				m.routeTarget, m.routeIndex, m.routeAction, m.activeView = target, 0, routeTransfer, routeSelectorView
				return m, nil
			}
			return m.startTransfer(target.Routes[0], target.VM)
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Mount) {
		if !usesAAD(m.config.Authentication) {
			m.status = "Mounting requires AAD authentication"
			return m, nil
		}
		if target := m.selectedTarget(); target != nil {
			if len(target.Routes) > 1 {
				m.routeTarget, m.routeIndex, m.routeAction, m.activeView = target, 0, routeMount, routeSelectorView
				return m, nil
			}
			m.openMountPath(target.Routes[0], target.VM)
		}
		return m, nil
	}
	if msg.Key().Code == tea.KeyEnter {
		if target := m.selectedTarget(); target != nil {
			if len(target.Routes) > 1 {
				m.routeTarget, m.routeIndex, m.routeAction, m.activeView = target, 0, routeConnect, routeSelectorView
				return m, nil
			}
			return m.startShell(target.Routes[0], target.VM, msg.Key().Mod&tea.ModShift != 0)
		}
	}

	var cmd tea.Cmd
	m.vmList, cmd = m.vmList.Update(msg)
	m.updateSearchStatus()
	return m, cmd
}

func (m *Model) updateSearchStatus() {
	if m.vmList.SettingFilter() {
		m.status = fmt.Sprintf("%d matching VMs", len(m.vmList.VisibleItems()))
		return
	}
	if strings.HasSuffix(m.status, "matching VMs") {
		m.status = ""
		return
	}
	if separator := strings.LastIndex(m.status, "; "); separator >= 0 && strings.HasSuffix(m.status, " matching") {
		m.status = m.status[:separator]
	}
}

func (m Model) updateSubscriptionFilter(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Help) {
		m.previousView, m.activeView = subscriptionFilterView, helpView
		return m, nil
	}
	if key.Matches(msg, m.keys.Cancel) {
		m.activeView = mainView
		return m, nil
	}
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if len(m.subscriptions) == 0 {
		return m, nil
	}
	switch msg.String() {
	case "up", "k":
		if m.filterIndex > 0 {
			m.filterIndex--
		}
	case "down", "j":
		if m.filterIndex < len(m.subscriptions)-1 {
			m.filterIndex++
		}
	default:
		switch {
		case key.Matches(msg, m.keys.Toggle):
			id := m.subscriptions[m.filterIndex].ID
			m.filterDraft[id] = !m.filterDraft[id]
		case key.Matches(msg, m.keys.All):
			m.filterDraft = make(map[string]bool)
		case key.Matches(msg, m.keys.None):
			for _, subscription := range m.subscriptions {
				m.filterDraft[subscription.ID] = true
			}
		case key.Matches(msg, m.keys.Apply):
			if err := m.applySubscriptionFilter(); err != nil {
				m.status = "Save filter failed: " + err.Error()
			}
		}
	}
	return m, nil
}

func (m Model) updateRouteSelector(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Help) {
		m.previousView, m.activeView = routeSelectorView, helpView
		return m, nil
	}
	if key.Matches(msg, m.keys.Cancel) {
		m.activeView, m.routeTarget, m.routeAction = mainView, nil, routeConnect
		return m, nil
	}
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if m.routeTarget == nil {
		m.activeView = mainView
		return m, nil
	}
	if msg.Key().Code == tea.KeyEnter {
		route := m.routeTarget.Routes[m.routeIndex]
		vm := m.routeTarget.VM
		action := m.routeAction
		m.activeView, m.routeTarget, m.routeAction = mainView, nil, routeConnect
		if action == routeTransfer {
			return m.startTransfer(route, vm)
		}
		if action == routeMount {
			m.openMountPath(route, vm)
			return m, nil
		}
		return m.startShell(route, vm, msg.Key().Mod&tea.ModShift != 0)
	}
	switch msg.String() {
	case "up", "k":
		if m.routeIndex > 0 {
			m.routeIndex--
		}
	case "down", "j":
		if m.routeIndex < len(m.routeTarget.Routes)-1 {
			m.routeIndex++
		}
	}
	return m, nil
}

func usesAAD(authentication shell.Authentication) bool {
	typeName := strings.TrimSpace(authentication.Type)
	return typeName == "" || strings.EqualFold(typeName, "AAD")
}

func (m *Model) openMountPath(route inventory.BastionRoute, vm inventory.VirtualMachine) {
	m.routeTarget = &inventory.EligibleTarget{VM: vm, Routes: []inventory.BastionRoute{route}}
	m.mountPath.SetValue(".")
	m.mountPath.Focus()
	m.activeView = mountPathView
}

func (m Model) updateMountPath(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) {
		m.mountPath.Blur()
		m.routeTarget = nil
		m.activeView = mainView
		return m, nil
	}
	if key.Matches(msg, m.keys.Quit) {
		return m, tea.Quit
	}
	if msg.Key().Code == tea.KeyEnter {
		remotePath := strings.TrimSpace(m.mountPath.Value())
		if remotePath == "" || strings.ContainsRune(remotePath, '\x00') || strings.Contains(remotePath, "\n") || strings.Contains(remotePath, "\r") {
			m.status = "Remote path must be a non-empty POSIX path"
			return m, nil
		}
		if m.routeTarget == nil || len(m.routeTarget.Routes) != 1 {
			m.activeView = mainView
			return m, nil
		}
		m.mountRequest = &mountRequest{route: m.routeTarget.Routes[0], vm: m.routeTarget.VM, remotePath: remotePath}
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.mountPath, cmd = m.mountPath.Update(msg)
	return m, cmd
}

func (m Model) startTransfer(route inventory.BastionRoute, vm inventory.VirtualMachine) (tea.Model, tea.Cmd) {
	m.transferRequest = &transferRequest{route: route, vm: vm}
	return m, tea.Quit
}

func (m Model) startShell(route inventory.BastionRoute, vm inventory.VirtualMachine, review bool) (tea.Model, tea.Cmd) {
	command, err := shell.Command(route, vm, m.config.Authentication)
	if err != nil {
		m.status = "Cannot start shell: " + err.Error()
		return m, nil
	}
	if review {
		m.reviewCommand = append([]string(nil), command.Args...)
		return m, tea.Quit
	} else {
		m.status = "Opening shell for " + vm.Name
	}
	return m, tea.ExecProcess(command, func(err error) tea.Msg { return shellFinishedMsg{err: err} })
}
