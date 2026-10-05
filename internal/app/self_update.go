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

type updateFetchedMsg struct {
	generation int
	artifact   release.Artifact
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
		m.updatePhase, m.updateText = "downloading", "Downloading release assets…"
		m.secondaryOffset, m.secondaryManualScroll = 0, false
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		m.updateCancel = cancel
		version, generation := m.availableUpdate, m.updateGeneration
		return m, func() tea.Msg {
			defer cancel()
			artifact, err := release.Fetch(ctx, version)
			return updateFetchedMsg{generation: generation, artifact: artifact, err: err}
		}
	}
	return m, nil
}

func (m Model) handleUpdateFetched(msg updateFetchedMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "downloading" {
		return m, nil
	}
	if msg.err != nil {
		m.updateCancel = nil
		m.updatePhase, m.updateText = "failed", "Download failed: "+msg.err.Error()+"\nCheck connectivity and release availability, then return and try again."
		return m, nil
	}
	m.updatePhase, m.updateText = "verifying", "Verifying SHA-256 and extracting the release…"
	m.secondaryOffset, m.secondaryManualScroll = 0, false
	generation := m.updateGeneration
	return m, func() tea.Msg {
		binary, err := release.Verify(msg.artifact)
		return updateDownloadedMsg{generation: generation, binary: binary, err: err}
	}
}

func (m Model) handleUpdateDownloaded(msg updateDownloadedMsg) (tea.Model, tea.Cmd) {
	if msg.generation != m.updateGeneration || m.updatePhase != "verifying" {
		return m, nil
	}
	m.updateCancel = nil
	if msg.err != nil {
		m.updatePhase, m.updateText = "failed", "Verification failed: "+msg.err.Error()+"\nThe executable was not replaced. Return and retry with a fresh download."
		return m, nil
	}
	m.updatePhase, m.updateText = "installing", "Installing verified release at "+m.updateDestination+"…"
	m.secondaryOffset, m.secondaryManualScroll = 0, false
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
	m.secondaryOffset, m.secondaryManualScroll = 0, false
	if msg.err != nil {
		m.updatePhase, m.updateText = "failed", "Installation failed or sudo cancelled: "+msg.err.Error()+"\nCheck destination permissions or retry the chosen installation method."
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
	return m, nil
}

func (m Model) selfUpdateScreen() string {
	content, _ := m.renderSelfUpdate()
	return content
}

func (m Model) renderSelfUpdate() (string, int) {
	lines := []string{accentStyle.Render("Update azssh"), ""}
	if m.updatePhase == "confirm" {
		transition := " -> "
		if m.useUnicode {
			transition = " → "
		}
		lines = append(lines, m.config.Version+transition+m.availableUpdate, "", "Current location", m.installation.Current, "")
		var options, details []string
		if m.installation.Writable {
			options = []string{"Update in place", "Cancel"}
			details = []string{m.installation.Current + " · no sudo", ""}
		} else {
			lines = append(lines, "This location requires administrator privileges.", "")
			options = []string{"Install for my user (recommended)", "Update system installation", "Cancel"}
			details = []string{m.installation.Local + " · no sudo", m.installation.Current + " · sudo required", ""}
		}
		focus := -1
		for index, option := range options {
			cursor := "  "
			if index == m.updateIndex {
				cursor = "> "
			}
			label := cursor + option
			if index == m.updateIndex {
				label = accentStyle.Render(label)
			}
			lines = append(lines, label)
			if details[index] != "" {
				lines = append(lines, mutedStyle.Render("    "+details[index]))
			}
			if index == m.updateIndex {
				focus = lipgloss.Height(lipgloss.Wrap(strings.Join(lines, "\n"), max(1, m.screenWidth()-4), " /,=")) - 1
			}
		}
		lines = append(lines, "", "Release details", "https://github.com/reinier-vegter/azssh/releases")
		return m.secondaryFrame(strings.Join(lines, "\n"), []shortcut{{"up/down", "choose"}, {"enter", "confirm"}, {"esc", "back"}, {"q", "quit"}}, focus)
	} else {
		text := m.updateText
		if m.updatePhase == "failed" {
			parts := strings.SplitN(text, "\n", 2)
			text = errorStyle.Render(parts[0])
			if len(parts) == 2 {
				text += "\n" + parts[1]
			}
		}
		lines = append(lines, text)
		controls := []shortcut{}
		if m.updatePhase == "downloading" || m.updatePhase == "verifying" {
			controls = append(controls, shortcut{"esc", "cancel"}, shortcut{"q", "quit"})
		} else if m.updatePhase != "installing" {
			controls = append(controls, shortcut{"esc", "back"}, shortcut{"q", "quit"})
		}
		return m.secondaryFrame(strings.Join(lines, "\n"), controls, -1)
	}
}
