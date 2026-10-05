package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"azssh/internal/release"
	tea "charm.land/bubbletea/v2"
)

func TestSelfUpdateChoices(t *testing.T) {
	for _, tc := range []struct {
		name        string
		writable    bool
		index       int
		destination string
		sudo        bool
	}{
		{"writable", true, 0, "/current/azssh", false},
		{"local migration", false, 0, "/home/user/.local/bin/azssh", false},
		{"system", false, 1, "/current/azssh", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := Model{activeView: selfUpdateView, availableUpdate: "v1.2.3", updatePhase: "confirm", updateIndex: tc.index,
				installation: release.Installation{Current: "/current/azssh", Local: "/home/user/.local/bin/azssh", Writable: tc.writable}}
			updated, cmd := m.updateSelfUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			result := updated.(Model)
			defer result.updateCancel()
			if cmd == nil || result.updatePhase != "downloading" || result.updateDestination != tc.destination || result.updateSudo != tc.sudo {
				t.Fatalf("unexpected choice: %+v", result)
			}
			if !tc.writable && !strings.Contains(m.selfUpdateScreen(), "recommended") {
				t.Fatal("missing migration proposal")
			}
		})
	}
}

func TestSelfUpdateCancellationIgnoresStaleDownloads(t *testing.T) {
	cancelled := false
	m := Model{activeView: selfUpdateView, updatePhase: "downloading", updateGeneration: 1, updateCancel: func() { cancelled = true }}
	updated, _ := m.updateSelfUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m = updated.(Model)
	if !cancelled || m.activeView != mainView {
		t.Fatal("download not cancelled")
	}
	updated, cmd := m.handleUpdateDownloaded(updateDownloadedMsg{generation: 1, binary: []byte("stale")})
	if cmd != nil || updated.(Model).updatePhase == "installing" {
		t.Fatal("stale download initiated installation")
	}
}

func TestSelfUpdateInstallationAndCompletion(t *testing.T) {
	target := filepath.Join(t.TempDir(), "azssh")
	m := Model{activeView: selfUpdateView, updatePhase: "verifying", updateDestination: target, availableUpdate: "v1.2.3", config: Config{Version: "v1.2.2"}}
	updated, cmd := m.handleUpdateDownloaded(updateDownloadedMsg{binary: []byte("binary")})
	m = updated.(Model)
	if cmd == nil || m.updatePhase != "installing" {
		t.Fatal("installation not scheduled")
	}
	blocked, _ := m.updateSelfUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if blocked.(Model).activeView != selfUpdateView {
		t.Fatal("installation interrupted by navigation")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if !m.updateInstalled || m.availableUpdate != "" || m.config.Version != "v1.2.2" || !strings.Contains(m.updateText, "Restart azssh") {
		t.Fatalf("bad completion: %s", m.updateText)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "binary" {
		t.Fatal("wrong installed bytes")
	}
	updated, _ = m.Update(updateCheckSucceededMsg{latestVersion: "v1.2.3"})
	if updated.(Model).availableUpdate != "" {
		t.Fatal("old update banner returned")
	}
}

func TestSelfUpdateFailures(t *testing.T) {
	m := Model{updatePhase: "verifying"}
	updated, cmd := m.handleUpdateDownloaded(updateDownloadedMsg{err: errors.New("checksum mismatch")})
	if cmd != nil || updated.(Model).updatePhase != "failed" {
		t.Fatal("download failure attempted installation")
	}
	m.updatePhase = "installing"
	updated, _ = m.handleUpdateInstalled(updateInstalledMsg{err: errors.New("sudo cancelled")})
	if updated.(Model).updatePhase != "failed" || updated.(Model).updateInstalled {
		t.Fatal("sudo failure reported success")
	}
}

func TestUppercaseUpdateKeyAndCancelChoice(t *testing.T) {
	m := Model{keys: defaultKeyMap(), availableUpdate: "v1.2.3"}
	updated, cmd := m.updateKey(tea.KeyPressMsg(tea.Key{Code: 'u', ShiftedCode: 'U', Mod: tea.ModShift, Text: "U"}))
	if cmd == nil || updated.(Model).activeView != selfUpdateView {
		t.Fatal("U did not open update")
	}
	m = Model{activeView: selfUpdateView, updatePhase: "confirm", updateIndex: 2}
	updated, cmd = m.updateSelfUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	if cmd != nil || updated.(Model).activeView != mainView {
		t.Fatal("cancel choice started work")
	}
}

func TestMigrationCompletionIncludesGuidance(t *testing.T) {
	t.Setenv("PATH", "")
	m := Model{updatePhase: "installing", updateDestination: "/home/user/.local/bin/azssh", availableUpdate: "v1.2.3",
		installation: release.Installation{Current: "/usr/local/bin/azssh", Local: "/home/user/.local/bin/azssh"}}
	updated, _ := m.handleUpdateInstalled(updateInstalledMsg{})
	text := updated.(Model).updateText
	if !strings.Contains(text, "Put $HOME/.local/bin first") || !strings.Contains(text, "old system copy remains") {
		t.Fatalf("missing migration guidance: %s", text)
	}
}

func TestUpdateConfirmationOmitsPackageManagerAdvice(t *testing.T) {
	for _, writable := range []bool{true, false} {
		m := Model{updatePhase: "confirm", installation: release.Installation{
			Current: "/usr/local/bin/azssh", Local: "/home/user/.local/bin/azssh", Writable: writable,
		}}
		view := m.selfUpdateScreen()
		if strings.Contains(view, "package manager") || strings.Contains(view, "Standalone releases only") {
			t.Fatal("confirmation contains advice for nonexistent distribution channels")
		}
		if !strings.Contains(view, "Release details") {
			t.Fatal("release details were removed along with the disclaimer")
		}
	}
}
