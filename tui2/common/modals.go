package common

import tea "github.com/charmbracelet/bubbletea"

type ModalUpdateFunc func(msg tea.Msg) (tea.Model, tea.Cmd)

type SetModalUpdateFuncMsg ModalUpdateFunc
type UpdateSeenModulesMsg []string
type ModuleSelectedMsg string // Emitted to inform all components that a new module has been selected.
type BlockSelectedMsg uint64  // Emitted to inform all components that a new block has been selected.

func EmitModuleSelectedMsg(moduleName string) tea.Cmd {
	return func() tea.Msg {
		return ModuleSelectedMsg(moduleName)
	}
}

func EmitBlockSelectedMsg(blockNum uint64) tea.Cmd {
	return func() tea.Msg {
		return BlockSelectedMsg(blockNum)
	}
}
