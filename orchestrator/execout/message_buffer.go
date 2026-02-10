package execout

import (
	"fmt"
	"sync"
	"time"

	"github.com/streamingfast/substreams/orchestrator/response"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

type MessageBuffer struct {
	mut                sync.RWMutex
	buf                *pbsubstreamsrpcv2.BlockScopedDatas
	lastFlush          time.Time
	maxBufferedMessage int
}

func NewMessageBuffer(maxBufferedMessage int) *MessageBuffer {
	return &MessageBuffer{
		buf:                &pbsubstreamsrpcv2.BlockScopedDatas{Items: []*pbsubstreamsrpcv2.BlockScopedData{}},
		maxBufferedMessage: maxBufferedMessage,
	}
}

func (b *MessageBuffer) Len() int {
	b.mut.Lock()
	defer b.mut.Unlock()

	return len(b.buf.Items)
}

func (b *MessageBuffer) Append(msg *pbsubstreamsrpcv2.BlockScopedData) {
	b.mut.Lock()
	defer b.mut.Unlock()

	b.buf.Items = append(b.buf.Items, msg)
}

func (b *MessageBuffer) ShouldFlush() bool {
	b.mut.Lock()
	defer b.mut.Unlock()

	if len(b.buf.Items) > b.maxBufferedMessage {
		return true
	}

	return false
}

func (b *MessageBuffer) Flush(streamSrv *response.Stream) error {
	b.mut.Lock()
	defer b.mut.Unlock()

	err := streamSrv.BlockScopedDatas(b.buf)
	if err != nil {
		return fmt.Errorf("flushing buffer: %w", err)
	}

	b.lastFlush = time.Now()
	b.buf = &pbsubstreamsrpcv2.BlockScopedDatas{Items: []*pbsubstreamsrpcv2.BlockScopedData{}}

	return nil
}
