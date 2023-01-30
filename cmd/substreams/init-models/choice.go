package init_models

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
)

type ChoiceModel struct {
	isCompleted     bool
	projectName     string
	questionContext string
	choices         []string
	cursor          int
	Selected        string
}

func NewChainSelection() ChoiceModel {
	return ChoiceModel{
		isCompleted:     false,
		questionContext: "chain",
		choices:         []string{"Ethereum", "other"},
	}
}

func (m ChoiceModel) Init() tea.Cmd {
	return nil
}

func (m ChoiceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:

		switch msg.String() {

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			ok := m.Selected == m.choices[m.cursor]
			if ok {
				m.isCompleted = true
				return m, tea.Quit
			} else {
				m.Selected = m.choices[m.cursor]
			}
		case "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ChoiceModel) View() string {
	output := fmt.Sprintf("\033[32m ✔"+"\033[0m"+" Name: %s\n", m.projectName)
	output += fmt.Sprintf("What %s would you like your generated substream to be\n\n", m.questionContext)

	for i, choice := range m.choices {

		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}

		checked := " "
		if m.Selected == m.choices[i] {
			checked = "x"
		}
		output += fmt.Sprintf("%s [%s] %s\n", cursor, checked, choice)
	}
	return output
}
