package footer

import (
	"log"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/styles"
)

// ToggleFooterMsg is a message sent to show/hide the footer.
type ToggleFooterMsg struct{}

type UpdateKeyMapMsg help.KeyMap

// Footer is a Bubble Tea model that displays help and other info.
type Footer struct {
	common.Common
	help   help.Model
	keymap help.KeyMap
}

// New creates a new Footer.
func New(c common.Common, keymap help.KeyMap) *Footer {
	h := help.New()
	h.Styles.ShortKey = styles.HelpKey
	h.Styles.ShortDesc = styles.HelpValue
	h.Styles.FullKey = styles.HelpKey
	h.Styles.FullDesc = styles.HelpValue
	h.Styles.FullSeparator = h.Styles.ShortSeparator
	f := &Footer{
		Common: c,
		help:   h,
		keymap: keymap,
	}
	f.SetSize(c.Width, c.Height)
	return f
}

// SetSize implements common.Component.
func (f *Footer) SetSize(width, height int) {
	f.Common.SetSize(width, height)
	f.help.Width = width -
		styles.Footer.GetHorizontalFrameSize()
}

// Init implements tea.Model.
func (f *Footer) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (f *Footer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case UpdateKeyMapMsg:
		log.Println("Updating keymap in footer")
		f.keymap = msg
	}
	return f, nil
}

func (f *Footer) SetKeyMap(keymap help.KeyMap) {
	f.keymap = keymap
}

// View implements tea.Model.
func (f *Footer) View() string {
	if f.keymap == nil {
		return ""
	}
	helpView := f.help.View(f.keymap)
	return styles.Footer.Width(f.Width).Render(helpView)
}

// ShowAll returns whether the full help is shown.
func (f *Footer) ShowAll() bool {
	return f.help.ShowAll
}

// SetShowAll sets whether the full help is shown.
func (f *Footer) SetShowAll(show bool) {
	f.help.ShowAll = show
}

// Height returns the height of the footer.
func (f *Footer) Height() int {
	return lipgloss.Height(f.View()) + 1
}

func UpdateKeyMapCmd(keymap help.KeyMap) tea.Msg {
	return UpdateKeyMapMsg(keymap)
}
