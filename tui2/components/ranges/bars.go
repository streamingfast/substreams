package ranges

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/stream"
)

type Bars struct {
	common.Common
	linearHandoff uint64

	labelWidth int
	bars       []*Bar
	barsMap    map[string]*Bar
}

func NewBars(c common.Common, linearHandoff uint64) *Bars {
	return &Bars{
		Common:        c,
		barsMap:       make(map[string]*Bar),
		linearHandoff: linearHandoff,
		labelWidth:    45,
	}
}

func (b *Bars) Init() tea.Cmd { return nil }

func (b *Bars) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case stream.ResponseProgressMsg:
		for _, mod := range msg.Modules {
			bar, found := b.barsMap[mod.Name]
			if !found {
				bar = NewBar(b.Common, mod.Name, b.linearHandoff)
				b.barsMap[mod.Name] = bar
				b.bars = append(b.bars, bar)
				b.SetSize(b.Width, b.Height)
			}
			bar.Update(mod.Type)
		}
		// loop through msg.Modules
		// check if we have a Bar for that module, otherwise create it
		// and update it with the proper messages.
	}
	return b, nil
}

func (b *Bars) SetSize(w, h int) {
	b.Common.SetSize(w, h)
	for _, bar := range b.bars {
		bar.SetSize(w-b.labelWidth, 1)
	}
}

func (b *Bars) View() string {
	var labels []string
	var bars []string
	for idx, bar := range b.bars {
		if idx > b.Height-2 {
			labels = append(labels, "  ...  ")
			bars = append(bars, "")
			continue
		}
		labels = append(labels, lipgloss.NewStyle().Margin(0, 2).Render(bar.name))
		bars = append(bars, bar.View())
	}
	return lipgloss.JoinVertical(0,
		lipgloss.JoinHorizontal(0.5,
			lipgloss.JoinVertical(0, labels...),
			lipgloss.JoinVertical(0, bars...),
		),
	)
}