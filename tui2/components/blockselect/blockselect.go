package blockselect

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
)

type BlockSelectedMsg uint64

type BlockSelect struct {
	common.Common

	blocksWithData []uint64
	activeBlock    uint64
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

func (b *BlockSelect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if len(b.blocksWithData) == 0 {
			break
		}
		key := msg.String()
		switch key {
		case "o", "p":
			for idx, el := range b.blocksWithData {
				if el == b.activeBlock || b.activeBlock == 0 {
					var newIdx int
					if key == "o" {
						newIdx = (idx - 1 + len(b.blocksWithData)) % len(b.blocksWithData)
					} else {
						newIdx = (idx + 1) % len(b.blocksWithData)
					}
					b.activeBlock = b.blocksWithData[newIdx]
					break
				}
			}
			cmds = append(cmds, b.dispatchBlockSelected)
		}
	}
	return b, tea.Batch(cmds...)
}

func (b *BlockSelect) dispatchBlockSelected() tea.Msg {
	return BlockSelectedMsg(b.activeBlock)
}

func (b *BlockSelect) View() string {
	return Styles.Box.MaxWidth(b.Width).Render(
		lipgloss.JoinVertical(0,
			fmt.Sprintf("Active block: %d", b.activeBlock),
			fmt.Sprintf("Blocks with data: %v", b.blocksWithData),
		),
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
