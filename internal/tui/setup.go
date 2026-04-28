package tui

import (
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type SetupInput struct {
	Name             string
	RepoRoot         string
	StoreRoot        string
	MasterPassphrase string
}

type SetupResult struct {
	Input   SetupInput
	Created bool
	Message string
}

type SetupActions struct {
	Create func(SetupInput) (string, error)
}

type SetupModel struct {
	fields     []setupField
	focused    int
	width      int
	height     int
	status     string
	submitting bool
	result     SetupResult
	actions    SetupActions
}

type setupField struct {
	label string
	value string
}

type setupResultMsg struct {
	message string
	err     error
	input   SetupInput
}

var focusedFieldStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Padding(0, 1)

func NewSetupModel(actions SetupActions) SetupModel {
	return SetupModel{
		fields:  newSetupFields(),
		actions: actions,
	}
}

func RunSetup(output io.Writer, actions SetupActions) (SetupResult, error) {
	model, err := tea.NewProgram(NewSetupModel(actions), tea.WithOutput(output)).Run()
	if err != nil {
		return SetupResult{}, err
	}
	setupModel, ok := model.(SetupModel)
	if !ok {
		return SetupResult{}, fmt.Errorf("unexpected setup model result")
	}
	return setupModel.result, nil
}

func (m SetupModel) Init() tea.Cmd {
	return nil
}

func (m SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case setupResultMsg:
		m.submitting = false
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
			return m, nil
		}
		m.result = SetupResult{
			Input:   msg.input,
			Created: true,
			Message: msg.message,
		}
		return m, tea.Quit
	case tea.KeyMsg:
		if m.submitting {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "tab", "down":
			m.moveFocus(1)
		case "shift+tab", "up":
			m.moveFocus(-1)
		case "enter":
			if strings.TrimSpace(m.fields[m.focused].value) == "" {
				m.status = "error: " + setupFieldRequiredMessage(m.focused)
				return m, nil
			}
			if m.focused < len(m.fields)-1 {
				m.moveFocus(1)
				return m, nil
			}
			input := m.input()
			if err := validateSetupInput(input); err != nil {
				m.status = "error: " + err.Error()
				return m, nil
			}
			if m.actions.Create == nil {
				m.status = "error: setup action is unavailable"
				return m, nil
			}
			m.submitting = true
			m.status = "creating organization..."
			return m, runSetupCreate(m.actions.Create, input)
		case "backspace":
			m.deleteLastRune()
		case "ctrl+u":
			m.fields[m.focused].value = ""
		default:
			if len(msg.Runes) > 0 {
				m.fields[m.focused].value += string(msg.Runes)
			}
		}
	}
	return m, nil
}

func (m SetupModel) View() string {
	width := m.width
	if width <= 0 {
		width = 96
	}

	var body strings.Builder
	body.WriteString(titleStyle.Render("dot-vault setup"))
	body.WriteString(" ")
	body.WriteString(mutedStyle.Render("first organization"))
	body.WriteString("\n\n")

	for index, field := range m.fields {
		value := field.value
		if index == 3 && value != "" {
			value = strings.Repeat("*", len([]rune(value)))
		}
		line := fmt.Sprintf("%-22s %s", field.label, value)
		if field.value == "" {
			line = fmt.Sprintf("%-22s %s", field.label, mutedStyle.Render("required"))
		}
		if index == m.focused {
			line = focusedFieldStyle.Render(line)
		}
		body.WriteString(line)
		body.WriteString("\n")
	}

	if m.status != "" {
		body.WriteString("\n")
		body.WriteString(mutedStyle.Render(m.status))
		body.WriteString("\n")
	}

	body.WriteString("\n")
	body.WriteString(helpStyle.Render("enter next/create  tab move  ctrl+u clear  esc quit"))

	return appStyle.Width(width).Render(body.String())
}

func (m *SetupModel) moveFocus(delta int) {
	m.focused += delta
	if m.focused < 0 {
		m.focused = len(m.fields) - 1
	}
	if m.focused >= len(m.fields) {
		m.focused = 0
	}
}

func (m *SetupModel) deleteLastRune() {
	value := m.fields[m.focused].value
	if value == "" {
		return
	}
	runes := []rune(value)
	m.fields[m.focused].value = string(runes[:len(runes)-1])
}

func (m SetupModel) input() SetupInput {
	return SetupInput{
		Name:             strings.TrimSpace(m.fields[0].value),
		RepoRoot:         strings.TrimSpace(m.fields[1].value),
		StoreRoot:        strings.TrimSpace(m.fields[2].value),
		MasterPassphrase: m.fields[3].value,
	}
}

func validateSetupInput(input SetupInput) error {
	if input.Name == "" {
		return fmt.Errorf("organization is required")
	}
	if input.RepoRoot == "" {
		return fmt.Errorf("repository root is required")
	}
	if input.StoreRoot == "" {
		return fmt.Errorf("encrypted store root is required")
	}
	if len([]byte(input.MasterPassphrase)) < 12 {
		return fmt.Errorf("master passphrase must be at least 12 bytes")
	}
	return nil
}

func setupFieldRequiredMessage(index int) string {
	switch index {
	case 0:
		return "organization is required"
	case 1:
		return "repository root is required"
	case 2:
		return "encrypted store root is required"
	case 3:
		return "master passphrase is required"
	default:
		return "field is required"
	}
}

func runSetupCreate(create func(SetupInput) (string, error), input SetupInput) tea.Cmd {
	return func() tea.Msg {
		message, err := create(input)
		return setupResultMsg{
			message: message,
			err:     err,
			input:   input,
		}
	}
}
