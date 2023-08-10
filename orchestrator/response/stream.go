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

func (s *Stream) SendModulesStats(stats []*pbsubstreamsrpc.ModuleStats, stages []*pbsubstreamsrpc.Stage, jobs []*pbsubstreamsrpc.Job, processedBytes *pbsubstreamsrpc.ProcessedBytes) error {
	return s.respFunc(&pbsubstreamsrpc.Response{
		Message: &pbsubstreamsrpc.Response_Progress{
			Progress: &pbsubstreamsrpc.ModulesProgress{
				ModulesStats:   stats,
				Stages:         stages,
				RunningJobs:    jobs,
				ProcessedBytes: processedBytes,
			},
		},
	})
}

func (s *Stream) RPCFailedProgressResponse(reason string, logs []string, logsTruncated bool) error {
	// FIXME
	return nil
	//return s.respFunc(&pbsubstreamsrpc.Response{
	//	Message: &pbsubstreamsrpc.Response_Progress{
	//		Progress: &pbsubstreamsrpc.ModulesProgress{
	//			Modules: []*pbsubstreamsrpc.ModuleProgress{
	//				{
	//					Name: moduleName,
	//					Type: &pbsubstreamsrpc.ModuleProgress_Failed_{
	//						Failed: &pbsubstreamsrpc.ModuleProgress_Failed{
	//							Reason:        reason,
	//							Logs:          logs,
	//							LogsTruncated: logsTruncated,
	//						},
	//					},
	//				},
	//			},
	//		},
	//	},
	//})
}

func (s *Stream) RPCRangeProgressResponse(moduleNames []string, start, end uint64) error {
	// FIXME
	return nil
	//var mods []*pbsubstreamsrpc.ModuleProgress
	//for _, moduleName := range moduleNames {
	//	mods = append(mods, &pbsubstreamsrpc.ModuleProgress{
	//		Name: moduleName,
	//		Type: &pbsubstreamsrpc.ModuleProgress_ProcessedRanges_{
	//			ProcessedRanges: &pbsubstreamsrpc.ModuleProgress_ProcessedRanges{
	//				ProcessedRanges: []*pbsubstreamsrpc.BlockRange{
	//					{
	//						StartBlock: start,
	//						EndBlock:   end,
	//					},
	//				},
	//			},
	//		},
	//	})
	//}
	//return s.respFunc(&pbsubstreamsrpc.Response{
	//	Message: &pbsubstreamsrpc.Response_Progress{
	//		Progress: &pbsubstreamsrpc.ModulesProgress{
	//			Modules: mods,
	//		},
	//	},
	//})
}

func (s *Stream) RPCProcessedBytes(
	moduleName string,
	bytesReadDelta uint64,
	bytesWrittenDelta uint64,
	totalBytesRead uint64,
	totalBytesWritten uint64,
	nanoSeconds uint64,
) error {
	// FIXME
	return nil
	//return s.respFunc(&pbsubstreamsrpc.Response{
	//	Message: &pbsubstreamsrpc.Response_Progress{
	//		Progress: &pbsubstreamsrpc.ModulesProgress{
	//			Modules: []*pbsubstreamsrpc.ModuleProgress{
	//				{
	//					Name: moduleName,
	//					Type: &pbsubstreamsrpc.ModuleProgress_ProcessedBytes_{
	//						ProcessedBytes: &pbsubstreamsrpc.ModuleProgress_ProcessedBytes{
	//							BytesReadDelta:    bytesReadDelta,
	//							BytesWrittenDelta: bytesWrittenDelta,
	//							TotalBytesRead:    totalBytesRead,
	//							TotalBytesWritten: totalBytesWritten,
	//							NanoSecondsDelta:  nanoSeconds,
	//						},
	//					},
	//				},
	//			},
	//		},
	//	},
	//})
}

/*
outputstream.Walker
orchestrator/execout/stream.go Stream
orchestrator/execout/walker.go Walker
orchestrator/linear/reader.go Reader
orchestrator/execout/linearreader.go LinearReader
orchestrator/execout/walker.go execout.Walker


*/
