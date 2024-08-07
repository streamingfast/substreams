package statusbar

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"github.com/streamingfast/substreams/tui2/common"
	"github.com/streamingfast/substreams/tui2/stream"
	"github.com/streamingfast/substreams/tui2/styles"
)

type StatusBar struct {
	common.Common
	state string
	error error

	traceId            string
	linearHandoffBlock uint64
	resolveStartBlock  uint64

	dataPayloads uint64

	totalBytesRead    uint64
	totalBytesWritten uint64

	initCheckpointBlockCount uint64
	lastCheckpointTime       time.Time
	lastCheckpointBlocks     uint64
	lastCheckpointBlockRate  float64
	maxParallelWorkers       uint64
}

func New(c common.Common) *StatusBar {
	return &StatusBar{
		Common: c,
	}
}

func (s *StatusBar) Init() tea.Cmd {
	return nil
}

func (s *StatusBar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case stream.StreamErrorMsg:
		s.state = "Error"
		s.error = msg.(error)
	case *pbsubstreamsrpc.SessionInit:
		s.maxParallelWorkers = msg.MaxParallelWorkers
		s.traceId = msg.TraceId
		s.linearHandoffBlock = msg.LinearHandoffBlock
		s.resolveStartBlock = msg.ResolvedStartBlock
		s.dataPayloads = 0
	case *pbsubstreamsrpc.BlockScopedData:
		s.dataPayloads += 1
		s.state = "Streaming"

	case *pbsubstreamsrpc.ModulesProgress:
		if s.state == "Connected" {
			s.state = "Backprocessing"
			// TODO: when we detect we're backprocessing, let's switch the tab.
			//tabs.SelectTabCmd(int(outputPage))
		}
		if msg.ProcessedBytes != nil {
			s.totalBytesRead = msg.ProcessedBytes.TotalBytesRead
			s.totalBytesWritten = msg.ProcessedBytes.TotalBytesWritten
		}

		var totalBackprocessedBlocks uint64
		for _, j := range msg.RunningJobs {
			totalBackprocessedBlocks += j.ProcessedBlocks
		}
		for _, stage := range msg.Stages {
			for _, r := range stage.CompletedRanges {
				totalBackprocessedBlocks += (r.EndBlock - r.StartBlock)
			}
		}

		if totalBackprocessedBlocks < s.lastCheckpointBlocks {
			break
		}

		if elapsed := time.Since(s.lastCheckpointTime); elapsed > 900*time.Millisecond {
			if s.lastCheckpointBlocks == 0 {
				s.initCheckpointBlockCount = totalBackprocessedBlocks
			} else {
				blockDiff := totalBackprocessedBlocks - s.lastCheckpointBlocks
				s.lastCheckpointBlockRate = float64(blockDiff) * 1000.0 / float64(elapsed.Milliseconds())
			}
			s.lastCheckpointBlocks = totalBackprocessedBlocks
			s.lastCheckpointTime = time.Now()
		}
	}
	switch msg {
	case stream.ConnectingMsg:
		s.state = "Connecting"
	case stream.ConnectedMsg:
		s.state = "Connected"
	case stream.EndOfStreamMsg:
		s.state = "Stream ended"
	case stream.ReplayedMsg:
		s.state = "Replayed from log"
	}

	return s, nil
}

func (s *StatusBar) View() string {
	var line1, line2 []string

	// [ BACKPROCESSING ]  Press 'p' to see progress.

	state := strings.ToUpper(s.state)
	switch state {
	case "BACKPROCESSING":
		state = fmt.Sprintf(
			"%s (%d blocks, at %.1f blocks/s)",
			state, s.lastCheckpointBlocks-s.initCheckpointBlockCount, s.lastCheckpointBlockRate,
		)
	case "STREAMING":
		state = fmt.Sprintf(
			"%s (%d blocks)",
			state, s.dataPayloads,
		)
	case "ERROR":
		errMsg := s.error.Error()
		if len(errMsg) > 20 {
			errMsg = errMsg[:20] + "..."
		}
		state = fmt.Sprintf("%s: %s", state, errMsg)
	}

	line1 = append(line1, styles.StatusBarKey.Render(state))

	line1 = append(line1, styles.StatusBarBranch.Render(
		fmt.Sprintf("%s read / %s written", humanize.Bytes(s.totalBytesRead), humanize.Bytes(s.totalBytesWritten)),
	))

	if s.maxParallelWorkers != 0 {
		line1 = append(line1, styles.StatusBarHelp.Render(
			fmt.Sprintf("%d max workers", s.maxParallelWorkers),
		))
	}

	line2 = append(line2, styles.StatusBarInfo.Render("trace id: "+s.traceId))

	line2 = append(line2, styles.StatusBarValue.Render(fmt.Sprintf("handoff: %d", s.linearHandoffBlock)))
	line2 = append(line2, styles.StatusBarBranch.Render(fmt.Sprintf("start block: %d", s.resolveStartBlock)))

	fullLine1 := lipgloss.JoinHorizontal(lipgloss.Center, line1...)
	fullLine2 := lipgloss.JoinHorizontal(lipgloss.Center, line2...)

	twoLines := lipgloss.JoinVertical(0, fullLine1, fullLine2)
	return twoLines
}

func (s *StatusBar) SetSize(width, height int) {
	s.Common.SetSize(width, lipgloss.Height(s.View()))
}
