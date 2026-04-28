package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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
