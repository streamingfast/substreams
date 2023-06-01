package responses

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

func (s *Stream) InitialProgressMessage(in []*pbsubstreamsrpc.ModuleProgress) {
	s.respFunc(substreams.NewModulesProgressResponse(in))
}
