package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"azssh/internal/release"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type installationInspectedMsg struct {
	generation   int
	installation release.Installation
	err          error
}

type updateDownloadedMsg struct {
	generation int
	binary     []byte
	err        error
}

type updateInstalledMsg struct {
	generation int
	err        error
}

func (m Model) openSelfUpdate() (tea.Model, tea.Cmd) {
	if m.availableUpdate == "" {
		m.status = "No newer release is available"
		return m, nil
	}
	m.updateGeneration++
	generation := m.updateGeneration
	m.activeView = selfUpdateView
	m.updatePhase, m.updateText, m.updateIndex = "inspecting", "Checking installation location…", 0
	return m, func() tea.Msg {
		installation, err := release.InspectInstallation()
		return installationInspectedMsg{generation: generation, installation: installation, err: err}
	}
}

func (m Model) handleInstallationInspected(msg installationInspectedMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "inspecting" {
		return m, nil
	}
	if msg.err != nil {
		m.updatePhase, m.updateText = "failed", "Cannot inspect installation: "+msg.err.Error()
		return m, nil
	}
	m.installation = msg.installation
	m.updatePhase, m.updateText = "confirm", ""
	return m, nil
}

func (m Model) updateSelfUpdate(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.updatePhase == "installing" {
		return m, nil
	}
	if msg.String() == "esc" || msg.String() == "q" || msg.String() == "ctrl+c" {
		if m.updateCancel != nil {
			m.updateCancel()
			m.updateCancel = nil
		}
		m.updateGeneration++
		m.activeView = mainView
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m, nil
	}
	if m.updatePhase != "confirm" {
		return m, nil
	}
	last := 1
	if !m.installation.Writable {
		last = 2
	}
	switch msg.String() {
	case "up", "k":
		if m.updateIndex > 0 {
			m.updateIndex--
		}
	case "down", "j":
		if m.updateIndex < last {
			m.updateIndex++
		}
	case "enter":
		if m.updateIndex == last {
			m.activeView = mainView
			return m, nil
		}
		m.updateDestination = m.installation.Current
		m.updateSudo = !m.installation.Writable && m.updateIndex == 1
		if !m.installation.Writable && !m.updateSudo {
			m.updateDestination = m.installation.Local
		}
		m.updatePhase, m.updateText = "downloading", "Downloading and verifying release… (esc cancels)"
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		m.updateCancel = cancel
		version, generation := m.availableUpdate, m.updateGeneration
		return m, func() tea.Msg {
			defer cancel()
			binary, err := release.Download(ctx, version)
			return updateDownloadedMsg{generation: generation, binary: binary, err: err}
		}
	}
	return m, nil
}

func (m Model) handleUpdateDownloaded(msg updateDownloadedMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "downloading" {
		return m, nil
	}
	m.updateCancel = nil
	if msg.err != nil {
		m.updatePhase, m.updateText = "failed", "Update failed: "+msg.err.Error()
		return m, nil
	}
	m.updatePhase, m.updateText = "installing", "Installing verified release at "+m.updateDestination+"…"
	generation, destination := m.updateGeneration, m.updateDestination
	if m.updateSudo {
		command, err := release.SudoCommand(destination, msg.binary)
		if err != nil {
			return m.handleUpdateInstalled(updateInstalledMsg{generation: generation, err: err})
		}
		return m, tea.ExecProcess(command, func(err error) tea.Msg {
			return updateInstalledMsg{generation: generation, err: err}
		})
	}
	return m, func() tea.Msg {
		return updateInstalledMsg{generation: generation, err: release.Install(destination, msg.binary)}
	}
}

func (m Model) handleUpdateInstalled(msg updateInstalledMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "installing" {
		return m, nil
	}
	if msg.err != nil {
		m.updatePhase, m.updateText = "failed", "Update failed or sudo cancelled: "+msg.err.Error()
		return m, nil
	}
	m.updatePhase = "done"
	m.updateText = fmt.Sprintf("Installed %s at %s.\nRestart azssh to use the new version; this process is still %s.", m.availableUpdate, m.updateDestination, m.config.Version)
	if m.updateDestination == m.installation.Local && m.updateDestination != m.installation.Current {
		m.updateText += "\n\n" + release.LocalPathGuidance(m.updateDestination)
		m.updateText += "\n\nThe old system copy remains at " + m.installation.Current + ".\nAfter verifying the local installation, you may remove the old standalone copy manually using sudo."
	}
	m.updateInstalled = true
	m.availableUpdate = ""
	m.resizeList()
	return m, nil
}

func (m Model) selfUpdateScreen() string {
	lines := []string{accentStyle.Render("Update azssh"), ""}
	if m.updatePhase == "confirm" {
		lines = append(lines, fmt.Sprintf("Update %s → %s?", m.config.Version, m.availableUpdate), "Current installation: "+m.installation.Current, "")
		var options []string
		if m.installation.Writable {
			options = []string{"Update " + m.installation.Current, "Cancel"}
		} else {
			lines = append(lines, "Updating this location requires administrator privileges.", "")
			options = []string{"Install in " + m.installation.Local + " instead (recommended)", "Update " + m.installation.Current + " using sudo", "Cancel"}
		}
		for index, option := range options {
			cursor := "  "
			if index == m.updateIndex {
				cursor = "> "
			}
			lines = append(lines, cursor+option)
		}
		lines = append(lines, "", "Standalone release binaries only; use your package manager for managed installations.", mutedStyle.Render("up/down choose  enter confirm  esc cancel"))
	} else {
		lines = append(lines, m.updateText)
		if m.updatePhase != "installing" {
			lines = append(lines, "", mutedStyle.Render("esc return"))
		}
	}
	content := strings.Join(lines, "\n")
	if m.width > 0 {
		content = lipgloss.Wrap(content, max(20, m.width-6), " /,=")
	}
	return panelStyle.Render(content)
}
