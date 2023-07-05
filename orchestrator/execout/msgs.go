package execout

import (
	"time"

	"github.com/streamingfast/substreams/orchestrator/loop"
)

type MsgDownloadSegment struct {
	Wait time.Duration
}

type MsgFileDownloaded struct{}
type MsgFileNotPresent struct {
	NextWait time.Duration
} // In which case, simply re-issue the CmdDownloadFile

type MsgWalkerCompleted struct{}

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
