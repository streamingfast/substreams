package ranges

import (
	"bufio"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
}

func NewBars(c common.Common, targetBlock uint64) *Bars {
	return &Bars{
		Common:      c,
		targetBlock: targetBlock,
		labelWidth:  45,
	}
}

func (b *Bars) Init() tea.Cmd { return nil }

func (b *Bars) NewBar(displayedName string, ranges []*BlockRange, modules []string) *Bar {
	out := NewBar(b.Common, displayedName, b.targetBlock)
	out.ranges = ranges
	out.modules = modules
	return out
}

func (b *Bars) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	b.bars = msg.([]*Bar)
	b.SetSize(b.Width, b.Height)
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
		labels = append(labels, lipgloss.NewStyle().Margin(0, 1).Bold(true).Render(barName))
		switch b.Mode {
		case 0:
			bars = append(bars, bar.View())
		case 1:
			bars = append(bars, bar.RangeView(false))
		case 2:
			bars = append(bars, bar.RangeView(true))
		}
	}

	withoutModules := lipgloss.JoinVertical(0,
		lipgloss.JoinHorizontal(0.5,
			lipgloss.JoinVertical(0, labels...),
			lipgloss.JoinVertical(0, bars...),
		),
	)
	scanner := bufio.NewScanner(strings.NewReader(withoutModules))
	var out string
	i := 0
	for scanner.Scan() {
		out += scanner.Text() + "\n"
		out += lipgloss.NewStyle().Margin(0, 0, 0, 3).Italic(true).Width(b.Width-6).Render(strings.Join(b.bars[i].modules, " ")) + "\n\n"
		i++
	}

	return out
}
