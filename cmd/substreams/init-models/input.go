package init_models

import (
	"fmt"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type completedState int

const (
	incomplete completedState = iota
	complete
	exited
)

type InputModel struct {
	state     completedState
	TextInput textinput.Model
	err       error
}

type (
	errMsg error
)

func ProjectNameSelection() InputModel {
	ti := textinput.New()
	ti.Placeholder = "Project name"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	return InputModel{
		state:     incomplete,
		TextInput: ti,
		err:       nil,
	}
}

func (m InputModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m InputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter, tea.KeyEsc:
			m.state = complete
			return m, tea.Quit
		case tea.KeyCtrlC:
			m.state = exited
			return m, tea.Quit
		}

	case errMsg:
		m.err = msg
		return m, nil
	}

	m.TextInput, cmd = m.TextInput.Update(msg)
	return m, cmd
}

func (m InputModel) View() string {
	return fmt.Sprintf(
		"What would you like your project to be named?\n\n%s",
		m.TextInput.View(),
	) + "\n"
}
