package common

import tea "github.com/charmbracelet/bubbletea"

type ModalUpdateFunc func(msg tea.Msg) (tea.Model, tea.Cmd)

type SetModalUpdateFuncMsg ModalUpdateFunc
