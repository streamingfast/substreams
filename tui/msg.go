package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

type msg int

const (
	Connecting msg = iota
	Connected

	Quit
)

func (ui *TUI) Connecting() {
	ui.send(Connecting)
}
func (ui *TUI) Connected() {
	ui.send(Connected)
}
func (ui *TUI) send(msg tea.Msg) {
	if ui.prog != nil {
		ui.prog.Send(msg)
	}
}

type BlockMessage string

type SessionInitMessage struct {
	TraceID string
}
