package request

import "github.com/charmbracelet/bubbles/key"

func (r *Request) ShortHelp() []key.Binding {
	return []key.Binding{
		key.NewBinding(
			key.WithKeys("up", "k", "down", "j"),
			key.WithHelp("↑/k/↓/j", "up/down"),
		),
		key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "more"),
		),
		key.NewBinding(
			key.WithKeys("q", "esc"),
			key.WithHelp("q", "quit"),
		),
	}
}
func (r *Request) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		r.ShortHelp(),
	}
}
