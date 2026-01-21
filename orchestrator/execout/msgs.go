package execout

import (
	"time"

	"github.com/streamingfast/substreams/orchestrator/loop"
)

type MsgDownloadSegment struct {
	loop.IsMsg
	Wait time.Duration
}

type MsgWaitFirstTier2MapSegmentStreamed struct {
	loop.IsMsg
}

type MsgFileDownloaded struct{ loop.IsMsg }
type MsgFileNotPresent struct {
	loop.IsMsg
	NextWait time.Duration
} // In which case, simply re-issue the CmdDownloadFile

type MsgFileReadTransientError struct {
	loop.IsMsg
	Error    error
	NextWait time.Duration
} // In which case, simply re-issue the CmdDownloadFile

type MsgWalkerCompleted struct{ loop.IsMsg }

func CmdWalkerCompleted() loop.Cmd {
	return func() loop.Msg {
		return MsgWalkerCompleted{}
	}
}

func CmdDownloadSegment(wait time.Duration) loop.Cmd {
	return func() loop.Msg {
		return MsgDownloadSegment{Wait: wait}
	}
}

func CmdWaitFirstTier2MapSegmentStreamed(wait time.Duration) loop.Cmd {
	return loop.Tick(wait, MsgWaitFirstTier2MapSegmentStreamed{})
}
