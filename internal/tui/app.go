package tui

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Row struct {
	Repo         string
	EnvFile      string
	DriftStatus  string
	BackupStatus string
	ImportedAt   string
	BackupAt     string
	CurrentAt    string
	Missing      bool
}

type Actions struct {
	Refresh func() ([]Row, error)
	Import  func(Row) (string, error)
	Backup  func(Row) (string, error)
	Restore func(Row) (string, error)
	Unlock  func(string) (string, error)
}

type Model struct {
	rows          []Row
	filtered      []int
	selected      int
	width         int
	height        int
	filter        string
	typing        bool
	unlocking     bool
	unlockInput   string
	pendingAction string
	statusMessage string
	actions       Actions
}

var (
	appStyle       = lipgloss.NewStyle().Padding(1, 2)
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("24")).Padding(0, 1)
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
	selectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	cleanStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	missingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	detailBoxStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
)

type actionResultMsg struct {
	rows    []Row
	message string
	err     error
}

type unlockResultMsg struct {
	message string
	err     error
}

func NewModel(rows []Row) Model {
	return NewModelWithActions(rows, Actions{})
}

func NewModelWithActions(rows []Row, actions Actions) Model {
	model := Model{
		rows:    rows,
		actions: actions,
	}
	model.applyFilter()
	return model
}

func Run(output io.Writer, rows []Row, actions Actions) error {
	_, err := tea.NewProgram(NewModelWithActions(rows, actions), tea.WithOutput(output)).Run()
	return err
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case actionResultMsg:
		if msg.err != nil {
			m.statusMessage = "error: " + msg.err.Error()
			return m, nil
		}
		m.rows = msg.rows
		m.applyFilter()
		m.statusMessage = msg.message
	case unlockResultMsg:
		if msg.err != nil {
			m.statusMessage = "error: " + msg.err.Error()
			return m, nil
		}
		m.statusMessage = msg.message
	case tea.KeyMsg:
		if m.unlocking {
			return m.updateUnlock(msg)
		}
		if m.typing {
			return m.updateFilter(msg)
		}
		if m.pendingAction != "" {
			return m.updateConfirmation(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "g", "home":
			m.selected = 0
		case "G", "end":
			if len(m.filtered) > 0 {
				m.selected = len(m.filtered) - 1
			}
		case "/":
			m.typing = true
		case "u":
			m.startUnlock()
		case "i":
			m.startAction("import")
		case "b":
			m.startAction("backup")
		case "r":
			m.startAction("restore")
		}
	}
	return m, nil
}

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = 96
	}

	var body strings.Builder
	body.WriteString(titleStyle.Render("dot-vault"))
	body.WriteString(" ")
	body.WriteString(mutedStyle.Render(fmt.Sprintf("%d env files", len(m.rows))))
	if m.filter != "" {
		body.WriteString(" ")
		body.WriteString(mutedStyle.Render("filter: " + m.filter))
	}
	if m.statusMessage != "" {
		body.WriteString("\n")
		body.WriteString(mutedStyle.Render(m.statusMessage))
	}
	if m.pendingAction != "" {
		body.WriteString("\n")
		body.WriteString(warnStyle.Render(fmt.Sprintf("Confirm %s on selected env file? y/n", m.pendingAction)))
	}
	if m.unlocking {
		body.WriteString("\n")
		body.WriteString(warnStyle.Render("Master passphrase: " + strings.Repeat("*", len([]rune(m.unlockInput)))))
	}
	body.WriteString("\n\n")

	body.WriteString(m.renderHeader(width))
	body.WriteString("\n")
	body.WriteString(m.renderRows(width))
	body.WriteString("\n")
	body.WriteString(m.renderDetail(width))
	body.WriteString("\n")
	body.WriteString(m.renderHelp())

	return appStyle.Width(width).Render(body.String())
}

func (m Model) updateUnlock(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		passphrase := m.unlockInput
		m.unlockInput = ""
		m.unlocking = false
		if strings.TrimSpace(passphrase) == "" {
			m.statusMessage = "unlock cancelled"
			return m, nil
		}
		if m.actions.Unlock == nil {
			m.statusMessage = "unlock action is unavailable"
			return m, nil
		}
		return m, runUnlock(m.actions.Unlock, passphrase)
	case "esc":
		m.unlockInput = ""
		m.unlocking = false
		m.statusMessage = "unlock cancelled"
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if len(m.unlockInput) > 0 {
			runes := []rune(m.unlockInput)
			m.unlockInput = string(runes[:len(runes)-1])
		}
	default:
		if len(msg.Runes) > 0 {
			m.unlockInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateConfirmation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		action := m.pendingAction
		m.pendingAction = ""
		row, ok := m.selectedRow()
		if !ok {
			m.statusMessage = "no selected env file"
			return m, nil
		}
		return m, runAction(m.actions, action, row)
	case "n", "N", "esc":
		m.statusMessage = m.pendingAction + " cancelled"
		m.pendingAction = ""
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	default:
		return m, nil
	}
}

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.typing = false
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
		}
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
			m.applyFilter()
		}
	}
	return m, nil
}

func (m *Model) startAction(action string) {
	if _, ok := m.selectedRow(); !ok {
		m.statusMessage = "no selected env file"
		return
	}
	if !m.hasAction(action) {
		m.statusMessage = action + " action is unavailable"
		return
	}
	m.pendingAction = action
	m.statusMessage = ""
}

func (m *Model) startUnlock() {
	if m.actions.Unlock == nil {
		m.statusMessage = "unlock action is unavailable"
		return
	}
	m.unlocking = true
	m.unlockInput = ""
	m.statusMessage = ""
}

func (m Model) hasAction(action string) bool {
	switch action {
	case "import":
		return m.actions.Import != nil && m.actions.Refresh != nil
	case "backup":
		return m.actions.Backup != nil && m.actions.Refresh != nil
	case "restore":
		return m.actions.Restore != nil && m.actions.Refresh != nil
	default:
		return false
	}
}

func (m Model) selectedRow() (Row, bool) {
	if len(m.filtered) == 0 || m.selected >= len(m.filtered) {
		return Row{}, false
	}
	return m.rows[m.filtered[m.selected]], true
}

func (m *Model) move(delta int) {
	if len(m.filtered) == 0 {
		m.selected = 0
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
}

func (m *Model) applyFilter() {
	m.filtered = m.filtered[:0]
	filter := strings.ToLower(strings.TrimSpace(m.filter))
	for index, row := range m.rows {
		if filter == "" || strings.Contains(strings.ToLower(row.Repo+" "+row.EnvFile+" "+row.DriftStatus+" "+row.BackupStatus), filter) {
			m.filtered = append(m.filtered, index)
		}
	}
	if len(m.filtered) == 0 || m.selected >= len(m.filtered) {
		m.selected = 0
	}
}

func (m Model) renderHeader(width int) string {
	return headerStyle.Render(fixedColumns(width, "REPOSITORY", "ENV", "DRIFT", "BACKUP", "IMPORTED"))
}

func (m Model) renderRows(width int) string {
	if len(m.filtered) == 0 {
		return mutedStyle.Render("No env files match the current filter.")
	}

	limit := 12
	if m.height > 0 {
		limit = max(6, m.height-12)
	}
	start := 0
	if m.selected >= limit {
		start = m.selected - limit + 1
	}
	end := min(len(m.filtered), start+limit)

	lines := make([]string, 0, end-start)
	for visibleIndex := start; visibleIndex < end; visibleIndex++ {
		row := m.rows[m.filtered[visibleIndex]]
		line := fixedColumns(width, row.Repo, row.EnvFile, styleStatus(row.DriftStatus), styleStatus(row.BackupStatus), row.ImportedAt)
		if visibleIndex == m.selected {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderDetail(width int) string {
	if len(m.filtered) == 0 {
		return detailBoxStyle.Width(width - 6).Render("No selected env file.")
	}
	row := m.rows[m.filtered[m.selected]]
	detail := fmt.Sprintf(
		"%s/%s\ncurrent: %s\nimported: %s\nbackup: %s",
		row.Repo,
		row.EnvFile,
		row.CurrentAt,
		emptyAsNever(row.ImportedAt),
		emptyAsNever(row.BackupAt),
	)
	return detailBoxStyle.Width(width - 6).Render(detail)
}

func (m Model) renderHelp() string {
	if m.pendingAction != "" {
		return helpStyle.Render("y confirm  n cancel  esc cancel")
	}
	if m.unlocking {
		return helpStyle.Render("enter unlock  esc cancel  backspace edit")
	}
	if m.typing {
		return helpStyle.Render("/ filtering  enter accept  esc cancel  backspace edit")
	}
	return helpStyle.Render("j/k move  g/G jump  / filter  u unlock  i import  b backup  r restore  q quit")
}

func runAction(actions Actions, action string, row Row) tea.Cmd {
	return func() tea.Msg {
		var message string
		var err error
		switch action {
		case "import":
			message, err = actions.Import(row)
		case "backup":
			message, err = actions.Backup(row)
		case "restore":
			message, err = actions.Restore(row)
		default:
			err = fmt.Errorf("unknown action %q", action)
		}
		if err != nil {
			return actionResultMsg{err: err}
		}

		rows, err := actions.Refresh()
		if err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{
			rows:    rows,
			message: message,
		}
	}
}

func runUnlock(unlock func(string) (string, error), passphrase string) tea.Cmd {
	return func() tea.Msg {
		message, err := unlock(passphrase)
		return unlockResultMsg{
			message: message,
			err:     err,
		}
	}
}

func fixedColumns(width int, repo string, env string, drift string, backup string, imported string) string {
	repoWidth := max(18, min(36, width/3))
	envWidth := 18
	statusWidth := 12
	importedWidth := 25

	return fmt.Sprintf(
		"%-*s  %-*s  %-*s  %-*s  %-*s",
		repoWidth,
		truncate(repo, repoWidth),
		envWidth,
		truncate(env, envWidth),
		statusWidth,
		truncate(drift, statusWidth),
		statusWidth,
		truncate(backup, statusWidth),
		importedWidth,
		truncate(emptyAsNever(imported), importedWidth),
	)
}

func styleStatus(status string) string {
	switch status {
	case "clean", "backed_up":
		return cleanStyle.Render(status)
	case "drift", "backup_due":
		return warnStyle.Render(status)
	case "missing", "none":
		return missingStyle.Render(status)
	default:
		return status
	}
}

func emptyAsNever(value string) string {
	if value == "" {
		return "never"
	}
	return value
}

func truncate(value string, width int) string {
	if width <= 0 || len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "..."
}
