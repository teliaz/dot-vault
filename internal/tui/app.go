package tui

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Org struct {
	Name      string
	Active    bool
	RepoRoot  string
	StoreRoot string
}

type Dependency struct {
	Name      string
	Required  bool
	Available bool
	Detail    string
}

type Row struct {
	Organization     string
	Repo             string
	EnvFile          string
	DriftStatus      string
	BackupStatus     string
	ImportedAt       string
	BackupAt         string
	CurrentAt        string
	RemoteURL        string
	GitPresent       bool
	EnvPresent       bool
	EnvSuggestedFrom string
	StoreMissing     bool
	RepositoryOnly   bool
	DiffSummary      string
}

type Actions struct {
	Refresh    func() ([]Row, error)
	RefreshOrg func(string) ([]Row, error)
	SelectOrg  func(string) ([]Org, []Row, string, error)
	AddOrg     func(SetupInput) ([]Org, []Row, string, error)
	RemoveOrg  func(string) ([]Org, []Row, string, error)
	ResetOrg   func(string) ([]Org, []Row, string, error)
	Import     func(Row) (string, error)
	Backup     func(Row) (string, error)
	Restore    func(Row) (string, error)
	Clone      func(Row, string) (string, error)
	Diff       func(Row) (string, error)
	Unlock     func(string) (string, error)
	UnlockOrg  func(string, string) ([]Row, string, error)
}

type Model struct {
	orgs          []Org
	dependencies  []Dependency
	rows          []Row
	filtered      []int
	selected      int
	selectedOrg   int
	focus         string
	width         int
	height        int
	filter        string
	showAllRepos  bool
	typing        bool
	unlocking     bool
	unlockInput   string
	cloning       bool
	cloneInput    string
	addingOrg     bool
	setupFields   []setupField
	setupFocused  int
	diffView      string
	pendingAction string
	statusMessage string
	compareLoaded bool
	actions       Actions
}

var (
	appStyle       = lipgloss.NewStyle()
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("24")).Padding(0, 1)
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("244"))
	panelStyle     = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	focusStyle     = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("39")).Padding(0, 1)
	selectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	cleanStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	missingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	helpKeyStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230"))
	detailBoxStyle = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
)

type actionResultMsg struct {
	orgs        []Org
	rows        []Row
	selectedOrg string
	message     string
	err         error
}

type setupDashboardResultMsg struct {
	orgs        []Org
	rows        []Row
	selectedOrg string
	message     string
	err         error
}

type unlockResultMsg struct {
	rows    []Row
	message string
	err     error
}

type diffResultMsg struct {
	content string
	err     error
}

func NewModel(rows []Row) Model {
	return NewModelWithActions(rows, Actions{})
}

func NewModelWithActions(rows []Row, actions Actions) Model {
	return NewDashboardModel(nil, rows, actions)
}

func NewDashboardModel(orgs []Org, rows []Row, actions Actions) Model {
	return NewDashboardModelWithDependencies(orgs, nil, rows, actions)
}

func NewDashboardModelWithDependencies(orgs []Org, dependencies []Dependency, rows []Row, actions Actions) Model {
	model := Model{
		orgs:         normalizeOrgs(orgs, rows),
		dependencies: dependencies,
		rows:         rows,
		focus:        "files",
		showAllRepos: true,
		actions:      actions,
	}
	model.selectedOrg = selectedOrgIndex(model.orgs)
	model.applyFilter()
	return model
}

func Run(output io.Writer, rows []Row, actions Actions) error {
	return RunDashboard(output, nil, rows, actions)
}

func RunDashboard(output io.Writer, orgs []Org, rows []Row, actions Actions) error {
	return RunDashboardWithDependencies(output, orgs, nil, rows, actions)
}

func RunDashboardWithDependencies(output io.Writer, orgs []Org, dependencies []Dependency, rows []Row, actions Actions) error {
	_, err := tea.NewProgram(NewDashboardModelWithDependencies(orgs, dependencies, rows, actions), tea.WithOutput(output)).Run()
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
		if msg.orgs != nil {
			m.orgs = normalizeOrgs(msg.orgs, msg.rows)
			m.selectedOrg = indexOrg(m.orgs, msg.selectedOrg)
		}
		m.rows = msg.rows
		m.compareLoaded = false
		m.applyFilter()
		m.statusMessage = msg.message
	case setupDashboardResultMsg:
		m.addingOrg = false
		if msg.err != nil {
			m.statusMessage = "error: " + msg.err.Error()
			return m, nil
		}
		m.orgs = normalizeOrgs(msg.orgs, msg.rows)
		m.selectedOrg = indexOrg(m.orgs, msg.selectedOrg)
		m.rows = msg.rows
		m.compareLoaded = false
		m.applyFilter()
		m.statusMessage = msg.message
	case unlockResultMsg:
		if msg.err != nil {
			m.statusMessage = "error: " + msg.err.Error()
			return m, nil
		}
		if msg.rows != nil {
			m.rows = msg.rows
			m.compareLoaded = true
			m.applyFilter()
		}
		m.statusMessage = msg.message
	case diffResultMsg:
		if msg.err != nil {
			m.statusMessage = "error: " + msg.err.Error()
			return m, nil
		}
		m.diffView = msg.content
	case tea.KeyMsg:
		if m.diffView != "" {
			return m.updateDiffView(msg)
		}
		if m.unlocking {
			return m.updateUnlock(msg)
		}
		if m.cloning {
			return m.updateClone(msg)
		}
		if m.addingOrg {
			return m.updateAddOrg(msg)
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
		case "tab":
			m.toggleFocus()
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "pgdown":
			m.pageMove(1)
		case "pgup":
			m.pageMove(-1)
		case "g", "home":
			m.jumpStart()
		case "G", "end":
			m.jumpEnd()
		case "enter":
			return m.selectFocusedOrg()
		case "/":
			if m.focus == "files" {
				m.typing = true
			}
		case "e":
			if m.focus == "files" {
				m.toggleRepositoryOnlyRows()
			}
		case "u":
			if m.focus == "files" {
				m.startUnlock()
			}
		case "a":
			m.startAddOrg()
		case "x":
			m.startOrgAction("remove organization")
		case "R":
			m.startOrgAction("reset organization backups")
		case "i":
			m.startAction("import")
		case "b":
			m.startAction("backup")
		case "r":
			m.startAction("restore")
		case "c":
			if m.focus == "files" {
				m.startClone()
			}
		case "d":
			if m.focus == "files" {
				return m.startDiff()
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = 120
	}
	contentWidth := max(1, width-appStyle.GetHorizontalBorderSize())

	if m.diffView != "" {
		return appStyle.Width(contentWidth).Render(m.renderDiff(contentWidth))
	}

	var body strings.Builder
	body.WriteString(titleStyle.Render("dot-vault"))
	body.WriteString(" ")
	body.WriteString(mutedStyle.Render(fmt.Sprintf("%d orgs  %d repos  %d env files", len(m.orgs), m.repoCount(), m.envFileCount())))
	if missing := m.missingRequiredDependencies(); missing > 0 {
		body.WriteString(" ")
		body.WriteString(missingStyle.Render(fmt.Sprintf("%d dependency warning(s)", missing)))
	}
	if selected := m.selectedOrgName(); selected != "" {
		body.WriteString(" ")
		body.WriteString(mutedStyle.Render("selected: " + selected))
	}
	if m.filter != "" {
		body.WriteString(" ")
		body.WriteString(mutedStyle.Render("filter: " + m.filter))
	}
	if m.compareLoaded {
		body.WriteString(" ")
		body.WriteString(cleanStyle.Render("comparison loaded"))
	}
	if !m.showAllRepos {
		body.WriteString(" ")
		body.WriteString(mutedStyle.Render("env files only"))
	}
	if m.statusMessage != "" {
		body.WriteString("\n")
		body.WriteString(mutedStyle.Render(m.statusMessage))
	}
	if m.pendingAction != "" {
		body.WriteString("\n")
		body.WriteString(warnStyle.Render(m.confirmationPrompt()))
	}
	if m.unlocking {
		body.WriteString("\n")
		body.WriteString(warnStyle.Render("Master passphrase: " + strings.Repeat("*", len([]rune(m.unlockInput)))))
	}
	if m.cloning {
		body.WriteString("\n")
		body.WriteString(warnStyle.Render("Clone URL: " + m.cloneInput))
	}
	body.WriteString("\n\n")

	if m.addingOrg {
		body.WriteString(m.renderAddOrg(contentWidth))
		body.WriteString("\n")
	} else {
		body.WriteString(m.renderPanels(contentWidth))
		body.WriteString("\n")
		body.WriteString(m.renderDetail(contentWidth))
		body.WriteString("\n")
	}
	body.WriteString(m.renderHelp())

	return appStyle.Width(contentWidth).Render(body.String())
}

func (m Model) confirmationPrompt() string {
	switch m.pendingAction {
	case "remove organization", "reset organization backups":
		name := "selected organization"
		if len(m.orgs) > 0 && m.selectedOrg < len(m.orgs) {
			name = m.orgs[m.selectedOrg].Name
		}
		return fmt.Sprintf("Confirm %s for %s? y/n", m.pendingAction, name)
	default:
		return fmt.Sprintf("Confirm %s on selected env file? y/n", m.pendingAction)
	}
}

func (m Model) updateDiffView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "d":
		m.diffView = ""
	case "q", "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
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
		if m.actions.Unlock == nil && m.actions.UnlockOrg == nil {
			m.statusMessage = "unlock action is unavailable"
			return m, nil
		}
		return m, runUnlock(m.actions, m.selectedOrgName(), passphrase)
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

func (m Model) updateClone(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		cloneURL := strings.TrimSpace(m.cloneInput)
		m.cloneInput = ""
		m.cloning = false
		if cloneURL == "" {
			m.statusMessage = "clone cancelled"
			return m, nil
		}
		row, ok := m.selectedRow()
		if !ok {
			m.statusMessage = "no selected env file"
			return m, nil
		}
		return m, runClone(m.actions, row, cloneURL)
	case "esc":
		m.cloneInput = ""
		m.cloning = false
		m.statusMessage = "clone cancelled"
	case "ctrl+c":
		return m, tea.Quit
	case "backspace":
		if len(m.cloneInput) > 0 {
			runes := []rune(m.cloneInput)
			m.cloneInput = string(runes[:len(runes)-1])
		}
	default:
		if len(msg.Runes) > 0 {
			m.cloneInput += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateAddOrg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if strings.TrimSpace(m.setupFields[m.setupFocused].value) == "" {
			m.statusMessage = "error: " + setupFieldRequiredMessage(m.setupFocused)
			return m, nil
		}
		if m.setupFocused < len(m.setupFields)-1 {
			m.moveSetupFocus(1)
			return m, nil
		}
		input := m.setupInput()
		if err := validateSetupInput(input); err != nil {
			m.statusMessage = "error: " + err.Error()
			return m, nil
		}
		if m.actions.AddOrg == nil {
			m.statusMessage = "add organization action is unavailable"
			return m, nil
		}
		m.statusMessage = "creating organization..."
		return m, runAddOrg(m.actions.AddOrg, input)
	case "esc":
		m.addingOrg = false
		m.statusMessage = "add organization cancelled"
	case "ctrl+c":
		return m, tea.Quit
	case "tab", "down":
		m.moveSetupFocus(1)
	case "shift+tab", "up":
		m.moveSetupFocus(-1)
	case "backspace":
		m.deleteSetupRune()
	case "ctrl+u":
		m.setupFields[m.setupFocused].value = ""
	default:
		if len(msg.Runes) > 0 {
			m.setupFields[m.setupFocused].value += string(msg.Runes)
		}
	}
	return m, nil
}

func (m Model) updateConfirmation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		action := m.pendingAction
		m.pendingAction = ""
		switch action {
		case "remove organization":
			if len(m.orgs) == 0 {
				m.statusMessage = "no selected organization"
				return m, nil
			}
			return m, runOrgAction(m.actions.RemoveOrg, m.orgs[m.selectedOrg].Name)
		case "reset organization backups":
			if len(m.orgs) == 0 {
				m.statusMessage = "no selected organization"
				return m, nil
			}
			return m, runOrgAction(m.actions.ResetOrg, m.orgs[m.selectedOrg].Name)
		}
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
	if m.focus == "orgs" {
		m.statusMessage = "focus env files before running file actions"
		return
	}
	row, ok := m.selectedRow()
	if !ok {
		m.statusMessage = "no selected env file"
		return
	}
	if row.RepositoryOnly || row.EnvFile == "" {
		if row.EnvSuggestedFrom != "" {
			m.statusMessage = fmt.Sprintf("create %s from %s before file actions", row.EnvFile, row.EnvSuggestedFrom)
			return
		}
		m.statusMessage = "selected repository has no env file"
		return
	}
	if !m.hasAction(action) {
		m.statusMessage = action + " action is unavailable"
		return
	}
	m.pendingAction = action
	m.statusMessage = ""
}

func (m *Model) startOrgAction(action string) {
	if m.focus != "orgs" {
		m.statusMessage = "focus organizations before running organization actions"
		return
	}
	if len(m.orgs) == 0 {
		m.statusMessage = "no selected organization"
		return
	}
	switch action {
	case "remove organization":
		if m.actions.RemoveOrg == nil {
			m.statusMessage = "remove organization action is unavailable"
			return
		}
	case "reset organization backups":
		if m.actions.ResetOrg == nil {
			m.statusMessage = "reset organization backups action is unavailable"
			return
		}
	default:
		m.statusMessage = "unknown organization action"
		return
	}
	m.pendingAction = action
	m.statusMessage = ""
}

func (m *Model) startAddOrg() {
	if m.focus != "orgs" {
		m.statusMessage = "focus organizations before adding an organization"
		return
	}
	if m.actions.AddOrg == nil {
		m.statusMessage = "add organization action is unavailable"
		return
	}
	m.addingOrg = true
	m.setupFields = newSetupFields()
	m.setupFocused = 0
	m.statusMessage = ""
}

func (m *Model) startUnlock() {
	if m.actions.Unlock == nil && m.actions.UnlockOrg == nil {
		m.statusMessage = "unlock action is unavailable"
		return
	}
	m.unlocking = true
	m.unlockInput = ""
	m.statusMessage = ""
}

func (m *Model) toggleRepositoryOnlyRows() {
	m.showAllRepos = !m.showAllRepos
	m.applyFilter()
	if m.showAllRepos {
		m.statusMessage = "showing repositories without env files"
		return
	}
	m.statusMessage = "showing env files only"
}

func newSetupFields() []setupField {
	return []setupField{
		{label: "Organization"},
		{label: "Repository root"},
		{label: "Encrypted store root"},
		{label: "Master passphrase"},
	}
}

func (m *Model) moveSetupFocus(delta int) {
	m.setupFocused += delta
	if m.setupFocused < 0 {
		m.setupFocused = len(m.setupFields) - 1
	}
	if m.setupFocused >= len(m.setupFields) {
		m.setupFocused = 0
	}
}

func (m *Model) deleteSetupRune() {
	value := m.setupFields[m.setupFocused].value
	if value == "" {
		return
	}
	runes := []rune(value)
	m.setupFields[m.setupFocused].value = string(runes[:len(runes)-1])
}

func (m Model) setupInput() SetupInput {
	return SetupInput{
		Name:             strings.TrimSpace(m.setupFields[0].value),
		RepoRoot:         strings.TrimSpace(m.setupFields[1].value),
		StoreRoot:        strings.TrimSpace(m.setupFields[2].value),
		MasterPassphrase: m.setupFields[3].value,
	}
}

func (m *Model) startClone() {
	row, ok := m.selectedRow()
	if !ok {
		m.statusMessage = "no selected env file"
		return
	}
	if row.GitPresent {
		m.statusMessage = "selected repository already has a git checkout"
		return
	}
	if m.actions.Clone == nil {
		m.statusMessage = "clone action is unavailable"
		return
	}
	if !m.hasRefresh() {
		m.statusMessage = "refresh action is unavailable"
		return
	}
	m.cloning = true
	m.cloneInput = row.RemoteURL
	m.statusMessage = ""
}

func (m Model) startDiff() (tea.Model, tea.Cmd) {
	row, ok := m.selectedRow()
	if !ok {
		m.statusMessage = "no selected env file"
		return m, nil
	}
	if row.RepositoryOnly || row.EnvFile == "" {
		if row.EnvSuggestedFrom != "" {
			m.statusMessage = fmt.Sprintf("create %s from %s before diffing", row.EnvFile, row.EnvSuggestedFrom)
			return m, nil
		}
		m.statusMessage = "selected repository has no env file"
		return m, nil
	}
	if row.DriftStatus == "clean" {
		m.statusMessage = "selected env file matches the encrypted store"
		return m, nil
	}
	if m.actions.Diff == nil {
		m.statusMessage = "diff action is unavailable"
		return m, nil
	}
	return m, runDiff(m.actions.Diff, row)
}

func (m Model) selectFocusedOrg() (tea.Model, tea.Cmd) {
	if m.focus != "orgs" || len(m.orgs) == 0 {
		return m, nil
	}
	if m.actions.SelectOrg == nil {
		m.statusMessage = "organization selection is unavailable"
		return m, nil
	}
	return m, runSelectOrg(m.actions.SelectOrg, m.orgs[m.selectedOrg].Name)
}

func (m Model) hasAction(action string) bool {
	switch action {
	case "import":
		return m.actions.Import != nil && m.hasRefresh()
	case "backup":
		return m.actions.Backup != nil && m.hasRefresh()
	case "restore":
		return m.actions.Restore != nil && m.hasRefresh()
	default:
		return false
	}
}

func (m Model) hasRefresh() bool {
	return m.actions.Refresh != nil || m.actions.RefreshOrg != nil
}

func (m Model) selectedRow() (Row, bool) {
	if len(m.filtered) == 0 || m.selected >= len(m.filtered) {
		return Row{}, false
	}
	return m.rows[m.filtered[m.selected]], true
}

func (m Model) selectedOrgName() string {
	if len(m.orgs) == 0 || m.selectedOrg >= len(m.orgs) {
		if len(m.rows) > 0 {
			return m.rows[0].Organization
		}
		return ""
	}
	return m.orgs[m.selectedOrg].Name
}

func (m Model) repoCount() int {
	repos := map[string]struct{}{}
	for _, row := range m.rows {
		if row.Repo != "" {
			repos[row.Repo] = struct{}{}
		}
	}
	return len(repos)
}

func (m Model) envFileCount() int {
	count := 0
	for _, row := range m.rows {
		if !row.RepositoryOnly && row.EnvFile != "" {
			count++
		}
	}
	return count
}

func (m *Model) toggleFocus() {
	if m.focus == "orgs" {
		m.focus = "files"
		return
	}
	m.focus = "orgs"
}

func (m *Model) move(delta int) {
	if m.focus == "orgs" {
		if len(m.orgs) == 0 {
			m.selectedOrg = 0
			return
		}
		m.selectedOrg += delta
		if m.selectedOrg < 0 {
			m.selectedOrg = 0
		}
		if m.selectedOrg >= len(m.orgs) {
			m.selectedOrg = len(m.orgs) - 1
		}
		return
	}
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

func (m *Model) pageMove(direction int) {
	if direction == 0 {
		return
	}
	step := m.visibleRowLimit()
	if step < 1 {
		step = 1
	}
	if direction < 0 {
		step = -step
	}
	m.move(step)
}

func (m *Model) jumpStart() {
	if m.focus == "orgs" {
		m.selectedOrg = 0
		return
	}
	m.selected = 0
}

func (m *Model) jumpEnd() {
	if m.focus == "orgs" {
		if len(m.orgs) > 0 {
			m.selectedOrg = len(m.orgs) - 1
		}
		return
	}
	if len(m.filtered) > 0 {
		m.selected = len(m.filtered) - 1
	}
}

func (m *Model) applyFilter() {
	m.filtered = m.filtered[:0]
	filter := strings.ToLower(strings.TrimSpace(m.filter))
	for index, row := range m.rows {
		if row.RepositoryOnly && !m.showAllRepos {
			continue
		}
		search := row.Organization + " " + row.Repo + " " + row.EnvFile + " " + row.DriftStatus + " " + row.BackupStatus + " " + row.DiffSummary + " " + row.RemoteURL + " " + row.EnvSuggestedFrom
		if filter == "" || strings.Contains(strings.ToLower(search), filter) {
			m.filtered = append(m.filtered, index)
		}
	}
	if len(m.filtered) == 0 || m.selected >= len(m.filtered) {
		m.selected = 0
	}
}

func (m Model) renderPanels(width int) string {
	style := panelStyle
	if m.focus == "orgs" || m.focus == "files" {
		style = focusStyle
	}
	contentWidth := panelContentWidth(style, width)

	gapWidth := 3
	leftWidth := 24
	if width < 100 {
		leftWidth = 20
	}
	minFileWidth := 50
	if contentWidth < leftWidth+gapWidth+minFileWidth {
		leftWidth = max(16, min(leftWidth, contentWidth-gapWidth-minFileWidth))
	}
	fileWidth := max(1, contentWidth-leftWidth-gapWidth)
	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderSidebar(leftWidth),
		mutedStyle.Render(" │ "),
		m.renderFilesContent(fileWidth),
	)
	return style.Width(contentWidth).Render(body)
}

func (m Model) renderAddOrg(width int) string {
	contentWidth := panelContentWidth(focusStyle, width)
	var body strings.Builder
	body.WriteString(headerStyle.Render("ADD ORGANIZATION"))
	body.WriteString("\n")

	for index, field := range m.setupFields {
		value := field.value
		if index == 3 && value != "" {
			value = strings.Repeat("*", len([]rune(value)))
		}
		if value == "" {
			value = mutedStyle.Render("required")
		}
		line := formatCell(fmt.Sprintf("%-22s %s", field.label, value), contentWidth)
		if index == m.setupFocused {
			line = selectedStyle.Render(line)
		}
		body.WriteString(line)
		body.WriteString("\n")
	}

	return focusStyle.Width(contentWidth).Render(strings.TrimRight(body.String(), "\n"))
}

func (m Model) renderSidebar(width int) string {
	var body strings.Builder
	orgHeader := headerStyle.Render(formatCell("ORGANIZATIONS", width))
	if m.focus == "orgs" {
		orgHeader = selectedStyle.Render(formatCell("ORGANIZATIONS", width))
	}
	body.WriteString(orgHeader)
	body.WriteString("\n")
	if len(m.orgs) == 0 {
		body.WriteString(mutedStyle.Render(formatCell("none configured", width)))
	} else {
		for index, org := range m.orgs {
			mark := " "
			if org.Active {
				mark = "*"
			}
			line := formatCell(fmt.Sprintf("%s %s", mark, org.Name), width)
			if index == m.selectedOrg && m.focus == "orgs" {
				line = selectedStyle.Render(line)
			}
			body.WriteString(line)
			if index < len(m.orgs)-1 {
				body.WriteString("\n")
			}
		}
	}
	body.WriteString("\n\n")
	dependencyHeader := headerStyle.Render(formatCell("DEPENDENCIES", width))
	body.WriteString(dependencyHeader)
	body.WriteString("\n")
	if len(m.dependencies) == 0 {
		body.WriteString(mutedStyle.Render(formatCell("not checked", width)))
	} else {
		for index, dep := range m.dependencies {
			body.WriteString(formatCell(dependencyLine(dep, width), width))
			if index < len(m.dependencies)-1 {
				body.WriteString("\n")
			}
		}
	}
	return body.String()
}

func (m Model) renderFilesContent(width int) string {
	var body strings.Builder
	header := m.renderHeader(width)
	if m.focus == "files" {
		header = selectedStyle.Render(fixedColumns(width, "REPOSITORY", "ENV", "DRIFT", "GIT", "REPO BACKUP", "LAST BACKUP", "COMPARE"))
	}
	body.WriteString(header)
	body.WriteString("\n")
	body.WriteString(m.renderRows(width))
	return body.String()
}

func (m Model) renderHeader(width int) string {
	return headerStyle.Render(fixedColumns(width, "REPOSITORY", "ENV", "DRIFT", "GIT", "REPO BACKUP", "LAST BACKUP", "COMPARE"))
}

func (m Model) renderRows(width int) string {
	if len(m.filtered) == 0 {
		return mutedStyle.Render("No env files match the current filter.")
	}

	limit := m.visibleRowLimit()
	start := 0
	if m.selected >= limit {
		start = m.selected - limit + 1
	}
	end := min(len(m.filtered), start+limit)

	lines := make([]string, 0, end-start)
	for visibleIndex := start; visibleIndex < end; visibleIndex++ {
		row := m.rows[m.filtered[visibleIndex]]
		line := fixedColumns(
			width,
			row.Repo,
			row.envLabel(),
			row.driftLabel(),
			boolLabel(row.GitPresent),
			row.BackupStatus,
			row.BackupAt,
			row.compareLabel(),
		)
		if visibleIndex == m.selected && m.focus == "files" {
			line = fixedColumnsSelected(
				width,
				row.Repo,
				row.envLabel(),
				row.driftLabel(),
				boolLabel(row.GitPresent),
				row.BackupStatus,
				row.BackupAt,
				row.compareLabel(),
			)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m Model) visibleRowLimit() int {
	if m.height <= 0 {
		return 12
	}
	return max(6, m.height-16)
}

func (m Model) renderDetail(width int) string {
	contentWidth := panelContentWidth(detailBoxStyle, width)
	if len(m.filtered) == 0 {
		return detailBoxStyle.Width(contentWidth).Render("No selected env file.")
	}
	row := m.rows[m.filtered[m.selected]]
	if row.EnvSuggestedFrom != "" {
		detail := fmt.Sprintf(
			"%s/%s\norg: %s\ngit: %s  env: suggested from %s\nremote: %s\ncurrent: %s missing\nimported: never\nlast backup: never\ncomparison: create %s from %s",
			row.Repo,
			row.EnvFile,
			emptyAs(row.Organization, "default"),
			boolLabel(row.GitPresent),
			row.EnvSuggestedFrom,
			emptyAs(row.RemoteURL, "unknown"),
			row.EnvFile,
			row.EnvFile,
			row.EnvSuggestedFrom,
		)
		return detailBoxStyle.Width(contentWidth).Render(detail)
	}
	if row.RepositoryOnly || row.EnvFile == "" {
		detail := fmt.Sprintf(
			"%s\norg: %s\ngit: %s  env: none\nremote: %s\ncurrent: no visible env files\nimported: never\nlast backup: never\ncomparison: no env file",
			row.Repo,
			emptyAs(row.Organization, "default"),
			boolLabel(row.GitPresent),
			emptyAs(row.RemoteURL, "unknown"),
		)
		return detailBoxStyle.Width(contentWidth).Render(detail)
	}
	detail := fmt.Sprintf(
		"%s/%s\norg: %s\ngit: %s  env: %s\nremote: %s\ncurrent: %s\nimported: %s\nlast backup: %s\ncomparison: %s",
		row.Repo,
		row.EnvFile,
		emptyAs(row.Organization, "default"),
		boolLabel(row.GitPresent),
		boolLabel(row.EnvPresent),
		emptyAs(row.RemoteURL, "unknown"),
		emptyAs(row.CurrentAt, "missing"),
		emptyAsNever(row.ImportedAt),
		emptyAsNever(row.BackupAt),
		emptyAs(row.DiffSummary, "unlock to compare"),
	)
	return detailBoxStyle.Width(contentWidth).Render(detail)
}

func (r Row) envLabel() string {
	if r.EnvSuggestedFrom != "" {
		return emptyAs(r.EnvFile, ".env")
	}
	if r.RepositoryOnly || r.EnvFile == "" {
		return "none"
	}
	return r.EnvFile
}

func (r Row) compareLabel() string {
	if r.EnvSuggestedFrom != "" {
		return "sample"
	}
	return emptyAs(r.DiffSummary, "locked")
}

func (r Row) driftLabel() string {
	if r.DriftStatus == "env_suggested" {
		return "suggested"
	}
	return r.DriftStatus
}

func (m Model) renderDiff(width int) string {
	var body strings.Builder
	body.WriteString(titleStyle.Render("diff"))
	body.WriteString(" ")
	body.WriteString(mutedStyle.Render("esc close"))
	body.WriteString("\n\n")
	body.WriteString(m.diffView)
	return body.String()
}

func (m Model) renderHelp() string {
	if m.diffView != "" {
		return helpItems("esc", "close diff", "q", "quit")
	}
	if m.pendingAction != "" {
		return helpItems("y", "confirm", "n", "cancel", "esc", "cancel")
	}
	if m.unlocking {
		return helpItems("enter", "unlock", "esc", "cancel", "backspace", "edit")
	}
	if m.cloning {
		return helpItems("enter", "clone", "esc", "cancel", "backspace", "edit")
	}
	if m.addingOrg {
		return helpItems("enter", "next/create", "tab", "move", "ctrl+u", "clear", "esc", "cancel")
	}
	if m.typing {
		return helpItems("/", "filtering", "enter", "accept", "esc", "cancel", "backspace", "edit")
	}
	if m.focus == "orgs" {
		return helpItems("tab", "focus files", "j/k", "move", "enter", "select", "a", "add org", "x", "remove org", "R", "reset backups", "q", "quit")
	}
	return helpItems("tab", "focus", "j/k", "move", "/", "filter", "e", "env-only/all repos", "u", "unlock/compare", "d", "diff", "i", "import", "b", "backup", "r", "restore", "c", "clone", "q", "quit")
}

func helpItems(parts ...string) string {
	items := make([]string, 0, len(parts)/2)
	for index := 0; index+1 < len(parts); index += 2 {
		items = append(items, helpKeyStyle.Render(parts[index])+" "+helpStyle.Render(parts[index+1]))
	}
	return strings.Join(items, helpStyle.Render("  "))
}

func (m Model) missingRequiredDependencies() int {
	missing := 0
	for _, dep := range m.dependencies {
		if dep.Required && !dep.Available {
			missing++
		}
	}
	return missing
}

func dependencyLine(dep Dependency, width int) string {
	status := "ok"
	style := cleanStyle
	if !dep.Available {
		status = "missing"
		style = warnStyle
		if dep.Required {
			style = missingStyle
		}
	}

	requirement := "optional"
	if dep.Required {
		requirement = "required"
	}
	detail := dep.Detail
	if detail == "" {
		detail = requirement
	}
	line := fmt.Sprintf("%s  %s  %s", dep.Name, status, detail)
	return style.Render(truncate(line, max(8, width-4)))
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

		rows, err := refreshRows(actions, row.Organization)
		if err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{
			rows:    rows,
			message: message,
		}
	}
}

func runClone(actions Actions, row Row, cloneURL string) tea.Cmd {
	return func() tea.Msg {
		message, err := actions.Clone(row, cloneURL)
		if err != nil {
			return actionResultMsg{err: err}
		}
		rows, err := refreshRows(actions, row.Organization)
		if err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{rows: rows, message: message}
	}
}

func runSelectOrg(selectOrg func(string) ([]Org, []Row, string, error), org string) tea.Cmd {
	return func() tea.Msg {
		orgs, rows, message, err := selectOrg(org)
		if err != nil {
			return actionResultMsg{err: err}
		}
		return actionResultMsg{orgs: orgs, rows: rows, selectedOrg: org, message: message}
	}
}

func runAddOrg(addOrg func(SetupInput) ([]Org, []Row, string, error), input SetupInput) tea.Cmd {
	return func() tea.Msg {
		orgs, rows, message, err := addOrg(input)
		return setupDashboardResultMsg{
			orgs:        orgs,
			rows:        rows,
			selectedOrg: input.Name,
			message:     message,
			err:         err,
		}
	}
}

func runOrgAction(action func(string) ([]Org, []Row, string, error), org string) tea.Cmd {
	return func() tea.Msg {
		if action == nil {
			return actionResultMsg{err: fmt.Errorf("organization action is unavailable")}
		}
		orgs, rows, message, err := action(org)
		return actionResultMsg{
			orgs:        orgs,
			rows:        rows,
			selectedOrg: selectedOrgFromRows(orgs, rows),
			message:     message,
			err:         err,
		}
	}
}

func selectedOrgFromRows(orgs []Org, rows []Row) string {
	for _, org := range orgs {
		if org.Active {
			return org.Name
		}
	}
	if len(rows) > 0 {
		return rows[0].Organization
	}
	return ""
}

func runUnlock(actions Actions, org string, passphrase string) tea.Cmd {
	return func() tea.Msg {
		if actions.UnlockOrg != nil {
			rows, message, err := actions.UnlockOrg(org, passphrase)
			return unlockResultMsg{rows: rows, message: message, err: err}
		}
		message, err := actions.Unlock(passphrase)
		return unlockResultMsg{message: message, err: err}
	}
}

func runDiff(diffAction func(Row) (string, error), row Row) tea.Cmd {
	return func() tea.Msg {
		content, err := diffAction(row)
		return diffResultMsg{content: content, err: err}
	}
}

func refreshRows(actions Actions, org string) ([]Row, error) {
	if actions.RefreshOrg != nil {
		return actions.RefreshOrg(org)
	}
	if actions.Refresh == nil {
		return nil, fmt.Errorf("refresh action is unavailable")
	}
	return actions.Refresh()
}

func normalizeOrgs(orgs []Org, rows []Row) []Org {
	if len(orgs) > 0 {
		return orgs
	}
	if len(rows) == 0 || rows[0].Organization == "" {
		return nil
	}
	return []Org{{Name: rows[0].Organization, Active: true}}
}

func selectedOrgIndex(orgs []Org) int {
	for index, org := range orgs {
		if org.Active {
			return index
		}
	}
	return 0
}

func indexOrg(orgs []Org, name string) int {
	if name == "" {
		return selectedOrgIndex(orgs)
	}
	for index, org := range orgs {
		if org.Name == name {
			return index
		}
	}
	return selectedOrgIndex(orgs)
}

func panelContentWidth(style lipgloss.Style, outerWidth int) int {
	return max(1, outerWidth-style.GetHorizontalBorderSize())
}

type tableColumns struct {
	repo     int
	env      int
	drift    int
	git      int
	backup   int
	backupAt int
	compare  int
}

func fixedColumns(width int, repo string, env string, drift string, git string, backup string, backupAt string, compare string) string {
	columns := tableLayout(width)
	return formatColumns(columns, tableValues(repo, env, drift, git, backup, backupAt, compare), false)
}

func fixedColumnsSelected(width int, repo string, env string, drift string, git string, backup string, backupAt string, compare string) string {
	columns := tableLayout(width)
	return formatColumns(columns, tableValues(repo, env, drift, git, backup, backupAt, compare), true)
}

func tableValues(repo string, env string, drift string, git string, backup string, backupAt string, compare string) []string {
	return []string{
		repo,
		env,
		drift,
		git,
		backup,
		emptyAsNever(backupAt),
		compare,
	}
}

func tableLayout(width int) tableColumns {
	columns := tableColumns{
		repo:     12,
		env:      10,
		drift:    7,
		git:      3,
		backup:   6,
		backupAt: 8,
		compare:  6,
	}
	columns = shrinkTableLayout(columns, width)
	preferred := tableColumns{
		repo:     32,
		env:      16,
		drift:    9,
		git:      3,
		backup:   12,
		backupAt: 24,
		compare:  18,
	}

	available := max(0, width-6-tableWidth(columns))
	available = growColumn(&columns.repo, preferred.repo, available)
	available = growColumn(&columns.backupAt, preferred.backupAt, available)
	available = growColumn(&columns.compare, preferred.compare, available)
	available = growColumn(&columns.backup, preferred.backup, available)
	available = growColumn(&columns.env, preferred.env, available)
	available = growColumn(&columns.drift, preferred.drift, available)
	available = growColumn(&columns.git, preferred.git, available)

	if available > 0 {
		columns.repo += (available + 1) / 2
		columns.backupAt += available / 2
	}
	return columns
}

func shrinkTableLayout(columns tableColumns, width int) tableColumns {
	target := max(0, width-6)
	if tableWidth(columns) <= target {
		return columns
	}

	floors := tableColumns{
		repo:     8,
		env:      6,
		drift:    5,
		git:      2,
		backup:   5,
		backupAt: 5,
		compare:  5,
	}
	needed := tableWidth(columns) - target
	shrinkColumn := func(column *int, floor int) {
		if needed <= 0 || *column <= floor {
			return
		}
		reduction := min(*column-floor, needed)
		*column -= reduction
		needed -= reduction
	}

	shrinkColumn(&columns.repo, floors.repo)
	shrinkColumn(&columns.backupAt, floors.backupAt)
	shrinkColumn(&columns.env, floors.env)
	shrinkColumn(&columns.backup, floors.backup)
	shrinkColumn(&columns.drift, floors.drift)
	shrinkColumn(&columns.compare, floors.compare)
	shrinkColumn(&columns.git, floors.git)
	return columns
}

func growColumn(width *int, preferred int, available int) int {
	if available <= 0 || *width >= preferred {
		return available
	}
	growth := min(preferred-*width, available)
	*width += growth
	return available - growth
}

func tableWidth(columns tableColumns) int {
	return columns.repo + columns.env + columns.drift + columns.git + columns.backup + columns.backupAt + columns.compare
}

func formatColumns(columns tableColumns, values []string, selected bool) string {
	widths := []int{columns.repo, columns.env, columns.drift, columns.git, columns.backup, columns.backupAt, columns.compare}
	cells := make([]string, 0, len(widths))
	for index, width := range widths {
		cell := formatCell(values[index], width)
		switch index {
		case 2, 4:
			cell = renderStatusCell(values[index], cell, selected)
		default:
			if selected {
				cell = selectedStyle.Render(cell)
			}
		}
		cells = append(cells, cell)
	}
	separator := " "
	if selected {
		separator = selectedStyle.Render(separator)
	}
	return strings.Join(cells, separator)
}

func renderStatusCell(status string, value string, selected bool) string {
	style := statusStyle(status)
	if selected {
		style = style.Background(lipgloss.Color("24"))
	}
	return style.Render(value)
}

func formatCell(value string, width int) string {
	value = truncate(value, width)
	padding := width - lipgloss.Width(value)
	if padding > 0 {
		value += strings.Repeat(" ", padding)
	}
	return value
}

func styleStatus(status string) string {
	return statusStyle(status).Render(status)
}

func statusStyle(status string) lipgloss.Style {
	switch status {
	case "clean", "backed_up":
		return cleanStyle
	case "drift", "backup_due", "env_missing", "env_suggested", "suggested":
		return warnStyle
	case "missing", "none", "repo_missing", "no_env":
		return missingStyle
	default:
		return lipgloss.NewStyle()
	}
}

func boolLabel(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func emptyAsNever(value string) string {
	return emptyAs(value, "never")
}

func emptyAs(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func truncate(value string, width int) string {
	if width <= 0 || lipgloss.Width(value) <= width {
		return value
	}
	if width <= 1 {
		runes := []rune(value)
		return string(runes[:width])
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}
