package common

import tea "github.com/charmbracelet/bubbletea"

// Common is a struct all components should embed.
type Common struct {
	Width  int
	Height int
}

// SetSize sets the width and height of the common struct.
func (c *Common) SetSize(width, height int) {
	c.Width = width
	c.Height = height
}

func (c *Common) GetWidth() int  { return c.Width }
func (c *Common) GetHeight() int { return c.Height }

// BlockContext represents a module and block number combination
type BlockContext struct {
	Module   string
	BlockNum uint64
}

// TUIConfig contains TUI-specific configuration that doesn't belong in SinkerConfig
type TUIConfig struct {
	ManifestPath  string
	HomeDir       string
	Vcr           bool
	Headers       map[string]string
	StartBlock    int64
	StopBlock     string
	Cursor        string
	Params        string
	DefaultParams string
	OutputModule  string
	RequiresBuild bool
}

type SetupNewInstanceMsg struct {
	StartStream bool
}

func SetupNewInstanceCmd(startStream bool) tea.Cmd {
	return func() tea.Msg { return SetupNewInstanceMsg{StartStream: startStream} }
}
