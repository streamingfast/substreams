package common

import tea "github.com/charmbracelet/bubbletea"

type ModalUpdateFunc func(msg tea.Msg) (tea.Model, tea.Cmd)

type SetModalUpdateFuncMsg ModalUpdateFunc

type ModuleModalUpdateFunc func(msg tea.Msg) (tea.Model, tea.Cmd)

type SetModuleModalUpdateFuncMsg ModuleModalUpdateFunc
