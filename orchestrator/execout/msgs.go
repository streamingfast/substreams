package execout

type MsgStartDownload struct{}
type MsgFileDownloaded struct{}
type MsgFileNotPresent struct{} // In which case, simply re-issue the CmdDownloadFile

type MsgWalkerCompleted struct{}
