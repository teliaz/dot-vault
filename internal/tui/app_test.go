package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	lipgloss.SetColorProfile(termenv.ANSI256)
}

func TestModelFilter(t *testing.T) {
	t.Parallel()

	model := NewModel([]Row{
		{Repo: "api", EnvFile: ".env", DriftStatus: "clean", BackupStatus: "backed_up"},
		{Repo: "web", EnvFile: ".env.local", DriftStatus: "drift", BackupStatus: "backup_due"},
	})

	if len(model.filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(model.filtered))
	}

	model.filter = "drift"
	model.applyFilter()
	if len(model.filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(model.filtered))
	}
	if model.rows[model.filtered[0]].Repo != "web" {
		t.Fatalf("filtered row repo = %q, want web", model.rows[model.filtered[0]].Repo)
	}
}

func TestModelShowsRepoOnlyRowsByDefaultAndTogglesEnvOnly(t *testing.T) {
	t.Parallel()

	model := NewModel([]Row{
		{Repo: "api", EnvFile: ".env", DriftStatus: "clean", BackupStatus: "backed_up"},
		{Repo: "docs", DriftStatus: "no_env", BackupStatus: "none", RepositoryOnly: true},
	})

	if len(model.filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(model.filtered))
	}
	view := model.View()
	if !strings.Contains(view, "docs") || !strings.Contains(view, "none") || !strings.Contains(view, "no_env") {
		t.Fatalf("View() missing repo-only row: %q", view)
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(Model)
	if model.showAllRepos {
		t.Fatalf("showAllRepos = true, want false")
	}
	if len(model.filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(model.filtered))
	}
	if model.rows[model.filtered[0]].Repo != "api" {
		t.Fatalf("filtered repo = %q, want api", model.rows[model.filtered[0]].Repo)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(Model)
	if !model.showAllRepos {
		t.Fatalf("showAllRepos = false, want true")
	}
	if len(model.filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2 after toggling back", len(model.filtered))
	}
}

func TestModelViewContainsStatusRowsAndHelp(t *testing.T) {
	t.Parallel()

	model := NewModel([]Row{
		{
			Repo:         "api",
			EnvFile:      ".env",
			DriftStatus:  "missing",
			BackupStatus: "none",
			CurrentAt:    "2026-04-28T10:00:00Z",
		},
	})
	model.width = 100
	model.height = 24

	view := model.View()
	for _, want := range []string{"dot-vault", "api", ".env", "missing", "j/k move"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in %q", want, view)
		}
	}
}

func TestModelViewShowsDependencyWarnings(t *testing.T) {
	t.Parallel()

	model := NewDashboardModelWithDependencies(
		[]Org{{Name: "acme", Active: true}},
		[]Dependency{{Name: "git", Required: true, Available: false, Detail: "required for clone"}},
		[]Row{{Organization: "acme", Repo: "api", EnvFile: ".env"}},
		Actions{},
	)
	model.width = 100
	model.height = 24

	view := model.View()
	for _, want := range []string{"DEPENDENCIES", "git", "missing", "dependency warning"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in %q", want, view)
		}
	}
}

func TestFixedColumnsUsesVisibleWidthsForStyledCells(t *testing.T) {
	t.Parallel()

	line := fixedColumns(
		72,
		"api",
		".env",
		"missing",
		"yes",
		"backup_due",
		"never",
		"changed",
	)

	if got := lipgloss.Width(line); got != 72 {
		t.Fatalf("fixedColumns visible width = %d, want 72; line = %q", got, line)
	}
}

func TestSelectedRowStylesEveryCell(t *testing.T) {
	t.Parallel()

	line := fixedColumnsSelected(
		96,
		"api",
		".env",
		"clean",
		"yes",
		"backed_up",
		"never",
		"locked",
	)

	if got := lipgloss.Width(line); got != 96 {
		t.Fatalf("selected row width = %d, want 96; line = %q", got, line)
	}
	if count := strings.Count(line, "48;5;24"); count < 7 {
		t.Fatalf("selected row background applied %d times, want at least 7; line = %q", count, line)
	}
}

func TestDependencyHeaderIsNotHighlightedWithOrgFocus(t *testing.T) {
	t.Parallel()

	model := NewDashboardModelWithDependencies(
		[]Org{{Name: "acme", Active: true}},
		[]Dependency{{Name: "git", Available: true}},
		nil,
		Actions{},
	)
	model.focus = "orgs"

	lines := strings.Split(model.renderSidebar(24), "\n")
	if len(lines) < 4 {
		t.Fatalf("sidebar rendered too few lines: %q", strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[0], "48;5;24") {
		t.Fatalf("organization header was not highlighted: %q", lines[0])
	}
	if strings.Contains(lines[3], "48;5;24") {
		t.Fatalf("dependency header should not be highlighted: %q", lines[3])
	}
}

func TestTableLayoutGrowsDriftAndGitWithExtraWidth(t *testing.T) {
	t.Parallel()

	base := tableLayout(109)
	wide := tableLayout(129)

	if wide.repo != base.repo || wide.env != base.env || wide.backup != base.backup || wide.backupAt != base.backupAt || wide.compare != base.compare {
		t.Fatalf("non status columns changed with extra width: base=%#v wide=%#v", base, wide)
	}
	if wide.drift <= base.drift {
		t.Fatalf("wide drift width = %d, want greater than %d", wide.drift, base.drift)
	}
	if wide.git <= base.git {
		t.Fatalf("wide git width = %d, want greater than %d", wide.git, base.git)
	}
}

func TestPanelsFillAvailableWidth(t *testing.T) {
	t.Parallel()

	model := NewDashboardModel(
		[]Org{{Name: "acme", Active: true}},
		[]Row{{Organization: "acme", Repo: "api", EnvFile: ".env", DriftStatus: "clean", BackupStatus: "backed_up"}},
		Actions{},
	)

	for _, width := range []int{76, 96, 120} {
		assertRenderedBlockWidth(t, "renderPanels", model.renderPanels(width), width)
		assertRenderedBlockWidth(t, "renderDetail", model.renderDetail(width), width)
	}
}

func TestModelViewFitsWindowWidth(t *testing.T) {
	t.Parallel()

	model := NewDashboardModel(
		[]Org{{Name: "acme", Active: true}},
		[]Row{{Organization: "acme", Repo: "api", EnvFile: ".env", DriftStatus: "clean", BackupStatus: "backed_up"}},
		Actions{},
	)
	model.width = 100
	model.height = 24

	if got := lipgloss.Width(model.View()); got != model.width {
		t.Fatalf("View() visible width = %d, want %d", got, model.width)
	}
	for index, line := range strings.Split(model.View(), "\n") {
		if got := lipgloss.Width(line); got > model.width {
			t.Fatalf("View() line %d width = %d, want <= %d; line = %q", index, got, model.width, line)
		}
	}
}

func assertRenderedBlockWidth(t *testing.T, name string, block string, width int) {
	t.Helper()

	if got := lipgloss.Width(block); got != width {
		t.Fatalf("%s(%d) visible width = %d", name, width, got)
	}
	for index, line := range strings.Split(block, "\n") {
		if got := lipgloss.Width(line); got != width {
			t.Fatalf("%s(%d) line %d width = %d; line = %q", name, width, index, got, line)
		}
	}
}

func TestModelConfirmedActionRunsAndRefreshesRows(t *testing.T) {
	t.Parallel()

	importCalled := false
	model := NewModelWithActions([]Row{
		{Repo: "api", EnvFile: ".env", DriftStatus: "drift", BackupStatus: "backup_due"},
	}, Actions{
		Refresh: func() ([]Row, error) {
			return []Row{{Repo: "api", EnvFile: ".env", DriftStatus: "clean", BackupStatus: "backup_due"}}, nil
		},
		Import: func(row Row) (string, error) {
			importCalled = true
			if row.Repo != "api" || row.EnvFile != ".env" {
				t.Fatalf("row = %#v, want api/.env", row)
			}
			return "imported api/.env", nil
		},
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("import key returned cmd before confirmation")
	}
	if model.pendingAction != "import" {
		t.Fatalf("pendingAction = %q, want import", model.pendingAction)
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("confirm did not return action command")
	}

	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)

	if !importCalled {
		t.Fatalf("import action was not called")
	}
	if model.rows[0].DriftStatus != "clean" {
		t.Fatalf("DriftStatus = %q, want clean", model.rows[0].DriftStatus)
	}
	if model.statusMessage != "imported api/.env" {
		t.Fatalf("statusMessage = %q, want imported api/.env", model.statusMessage)
	}
}

func TestModelCancelledActionDoesNotRun(t *testing.T) {
	t.Parallel()

	backupCalled := false
	model := NewModelWithActions([]Row{
		{Repo: "api", EnvFile: ".env", DriftStatus: "clean", BackupStatus: "backup_due"},
	}, Actions{
		Refresh: func() ([]Row, error) {
			return nil, nil
		},
		Backup: func(row Row) (string, error) {
			backupCalled = true
			return "backed up", nil
		},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	model = updated.(Model)

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("cancel returned a command")
	}
	if backupCalled {
		t.Fatalf("backup action was called after cancel")
	}
	if model.statusMessage != "backup cancelled" {
		t.Fatalf("statusMessage = %q, want backup cancelled", model.statusMessage)
	}
}

func TestModelUnlockMasksInputAndRunsAction(t *testing.T) {
	t.Parallel()

	unlockCalled := false
	model := NewModelWithActions([]Row{
		{Repo: "api", EnvFile: ".env", DriftStatus: "clean", BackupStatus: "backup_due"},
	}, Actions{
		Unlock: func(passphrase string) (string, error) {
			unlockCalled = true
			if passphrase != "correct horse battery staple" {
				t.Fatalf("passphrase = %q, want test passphrase", passphrase)
			}
			return "unlocked acme", nil
		},
	})

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("unlock key returned command before passphrase")
	}
	if !model.unlocking {
		t.Fatalf("unlocking = false, want true")
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("correct horse battery staple")})
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("typing passphrase returned command")
	}

	view := model.View()
	if strings.Contains(view, "correct horse battery staple") {
		t.Fatalf("View() leaked passphrase: %q", view)
	}

	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("enter did not return unlock command")
	}

	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if !unlockCalled {
		t.Fatalf("unlock action was not called")
	}
	if model.statusMessage != "unlocked acme" {
		t.Fatalf("statusMessage = %q, want unlocked acme", model.statusMessage)
	}
}

func TestModelUnlockCanBeCancelled(t *testing.T) {
	t.Parallel()

	unlockCalled := false
	model := NewModelWithActions([]Row{{Repo: "api", EnvFile: ".env"}}, Actions{
		Unlock: func(passphrase string) (string, error) {
			unlockCalled = true
			return "unlocked", nil
		},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(Model)
	if cmd != nil {
		t.Fatalf("cancel returned command")
	}
	if unlockCalled {
		t.Fatalf("unlock action was called after cancel")
	}
	if model.statusMessage != "unlock cancelled" {
		t.Fatalf("statusMessage = %q, want unlock cancelled", model.statusMessage)
	}
}

func TestModelSelectsOrganizationFromFocusedPanel(t *testing.T) {
	t.Parallel()

	selected := ""
	model := NewDashboardModel([]Org{
		{Name: "acme", Active: true},
		{Name: "other"},
	}, []Row{{Organization: "acme", Repo: "api", EnvFile: ".env"}}, Actions{
		SelectOrg: func(org string) ([]Org, []Row, string, error) {
			selected = org
			return []Org{{Name: "acme"}, {Name: "other", Active: true}},
				[]Row{{Organization: "other", Repo: "web", EnvFile: ".env"}},
				"selected organization other",
				nil
		},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("enter on organization panel did not return select command")
	}

	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if selected != "other" {
		t.Fatalf("selected = %q, want other", selected)
	}
	if model.selectedOrgName() != "other" {
		t.Fatalf("selectedOrgName() = %q, want other", model.selectedOrgName())
	}
	if model.rows[0].Organization != "other" {
		t.Fatalf("row organization = %q, want other", model.rows[0].Organization)
	}
}

func TestModelShowsOrganizationHelpAndRunsOrgActions(t *testing.T) {
	t.Parallel()

	removed := ""
	reset := ""
	model := NewDashboardModel([]Org{
		{Name: "acme", Active: true},
	}, []Row{{Organization: "acme", Repo: "api", EnvFile: ".env"}}, Actions{
		RemoveOrg: func(org string) ([]Org, []Row, string, error) {
			removed = org
			return nil, nil, "removed organization acme", nil
		},
		ResetOrg: func(org string) ([]Org, []Row, string, error) {
			reset = org
			return []Org{{Name: org, Active: true}}, []Row{{Organization: org, Repo: "api", EnvFile: ".env"}}, "reset backups", nil
		},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if !strings.Contains(model.renderHelp(), "a add org") || !strings.Contains(model.renderHelp(), "R reset backups") {
		t.Fatalf("org help missing organization shortcuts: %q", model.renderHelp())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	if model.pendingAction != "remove organization" {
		t.Fatalf("pendingAction = %q, want remove organization", model.pendingAction)
	}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("confirm remove did not return command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if removed != "acme" {
		t.Fatalf("removed = %q, want acme", removed)
	}

	model = NewDashboardModel([]Org{{Name: "acme", Active: true}}, []Row{{Organization: "acme", Repo: "api", EnvFile: ".env"}}, model.actions)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("confirm reset did not return command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if reset != "acme" {
		t.Fatalf("reset = %q, want acme", reset)
	}
	if model.statusMessage != "reset backups" {
		t.Fatalf("statusMessage = %q, want reset backups", model.statusMessage)
	}
}

func TestModelAddOrganizationUsesSetupFlow(t *testing.T) {
	t.Parallel()

	added := SetupInput{}
	model := NewDashboardModel([]Org{{Name: "acme", Active: true}}, []Row{{Organization: "acme", Repo: "api", EnvFile: ".env"}}, Actions{
		AddOrg: func(input SetupInput) ([]Org, []Row, string, error) {
			added = input
			return []Org{{Name: "acme"}, {Name: input.Name, Active: true}},
				[]Row{{Organization: input.Name, Repo: "web", EnvFile: ".env"}},
				"created organization other",
				nil
		},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = updated.(Model)
	if !model.addingOrg {
		t.Fatalf("addingOrg = false, want true")
	}

	for _, value := range []string{"other", "/repos/other", "/secrets/other", "correct horse battery staple"} {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)})
		model = updated.(Model)
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model = updated.(Model)
	}
	cmd := func() tea.Msg { return nil }
	if !model.addingOrg {
		t.Fatalf("addingOrg ended before create command")
	}
	updated, actualCmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if actualCmd != nil {
		cmd = actualCmd
	}
	if actualCmd == nil {
		t.Fatalf("final enter did not return add organization command")
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if added.Name != "other" {
		t.Fatalf("added.Name = %q, want other", added.Name)
	}
	if model.selectedOrgName() != "other" {
		t.Fatalf("selectedOrgName() = %q, want other", model.selectedOrgName())
	}
}

func TestModelClonePromptsForURLAndRefreshes(t *testing.T) {
	t.Parallel()

	cloneCalled := false
	model := NewModelWithActions([]Row{
		{Organization: "acme", Repo: "missing", EnvFile: ".env", GitPresent: false},
	}, Actions{
		RefreshOrg: func(org string) ([]Row, error) {
			return []Row{{Organization: org, Repo: "missing", EnvFile: ".env", GitPresent: true}}, nil
		},
		Clone: func(row Row, cloneURL string) (string, error) {
			cloneCalled = true
			if row.Repo != "missing" {
				t.Fatalf("row.Repo = %q, want missing", row.Repo)
			}
			if cloneURL != "git@example.com:acme/missing.git" {
				t.Fatalf("cloneURL = %q, want git@example.com:acme/missing.git", cloneURL)
			}
			return "cloned missing", nil
		},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if !model.cloning {
		t.Fatalf("cloning = false, want true")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("git@example.com:acme/missing.git")})
	model = updated.(Model)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if cmd == nil {
		t.Fatalf("enter did not return clone command")
	}

	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if !cloneCalled {
		t.Fatalf("clone action was not called")
	}
	if !model.rows[0].GitPresent {
		t.Fatalf("GitPresent = false, want true after refresh")
	}
}

func TestModelClonePrefillsStoredRemoteURL(t *testing.T) {
	t.Parallel()

	model := NewModelWithActions([]Row{
		{
			Organization: "acme",
			Repo:         "missing",
			EnvFile:      ".env",
			GitPresent:   false,
			RemoteURL:    "git@example.com:acme/missing.git",
		},
	}, Actions{
		RefreshOrg: func(org string) ([]Row, error) {
			return nil, nil
		},
		Clone: func(row Row, cloneURL string) (string, error) {
			return "cloned", nil
		},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = updated.(Model)
	if model.cloneInput != "git@example.com:acme/missing.git" {
		t.Fatalf("cloneInput = %q, want stored remote URL", model.cloneInput)
	}
}

func TestSetupModelCreatesOrganization(t *testing.T) {
	t.Parallel()

	createCalled := false
	model := NewSetupModel(SetupActions{
		Create: func(input SetupInput) (string, error) {
			createCalled = true
			if input.Name != "acme" {
				t.Fatalf("Name = %q, want acme", input.Name)
			}
			if input.RepoRoot != "/repos/acme" {
				t.Fatalf("RepoRoot = %q, want /repos/acme", input.RepoRoot)
			}
			if input.StoreRoot != "/secrets/acme" {
				t.Fatalf("StoreRoot = %q, want /secrets/acme", input.StoreRoot)
			}
			if input.MasterPassphrase != "correct horse battery staple" {
				t.Fatalf("MasterPassphrase = %q, want test passphrase", input.MasterPassphrase)
			}
			return "created organization acme", nil
		},
	})

	model = updateSetupRunes(t, model, "acme")
	model = updateSetupKey(t, model, "enter")
	model = updateSetupRunes(t, model, "/repos/acme")
	model = updateSetupKey(t, model, "enter")
	model = updateSetupRunes(t, model, "/secrets/acme")
	model = updateSetupKey(t, model, "enter")
	model = updateSetupRunes(t, model, "correct horse battery staple")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(SetupModel)
	if cmd == nil {
		t.Fatalf("submit did not return create command")
	}

	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(SetupModel)

	if !createCalled {
		t.Fatalf("create action was not called")
	}
	if !model.result.Created {
		t.Fatalf("result.Created = false, want true")
	}
	if model.result.Message != "created organization acme" {
		t.Fatalf("Message = %q, want created organization acme", model.result.Message)
	}
}

func TestSetupModelValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	model := NewSetupModel(SetupActions{})
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(SetupModel)
	if cmd != nil {
		t.Fatalf("empty first field returned command")
	}
	if !strings.Contains(model.status, "organization is required") {
		t.Fatalf("status = %q, want organization required error", model.status)
	}
}

func TestSetupModelViewContainsFieldsAndHelp(t *testing.T) {
	t.Parallel()

	model := NewSetupModel(SetupActions{})
	model.width = 100
	view := model.View()

	for _, want := range []string{"dot-vault setup", "Organization", "Repository root", "Encrypted store root", "enter next/create"} {
		if !strings.Contains(view, want) {
			t.Fatalf("View() missing %q in %q", want, view)
		}
	}
}

func TestSetupModelMasksMasterPassphrase(t *testing.T) {
	t.Parallel()

	model := NewSetupModel(SetupActions{})
	model.focused = 3
	model.fields[3].value = "correct horse battery staple"

	view := model.View()
	if strings.Contains(view, "correct horse battery staple") {
		t.Fatalf("View() leaked master passphrase: %q", view)
	}
	if !strings.Contains(view, strings.Repeat("*", len([]rune("correct horse battery staple")))) {
		t.Fatalf("View() missing masked passphrase in %q", view)
	}
}

func updateSetupRunes(t *testing.T, model SetupModel, value string) SetupModel {
	t.Helper()
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)})
	if cmd != nil {
		t.Fatalf("typing %q returned command", value)
	}
	return updated.(SetupModel)
}

func updateSetupKey(t *testing.T, model SetupModel, key string) SetupModel {
	t.Helper()
	var updated tea.Model
	var cmd tea.Cmd
	switch key {
	case "enter":
		updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	default:
		updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	}
	if cmd != nil {
		t.Fatalf("key %q returned command", key)
	}
	return updated.(SetupModel)
}
