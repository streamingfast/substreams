package common

import (
	tea "github.com/charmbracelet/bubbletea"
)

type SetModalComponentMsg Component
type CancelModalMsg struct{} // Emitted by components to close itselt

type IsInlineModal interface {
	IsInlineModal()
}

func SetModalComponentCmd(comp Component) tea.Cmd {
	return func() tea.Msg {
		return SetModalComponentMsg(comp)
	}
}
func CancelModalCmd() tea.Cmd {
	return func() tea.Msg {
		return CancelModalMsg{}
	}
}

type UpdateSeenModulesMsg []string
type ModuleSelectedMsg string // Emitted to inform all components that a new module has been selected.
type BlockSelectedMsg uint64  // Emitted to inform all components that a new block has been selected.

type SetRequestValue struct {
	Field string
	Value string
}

func SetRequestValueCmd(field, value string) tea.Cmd {
	return func() tea.Msg {
		return SetRequestValue{Field: field, Value: value}
	}
}

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
