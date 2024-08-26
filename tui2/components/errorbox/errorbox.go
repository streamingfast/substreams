package errorbox

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/keymap"
	"github.com/streamingfast/substreams/tui2/styles"
)

type ErrorBox struct {
	common.Common
	common.SimpleHelp

	errorMessage string
}

func New(c common.Common, errorMessage string) *ErrorBox {

	return &ErrorBox{
		Common:       c,
		SimpleHelp:   common.NewSimpleHelp(keymap.EscapeModal),
		errorMessage: errorMessage,
	}
}

func (b *ErrorBox) Init() tea.Cmd {
	return nil
}

func (b *ErrorBox) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "enter":
			return b, common.CancelModalCmd()
		}
	}
	return b, nil
}

func (b *ErrorBox) IsFullWidthModal() {}

func (b *ErrorBox) View() string {
	formatted := ansi.Wrap(b.errorMessage, b.Width, "") // wrapping and linecount always recomputed on SetSize
	msg := styles.StreamError.Width(b.Width).Render(formatted)

	return lipgloss.JoinVertical(lipgloss.Top,
		lipgloss.PlaceHorizontal(b.Width, lipgloss.Center, "ERROR - [esc] to dismiss"),
		"",
		msg,
	)
}

func (b *ErrorBox) SetSize(w, h int) {
	b.Common.SetSize(w, h)
}
