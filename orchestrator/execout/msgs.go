package execout

import "github.com/streamingfast/substreams/orchestrator/loop"

type MsgStartDownload struct{}
type MsgFileDownloaded struct{}
type MsgFileNotPresent struct{} // In which case, simply re-issue the CmdDownloadFile

type MsgWalkerCompleted struct{}

func CmdMsgStartDownload() loop.Cmd {
	return func() loop.Msg {
		return MsgStartDownload{}
	}
}
