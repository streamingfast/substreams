package tui

type connectingSignals int

const (
	Connecting connectingSignals = iota
	Connected
)

func (ui *TUI) Connecting() {
	ui.prog.Send(Connecting)
}
func (ui *TUI) Connected() {
	ui.prog.Send(Connected)
}
