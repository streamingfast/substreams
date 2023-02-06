package blockselect

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/stretchr/testify/assert"
)

func TestBlockSelect_Bar(t *testing.T) {
	b := &BlockSelect{
		Common:         common.Common{Width: 45},
		blocksWithData: []uint64{2, 4, 6, 18},
		activeBlock:    18,
		lowBlock:       1,
		highBlock:      20,
	}
	assert.Equal(t, b.View(), `1 --- 20                           
 | | |           |                 
              18 ^                 `)
}

func TestBlockSelect_Update(t *testing.T) {
	b := &BlockSelect{
		blocksWithData: []uint64{2, 4, 6},
		activeBlock:    5,
	}
	b.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'o'}}))
	assert.Equal(t, 4, int(b.activeBlock))
}
