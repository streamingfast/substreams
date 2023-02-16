package blockselect

import (
	"fmt"
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

func (b *BlockSelect) View() string {
	if b.Width < 4 || b.highBlock == 0 || b.lowBlock == 0 || b.highBlock == b.lowBlock {
		return ""
	}

	bins := float64(b.Width - Styles.Box.GetVerticalBorderSize())
	binSize := float64(b.highBlock-b.lowBlock) / bins
	if binSize < 1 {
		binSize = 1
	}
	//log.Printf("BlockSelect: high %d low %d binSize %d width %d bins %d", b.highBlock, b.lowBlock, binSize, b.Width, bins)

	ptrs := make([]int, int(bins))
	for _, blk := range b.blocksWithData {
		index := int(float64(blk-b.lowBlock) / binSize)
		if index >= len(ptrs) {
			index = len(ptrs) - 1
		}
		ptrs[index] += 1
	}
	var ptrsBar []string
	for _, p := range ptrs {
		chr := " "
		if p == 1 {
			chr = "|"
		} else if p == 2 {
			chr = "‖"
		} else if p > 2 {
			chr = "⫴" // or: ⁞⁝⁚‧
		}
		ptrsBar = append(ptrsBar, chr)
	}

	var activeBlock string
	if b.activeBlock != 0 {
		ptr := int(float64(b.activeBlock-b.lowBlock) / binSize)
		activeBlock = fmt.Sprintf("%s: %s",
			Styles.CurrentBlock.Render("Current block"),
			Styles.SelectedBlock.Render(humanize.Comma(int64(b.activeBlock))),
		)

		labelLen := lipgloss.Width(activeBlock) + 1
		if ptr < labelLen {
			activeBlock = fmt.Sprintf("%s^ %s",
				strings.Repeat(" ", ptr),
				activeBlock,
			)
		} else {
			repeatLen := ptr - labelLen
			if repeatLen < 0 {
				repeatLen = 0
			}
			activeBlock = fmt.Sprintf("%s%s ^",
				strings.Repeat(" ", repeatLen),
				activeBlock,
			)
		}
	}

	highBlockMargin := len(string(humanize.Comma(int64(b.highBlock)))) + len(string(humanize.Comma(int64(b.highBlock)))) + 2
	highBlockStyle := lipgloss.NewStyle().MarginLeft(b.Width - highBlockMargin)

	return Styles.Box.Render(lipgloss.JoinVertical(0,
		fmt.Sprintf("%s%s", humanize.Comma(int64(b.lowBlock)), highBlockStyle.Render(humanize.Comma(int64(b.highBlock)))),
		Styles.Bar.Render(strings.Join(ptrsBar, "")),
		activeBlock,
	))
}

var Styles = struct {
	Box           lipgloss.Style
	SelectedBlock lipgloss.Style
	CurrentBlock  lipgloss.Style
	Bar           lipgloss.Style
}{
	Box:           lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true),
	SelectedBlock: lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	CurrentBlock:  lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	Bar:           lipgloss.NewStyle().Background(lipgloss.Color("235")),
}
