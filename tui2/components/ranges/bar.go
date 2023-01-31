package ranges

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/tui2/common"
)

// Bar is a single progress bar that handles ranges with holes

type Bar struct {
	common.Common
	name           string
	targetEndBlock uint64

	ranges ranges
}

func NewBar(c common.Common, name string, targetEndBlock uint64) *Bar {
	return &Bar{Common: c, name: name, targetEndBlock: targetEndBlock}
}

func (b *Bar) Init() tea.Cmd { return nil }

func (b *Bar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case *pbsubstreams.ModuleProgress_ProcessedRanges:
		for _, v := range msg.ProcessedRanges.ProcessedRanges {
			b.ranges = mergeRangeLists(b.ranges, &blockRange{
				Start: v.StartBlock,
				End:   v.EndBlock,
			})
		}
	}
	return b, nil
}

func (b *Bar) View() string {
	return lipgloss.JoinHorizontal(0,
		lipgloss.NewStyle().Width(20).Margin(0, 2).Render(b.name),
		"["+barmode(b.ranges, b.targetEndBlock, uint64(b.Width-26))+"]",
	)
	// Return the bar
}

func barmode(in ranges, backprocessingCompleteAtBlock, width uint64) string {
	lo := in.Lo()
	hi := backprocessingCompleteAtBlock
	binsize := (hi - lo) / width
	var out []string
	for i := uint64(0); i < width; i++ {
		loCheck := binsize*i + lo
		hiCheck := binsize*(i+1) + lo

		if in.Covered(loCheck, hiCheck) {
			out = append(out, "▓")
		} else if in.PartiallyCovered(loCheck, hiCheck) {
			out = append(out, "▒")
		} else {
			out = append(out, "░")
		}
	}
	return strings.Join(out, "")
}
