package common

import "github.com/charmbracelet/bubbles/key"

func ShortToFullHelp(comp Component) [][]key.Binding {
	shortHelp := comp.ShortHelp()
	fullHelp := make([][]key.Binding, len(shortHelp))
	for i, binding := range shortHelp {
		fullHelp[i] = []key.Binding{binding}
	}
	return fullHelp
}

type SimpleHelp struct {
	Bindings []key.Binding
}

func NewSimpleHelp(bindings ...key.Binding) SimpleHelp {
	return SimpleHelp{Bindings: bindings}
}

func (h SimpleHelp) ShortHelp() []key.Binding {
	return h.Bindings
}

func (h SimpleHelp) FullHelp() [][]key.Binding {
	shortHelp := h.ShortHelp()
	fullHelp := make([][]key.Binding, len(shortHelp))
	for i, binding := range shortHelp {
		fullHelp[i] = []key.Binding{binding}
	}
	return fullHelp
}
