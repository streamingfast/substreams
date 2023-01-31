package progress

import (
	"fmt"
	"time"

	"github.com/streamingfast/substreams/tui2/components/ranges"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/stream"
)

type Progress struct {
	common.Common
	KeyMap KeyMap

	state string

	progressUpdates   int
	dataPayloads      int
	updatedSecond     int64
	updatesPerSecond  int
	updatesThisSecond int

	bars *ranges.Bars
}

func New(c common.Common, targetEndBlock uint64) *Progress {
	return &Progress{
		Common: c,
		KeyMap: DefaultKeyMap(),
		state:  "Initializing",
		bars:   ranges.NewBars(c, targetEndBlock),
	}
}

func (p *Progress) Init() tea.Cmd { return nil }

func (p *Progress) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case stream.ResponseDataMsg:
		p.dataPayloads += 1
	case stream.ResponseProgressMsg:
		p.progressUpdates += 1
		thisSec := time.Now().Unix()
		if p.updatedSecond != thisSec {
			p.updatesPerSecond = p.updatesThisSecond
			p.updatesThisSecond = 0
			p.updatedSecond = thisSec
		}
		p.updatesThisSecond += 1
		return p, nil
	case stream.StreamErrorMsg:
		p.state = fmt.Sprintf("Error: %s", msg)
	}
	switch msg {
	case stream.ConnectingMsg:
		p.state = "Connecting"
	case stream.ConnectedMsg:
		p.state = "Connected"
	case stream.EndOfStreamMsg:
		p.state = "Stream ended"
	}
	return p, nil
}

func (p *Progress) View() string {
	return lipgloss.JoinVertical(50,
		"Progress view",
		fmt.Sprintf("Progress updates: %d", p.progressUpdates),
		fmt.Sprintf("Data payloads: %d", p.dataPayloads),
		fmt.Sprintf("Per second: %d", p.updatesPerSecond),
		fmt.Sprintf("Status: %s", p.Styles.StatusBarValue.Render(p.state)),
		p.bars.View(),
	)
}
