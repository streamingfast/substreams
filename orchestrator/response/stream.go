package response

import (
	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

type Stream struct {
	respFunc substreams.ResponseFunc
}

func New(respFunc substreams.ResponseFunc) *Stream {
	return &Stream{
		respFunc: respFunc,
	}
}

func (s *Stream) BlockScopedData(in *pbsubstreamsrpc.BlockScopedData) error {
	return s.respFunc(substreams.NewBlockScopedDataResponse(in))
}

func (s *Stream) SendModulesStats(stats []*pbsubstreamsrpc.ModuleStats, stages []*pbsubstreamsrpc.Stage, jobs []*pbsubstreamsrpc.Job, bytesRead, bytesWritten uint64) error {
	return s.respFunc(&pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Progress{
			Progress: &pbsubstreamsrpc.ModulesProgress{
				ModulesStats: stats,
				Stages:       stages,
				RunningJobs:  jobs,
				ProcessedBytes: &pbsubstreamsrpc.ProcessedBytes{
					TotalBytesRead:    bytesRead,
					TotalBytesWritten: bytesWritten,
				},
			},
		},
	})
}

func (s *Stream) RPCFailedProgressResponse(reason string, logs []string, logsTruncated bool) error {
	return s.respFunc(&pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_FatalError{
			FatalError: &pbsubstreamsrpc.Error{
				Reason:        reason,
				Logs:          logs,
				LogsTruncated: logsTruncated,
			},
		},
	})
}
