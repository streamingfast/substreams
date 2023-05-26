package ranges

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/common"
)

type Bars struct {
	common.Common
	targetBlock uint64

	TotalBlocks uint64
	BarCount    uint64

	labelWidth int
	Mode       int
	bars       []*Bar
	barsMap    map[string]*Bar
}

func NewBars(c common.Common, targetBlock uint64) *Bars {
	return &Bars{
		Common:      c,
		barsMap:     make(map[string]*Bar),
		targetBlock: targetBlock,
		labelWidth:  45,
	}
}

func (b *Bars) Init() tea.Cmd { return nil }

func (b *Bars) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case *pbsubstreamsrpc.ModulesProgress:
		for _, mod := range msg.Modules {
			bar, found := b.barsMap[mod.Name]
			if !found {
				bar = NewBar(b.Common, mod.Name, b.targetBlock)
				b.barsMap[mod.Name] = bar
				b.bars = append(b.bars, bar)
				b.SetSize(b.Width, b.Height)
			}
			bar.Update(mod.Type)
		}
	}
	var totalBlocks uint64
	for _, bar := range b.bars {
		totalBlocks += bar.totalBlocks
	}
	b.TotalBlocks = totalBlocks
	b.BarCount = uint64(len(b.bars))
	return b, nil
}

func (b *Bars) SetSize(w, h int) {
	b.Common.SetSize(w, h)
	for _, bar := range b.bars {
		bar.SetSize(w-b.labelWidth-1 /* padding here and there */, 1)
	}
}

func (b *Bars) View() string {
	var labels []string
	var bars []string
	for _, bar := range b.bars {
		barName := bar.name
		if len(barName) > b.labelWidth-4 {
			barName = barName[:b.labelWidth-4]
		}
		labels = append(labels, lipgloss.NewStyle().Margin(0, 1).Render(barName))
		switch b.Mode {
		case 0:
			bars = append(bars, bar.View())
		case 1:
			bars = append(bars, bar.RangeView(false))
		case 2:
			bars = append(bars, bar.RangeView(true))
		}
	}
	return lipgloss.JoinVertical(0,
		lipgloss.JoinHorizontal(0.5,
			lipgloss.JoinVertical(0, labels...),
			lipgloss.JoinVertical(0, bars...),
		),
	)
}
