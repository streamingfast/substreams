package blockselect

import (
	"regexp"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/stretchr/testify/assert"
)

const ansi = "[\u001B\u009B][[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[a-zA-Z\\d]*)*)?\u0007)|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PRZcf-ntqry=><~]))"

var re = regexp.MustCompile(ansi)

func Strip(str string) string {
	return re.ReplaceAllString(str, "")
}

func TestBlockSelect_Bar(t *testing.T) {
	b := &BlockSelect{
		Common:         common.Common{Width: 45},
		blocksWithData: []uint64{2, 4, 6, 18},
		activeBlock:    18,
		lowBlock:       1,
		highBlock:      20,
	}
	expected := `┌───────────────────────────────────────────┐
│1                                       20 │
│ | | |           |                         │
│                 ^ Current block: 18       │
└───────────────────────────────────────────┘`
	assert.Equal(t, expected, Strip(b.View()))
}

func TestBlockSelect_Update(t *testing.T) {
	b := &BlockSelect{
		blocksWithData: []uint64{2, 4, 6},
		activeBlock:    5,
	}
	b.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: []rune{'o'}}))
	assert.Equal(t, 4, int(b.activeBlock))
}
