package blockselect

import (
	"fmt"
	"log"
	"strings"

	"github.com/dustin/go-humanize"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
)

type BlockSelectedMsg uint64

type BlockSelect struct {
	common.Common

	blocksWithData []uint64
	activeBlock    uint64
	lowBlock       uint64
	highBlock      uint64
}

func New(c common.Common) *BlockSelect {
	return &BlockSelect{Common: c}
}

func (b *BlockSelect) Init() tea.Cmd {
	return nil
}

func (b *BlockSelect) SetAvailableBlocks(blocks []uint64) {
	if len(b.blocksWithData) == 0 && len(blocks) != 0 {
		b.activeBlock = blocks[0]
	}
	b.blocksWithData = blocks
}

func (b *BlockSelect) SetActiveBlock(blockNum uint64) {
	b.activeBlock = blockNum
}

func (b *BlockSelect) StretchBounds(low, high uint64) {
	b.lowBlock = low
	b.highBlock = high
}

func (b *BlockSelect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if len(b.blocksWithData) == 0 {
			break
		}
		key := msg.String()
		switch key {
		case "o":
			var prevIdx int
			for i, el := range b.blocksWithData {
				if el >= b.activeBlock {
					break
				}
				prevIdx = i
			}
			b.activeBlock = b.blocksWithData[prevIdx]
			cmds = append(cmds, b.dispatchBlockSelected)
		case "p":
			var prevIdx = len(b.blocksWithData) - 1
			for i := prevIdx; i >= 0; i-- {
				el := b.blocksWithData[i]
				if el <= b.activeBlock {
					break
				}
				prevIdx = i
			}
			b.activeBlock = b.blocksWithData[prevIdx]
			cmds = append(cmds, b.dispatchBlockSelected)
		}
	}
	return b, tea.Batch(cmds...)
}

func (b *BlockSelect) dispatchBlockSelected() tea.Msg {
	return BlockSelectedMsg(b.activeBlock)
}

func (b *BlockSelect) OldView() string {
	return Styles.Box.MaxWidth(b.Width).Render(
		lipgloss.JoinVertical(0,
			fmt.Sprintf("Active block: %d", b.activeBlock),
			fmt.Sprintf("Blocks with data: %v", b.blocksWithData),
			fmt.Sprintf("Range: %d - %d", b.lowBlock, b.highBlock),
		),
	)
}

func (b *BlockSelect) View() string {
	if b.Width < 11 || b.highBlock == 0 || b.lowBlock == 0 || b.highBlock == b.lowBlock {
		return ""
	}

	bins := float64(b.Width - 10)
	binSize := float64(b.highBlock-b.lowBlock) / bins
	log.Printf("BlockSelect: high %d low %d binSize %d width %d bins %d", b.highBlock, b.lowBlock, binSize, b.Width, bins)

	ptrs := make([]int, int(bins)+1)
	for _, blk := range b.blocksWithData {
		index := float64(blk-b.lowBlock) / binSize
		ptrs[int(index)] += 1
	}
	var ptrsBar []string
	for _, p := range ptrs {
		chr := " "
		if p == 1 {
			chr = "|"
		} else if p > 1 {
			chr = "‖"
		}
		ptrsBar = append(ptrsBar, chr)
	}

	ptr := int(float64(b.activeBlock-b.lowBlock) / binSize)
	if ptr < 0 {
		return ""
	}

	activeBlock := humanize.Comma(int64(b.activeBlock))
	if ptr < len(activeBlock)+3 {
		activeBlock = fmt.Sprintf("%s^ %s", strings.Repeat(" ", ptr), activeBlock)
	} else {
		activeBlock = fmt.Sprintf("%s%s ^", strings.Repeat(" ", ptr-len(activeBlock)-1), activeBlock)
	}

	return lipgloss.JoinVertical(0,
		fmt.Sprintf("%s --- %s", humanize.Comma(int64(b.lowBlock)), humanize.Comma(int64(b.highBlock))),
		strings.Join(ptrsBar, ""),
		activeBlock,
	)
}

var Styles = struct {
	Box             lipgloss.Style
	SelectedBlock   lipgloss.Style
	UnselectedBlock lipgloss.Style
}{
	Box:             lipgloss.NewStyle().Margin(1, 1),
	SelectedBlock:   lipgloss.NewStyle().Margin(0, 1).Foreground(lipgloss.Color("12")).Bold(true),
	UnselectedBlock: lipgloss.NewStyle().Margin(0, 1),
}
