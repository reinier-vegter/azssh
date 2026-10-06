package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"azssh/internal/cache"
	"azssh/internal/inventory"
	"azssh/internal/release"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func finderFixture() Model {
	vm := inventory.VirtualMachine{
		ID: "vm-1", Name: "api-nonprod-weu-01", SubscriptionID: "sub-1", ResourceGroup: "rg-api-nonprod-weu-01",
		Location: "westeurope", OSType: "Linux", Size: "Standard_D4s_v5", PrivateIPs: []string{"10.0.0.4"},
		VNetIDs:      []string{"/subscriptions/sub-1/resourceGroups/rg-network/providers/Microsoft.Network/virtualNetworks/vnet-nonprod-weu-01"},
		SubnetIDs:    []string{"/subscriptions/sub-1/resourceGroups/rg-network/providers/Microsoft.Network/virtualNetworks/vnet-nonprod-weu-01/subnets/snet-api"},
		OSDiskSizeGB: 128, OSDiskStorageType: "StandardSSD_LRS", Tags: map[string]string{"ApplicationID": "APM000123", "SubscriptionEnvironment": "nonprod"},
	}
	routes := []inventory.BastionRoute{{Bastion: inventory.Bastion{Name: "bastion-nonprod-weu-01", ResourceGroup: "rg-shared-network"}, RouteType: "same VNet"}}
	targets := []inventory.EligibleTarget{{VM: vm, Routes: routes}}
	for i := 2; i <= 9; i++ {
		copy := vm
		copy.ID = fmt.Sprintf("vm-%d", i)
		copy.Name = fmt.Sprintf("worker-nonprod-weu-%02d", i)
		targets = append(targets, inventory.EligibleTarget{VM: copy, Routes: routes})
	}
	m := NewModel(nil, nil, Config{Version: "v0.0.1"}, cache.TopologySnapshot{Subscriptions: []inventory.Subscription{{ID: "sub-1", Name: "Sandbox"}}}, cache.VMInventorySnapshot{Targets: targets}, cache.Preferences{FavoriteVMIDs: []string{"vm-1"}})
	m.availableUpdate, m.loading, m.lastRefresh = "v0.0.8", false, time.Now()
	m.status, m.useUnicode = "Loaded 9 eligible VMs (9 shown)", true
	return m
}

func assertFits(t *testing.T, view string, width, height int) {
	t.Helper()
	if lipgloss.Width(view) > width || lipgloss.Height(view) > height {
		t.Fatalf("rendered %dx%d exceeds %dx%d:\n%s", lipgloss.Width(view), lipgloss.Height(view), width, height, ansi.Strip(view))
	}
}

func TestFinderResponsiveLayout(t *testing.T) {
	for _, size := range [][2]int{{200, 65}, {120, 40}, {100, 30}, {86, 24}, {80, 24}, {60, 32}, {40, 40}, {32, 48}, {20, 12}, {80, 10}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := finderFixture()
			updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = updated.(Model)
			view := m.View().Content
			assertFits(t, view, size[0], size[1])
			plain := ansi.Strip(view)
			if m.finderLayout().tooSmall {
				if !strings.Contains(plain, "Terminal too small") {
					t.Fatal("missing resize hint")
				}
			} else {
				for _, text := range []string{"azssh v0.0.1", "v0.0.8 available", "U: update", "q: quit"} {
					if !strings.Contains(plain, text) {
						t.Fatalf("missing %s", text)
					}
				}
				layout := m.finderLayout()
				if !layout.stacked && layout.leftHeight != layout.rightHeight {
					t.Fatal("unequal panels")
				}
				if strings.Contains(plain, "https://") {
					t.Fatal("release URL leaked into finder")
				}
			}
			// Optional artifacts for visual review; no Azure or sudo is involved.
			if dir := os.Getenv("AZSSH_RENDER_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("finder-%dx%d.ansi", size[0], size[1])), []byte(view), 0644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestMainHeaderAndSearchOwnership(t *testing.T) {
	m := finderFixture()
	m.width, m.height = 120, 40
	if lipgloss.Height(m.mainHeader()) != 1 {
		t.Fatal("wide header should fit one line")
	}
	m.width = 40
	if lipgloss.Height(m.mainHeader()) != 2 {
		t.Fatal("narrow notice should use second line")
	}
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: '/', Text: "/"}))
	m = updated.(Model)
	if !m.vmList.SettingFilter() {
		t.Fatal("search not active")
	}
	for _, text := range []string{"U: update", "d: full IDs", "enter: connect"} {
		if strings.Contains(ansi.Strip(m.mainScreen()), text) {
			t.Fatalf("inactive shortcut advertised: %s", text)
		}
	}
	for _, char := range []rune{'U', 'd', 'q'} {
		updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: char, Text: string(char)}))
		m = updated.(Model)
	}
	if m.activeView != mainView || m.fullNetworkIDs || m.vmList.FilterInput.Value() != "Udq" {
		t.Fatal("shortcuts intercepted search")
	}
}

func TestDetailsDisclosureScrollAndHelp(t *testing.T) {
	m := finderFixture()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m = updated.(Model)
	if strings.Contains(m.detailView(), "/subscriptions/") {
		t.Fatal("full IDs shown by default")
	}
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'd', Text: "d"}))
	m = updated.(Model)
	if !strings.Contains(m.detailView(), "/subscriptions/") || !m.fullNetworkIDs {
		t.Fatal("IDs not expanded")
	}
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
	m = updated.(Model)
	if m.detailOffset == 0 || m.vmList.Index() != 0 {
		t.Fatal("detail scroll failed or moved VM selection")
	}
	offset := m.detailOffset
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '?', Text: "?"}))
	m = updated.(Model)
	if strings.Contains(ansi.Strip(m.View().Content), "v0.0.8 available") {
		t.Fatal("update notice repeated in Help")
	}
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m = updated.(Model)
	if m.detailOffset != offset || !m.fullNetworkIDs {
		t.Fatal("Help lost detail state")
	}
	updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m = updated.(Model)
	if m.fullNetworkIDs || m.detailOffset != 0 || m.vmList.Index() != 1 {
		t.Fatal("selection did not reset detail state")
	}
}

func TestMetadataWrappingAndResourceDisambiguation(t *testing.T) {
	lines := renderDetailRows([]detailRow{{"Label", strings.Repeat("long value ", 8)}, {"Other", "x"}}, 38)
	for _, line := range lines {
		if lipgloss.Width(line) > 38 {
			t.Fatal("metadata overflow")
		}
	}
	if !strings.HasPrefix(ansi.Strip(lines[1]), "       ") {
		t.Fatal("continuation not aligned beneath value")
	}
	lines = renderDetailRows([]detailRow{{"SubscriptionEnvironment", "nonprod"}}, 28)
	if len(lines) != 2 || ansi.Strip(lines[1]) != "  nonprod" {
		t.Fatalf("long label not stacked: %v", lines)
	}
	lines = renderDetailRows([]detailRow{{"環境", "production"}, {"Name", "café"}}, 38)
	if !strings.HasPrefix(ansi.Strip(lines[0]), "環境  ") {
		t.Fatal("labels not measured in terminal cells")
	}
	ids := []string{"/subscriptions/a/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/vnet-a/subnets/apps", "/subscriptions/b/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/vnet-b/subnets/apps"}
	names := networkNames(ids, false)
	if !strings.Contains(names, "vnet-a") || !strings.Contains(names, "vnet-b") {
		t.Fatal("duplicate subnet names not disambiguated")
	}
	if networkNames(ids, true) != strings.Join(ids, ", ") {
		t.Fatal("full identifiers were altered")
	}
}

func TestSecondaryScreensFitAndKeepControls(t *testing.T) {
	for _, size := range [][2]int{{120, 30}, {80, 24}, {40, 24}, {32, 20}} {
		for _, active := range []view{helpView, selfUpdateView, mountPathView, subscriptionFilterView, routeSelectorView} {
			m := finderFixture()
			m.width, m.height, m.activeView = size[0], size[1], active
			m.updatePhase = "confirm"
			m.installation = release.Installation{Current: "/usr/local/bin/azssh", Supported: true}
			m.routeTarget = &m.targets[0]
			m.filterDraft = map[string]bool{}
			view := m.View().Content
			assertFits(t, view, size[0], size[1])
			if strings.Contains(ansi.Strip(view), "v0.0.8 available") {
				t.Fatal("secondary notification repeated")
			}
			if !strings.Contains(ansi.Strip(view), "esc: ") {
				t.Fatalf("secondary footer missing: %s", ansi.Strip(view))
			}
			if dir := os.Getenv("AZSSH_RENDER_DIR"); dir != "" && active == selfUpdateView {
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("update-%dx%d.ansi", size[0], size[1])), []byte(view), 0644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestSecondaryScrollingIsBoundedAndMountInputOwnsText(t *testing.T) {
	m := finderFixture()
	m.width, m.height, m.activeView = 40, 20, helpView
	for i := 0; i < 30; i++ {
		updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
		m = updated.(Model)
	}
	if m.secondaryOffset == 0 || m.secondaryOffset > m.secondaryScrollLimit() {
		t.Fatal("secondary scrolling is not bounded")
	}
	before := m.secondaryOffset
	updated, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyPgUp}))
	m = updated.(Model)
	if m.secondaryOffset >= before {
		t.Fatal("page up was stuck after repeated page down")
	}
	m.openMountPath(m.targets[0].Routes[0], m.targets[0].VM)
	updated, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
	m = updated.(Model)
	if m.activeView != mountPathView || !strings.HasSuffix(m.mountPath.Value(), "q") {
		t.Fatal("q was intercepted in the remote-path field")
	}
	_ = cmd // Cursor commands do not need a running terminal in this test.
}

func TestUpdateStagesAreRealAndCancellationSafe(t *testing.T) {
	m := Model{activeView: selfUpdateView, updatePhase: "downloading", updateGeneration: 1}
	updated, cmd := m.handleUpdateFetched(updateFetchedMsg{generation: 1})
	m = updated.(Model)
	if cmd == nil || m.updatePhase != "verifying" {
		t.Fatal("fetch did not enter verification")
	}
	updated, install := m.handleUpdateDownloaded(cmd().(updateDownloadedMsg))
	if install != nil || updated.(Model).updatePhase != "failed" {
		t.Fatal("unverified bytes reached installation")
	}
	m.updatePhase = "downloading"
	updated, _ = m.updateSelfUpdate(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m = updated.(Model)
	_, cmd = m.handleUpdateFetched(updateFetchedMsg{generation: 1})
	if cmd != nil {
		t.Fatal("cancelled fetch started verification")
	}
}

func TestMetadataAndActionsAreNotDuplicated(t *testing.T) {
	m := finderFixture()
	plain := ansi.Strip(m.detailView())
	if strings.Contains(plain, "enter:") || !strings.Contains(plain, "One eligible Bastion route") {
		t.Fatal("detail pane repeats actions instead of contextual summary")
	}
	delegate := newTargetDelegate(true)
	if delegate.Styles.SelectedTitle.GetBackground() != (lipgloss.NoColor{}) || m.vmList.Styles.Title.GetBackground() != (lipgloss.NoColor{}) {
		t.Fatal("component title/selection has a filled background")
	}
	// Keep the favorite separator rule unchanged.
	items := []list.Item{targetItem{favorite: true}, targetItem{favorite: false}}
	if !delegate.isFavoriteBoundary(items, 0) {
		t.Fatal("favorite separator lost")
	}
}

func TestLongStatusDoesNotDisplaceControls(t *testing.T) {
	m := finderFixture()
	m.width, m.height = 80, 24
	m.status = "Refresh failed: " + strings.Repeat("long error detail ", 50) + "END_OF_ERROR"
	assertFits(t, m.mainScreen(), 80, 24)
	if !strings.Contains(ansi.Strip(m.mainScreen()), "Full status in Help") {
		t.Fatal("missing full-error disclosure")
	}
	m.activeView = helpView
	m.secondaryOffset = m.secondaryScrollLimit()
	if !strings.Contains(ansi.Strip(m.helpView()), "END_OF_ERROR") {
		t.Fatal("complete error is not accessible")
	}
}

func TestTerminalBudgetGrid(t *testing.T) {
	for _, width := range []int{1, 16, 24, 32, 40, 60, 80, 85, 86, 100, 160} {
		for _, height := range []int{1, 8, 16, 24, 40} {
			m := finderFixture()
			m.width, m.height = width, height
			assertFits(t, m.mainScreen(), width, height)
			m.fullNetworkIDs = true
			assertFits(t, m.mainScreen(), width, height)
		}
	}
}

func TestContextualActions(t *testing.T) {
	m := NewModel(nil, nil, Config{}, cache.TopologySnapshot{}, cache.VMInventorySnapshot{}, cache.Preferences{})
	controls := ansi.Strip(shortcutRows(100, m.mainControls(false)...))
	for _, unavailable := range []string{"enter: connect", "t: transfer", "m: mount", "x: favorite", "d: full IDs", "r: refresh"} {
		if strings.Contains(controls, unavailable) {
			t.Fatalf("unavailable action advertised: %s", unavailable)
		}
	}
	m = finderFixture()
	m.config.Authentication.Type = "ssh-key"
	controls = ansi.Strip(shortcutRows(100, m.mainControls(false)...))
	if strings.Contains(controls, "t: transfer") || strings.Contains(controls, "m: mount") {
		t.Fatal("AAD-only actions advertised for ssh-key authentication")
	}
}
