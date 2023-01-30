package init_models

import (
	tea "github.com/charmbracelet/bubbletea"
)

type sessionState int

const (
	nameSelect sessionState = iota
	networkSelect
	completed
)

type Questionaire struct {
	state       sessionState
	ProjectName InputModel
	Network     ChoiceModel
}

func NewQuestionaire() Questionaire {
	return Questionaire{
		state:       nameSelect,
		ProjectName: ProjectNameSelection(),
		Network:     NewChainSelection(),
	}
}

func (m Questionaire) Init() tea.Cmd {
	return nil
}

func (m Questionaire) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch m.state {
	case nameSelect:
		newProjectName, _ := m.ProjectName.Update(msg)
		projectNameModel, _ := newProjectName.(InputModel)
		m.ProjectName = projectNameModel
		if m.ProjectName.state == complete {
			m.state = networkSelect
			m.Network.projectName = m.ProjectName.TextInput.Value()
		} else if m.ProjectName.state == exited {
			return m, tea.Quit
		}
	}

	if m.state == networkSelect {
		newNetwork, _ := m.Network.Update(msg)
		networkModel, _ := newNetwork.(ChoiceModel)
		m.Network = networkModel
		if m.Network.isCompleted == true {
			m.state = completed
			return m, tea.Quit
		}
	}
	return m, cmd
}

func (m Questionaire) View() string {
	switch m.state {
	case nameSelect:
		return m.ProjectName.View()
	case networkSelect:
		return m.Network.View()
	case completed:
		return ""
	}
	return "smth went wrong"
}
