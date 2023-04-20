package common

import tea "github.com/charmbracelet/bubbletea"

type ModalUpdateFunc func(msg tea.Msg) (tea.Model, tea.Cmd)

type SetModalUpdateFuncMsg ModalUpdateFunc
type UpdateSeenModulesMsg []string
type ModuleSelectedMsg string // Emitted to inform all components that a new module has been selected.

func EmitModuleSelectedMsg(moduleName string) tea.Cmd {
	return func() tea.Msg {
		return ModuleSelectedMsg(moduleName)
	}
}
