package ranges

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/stream"
)

type Bars struct {
	common.Common
	targetEndBlock uint64

	bars    []*Bar
	barsMap map[string]*Bar
}

func NewBars(c common.Common, targetEndBlock uint64) *Bars {
	return &Bars{
		Common:         c,
		targetEndBlock: targetEndBlock,
	}
}

func (b *Bars) Init() tea.Cmd { return nil }

func (b *Bars) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case stream.ResponseProgressMsg:
		for _, mod := range msg.Modules {
			bar, found := b.barsMap[mod.Name]
			if !found {
				bar = NewBar(b.Common, mod.Name, b.targetEndBlock)
				b.barsMap[mod.Name] = bar
				b.bars = append(b.bars, bar)
			}
			bar.Update(mod.Type)
		}
		// loop through msg.Modules
		// check if we have a Bar for that module, otherwise create it
		// and update it with the proper messages.
	}
	return b, nil
}

func (b *Bars) View() string {
	var bars []string
	for _, bar := range b.bars {
		bars = append(bars, bar.View())
	}
	return lipgloss.JoinVertical(0, bars...)
}
