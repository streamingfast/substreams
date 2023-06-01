package orchestrator

import (
	"github.com/streamingfast/substreams"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

type StreamOut struct {
	respFunc substreams.ResponseFunc
}

func NewStreamOut(respFunc substreams.ResponseFunc) *StreamOut {
	return &StreamOut{
		respFunc: respFunc,
	}
}

func (s *StreamOut) InitialProgressMessage(in []*pbsubstreamsrpc.ModuleProgress) {
	s.respFunc(substreams.NewModulesProgressResponse(in))
}
