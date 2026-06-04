package execout

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/streamingfast/substreams/orchestrator/response"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	"go.uber.org/zap"
)

type MessageBuffer struct {
	mut                sync.RWMutex
	buf                *pbsubstreamsrpcv4.BlockScopedDatas
	lastFlush          time.Time
	maxBufferedMessage int
	DataSize           int
	logger             *zap.Logger
	maxDataSize        int
}

func NewMessageBuffer(maxBufferedMessage int, logger *zap.Logger) *MessageBuffer {
	maxDataSize := 1024 * 1024 * 10
	maxDataSizeString := os.Getenv("MESSAGE_BUFFER_MAX_DATA_SIZE")

	if maxDataSizeString != "" {
		parsed, err := strconv.Atoi(maxDataSizeString)
		if err != nil {
			logger.Warn("failed to parse MESSAGE_BUFFER_MAX_DATA_SIZE, using default value", zap.Error(err))
		} else {
			maxDataSize = parsed
		}
	}

	return &MessageBuffer{
		buf:                &pbsubstreamsrpcv4.BlockScopedDatas{Items: []*pbsubstreamsrpcv2.BlockScopedData{}},
		maxBufferedMessage: maxBufferedMessage,
		maxDataSize:        maxDataSize,
		logger:             logger.Named("message-buffer"),
	}
}

func (b *MessageBuffer) Len() int {
	b.mut.Lock()
	defer b.mut.Unlock()

	return len(b.buf.Items)
}

func (b *MessageBuffer) Append(msg *pbsubstreamsrpcv2.BlockScopedData, dataSize int) {
	b.mut.Lock()
	defer b.mut.Unlock()

	b.DataSize += dataSize
	b.buf.Items = append(b.buf.Items, msg)
}

func (b *MessageBuffer) ShouldFlush() bool {
	b.mut.Lock()
	defer b.mut.Unlock()

	return b.shouldFlushLocked()
}

func (b *MessageBuffer) shouldFlushLocked() bool {
	if b.DataSize > b.maxDataSize {
		b.logger.Debug("flushing due to large data size", zap.Int("data_size", b.DataSize), zap.Bool("keep", false))
		return true
	}

	if len(b.buf.Items) > b.maxBufferedMessage {
		return true
	}

	return false
}

// AppendAndShouldFlush appends a message and reports whether the buffer should be
// flushed, doing both under a single lock. This is called once per block on the
// output hot path, so it avoids the second lock acquisition of Append+ShouldFlush.
func (b *MessageBuffer) AppendAndShouldFlush(msg *pbsubstreamsrpcv2.BlockScopedData, dataSize int) bool {
	b.mut.Lock()
	defer b.mut.Unlock()

	b.DataSize += dataSize
	b.buf.Items = append(b.buf.Items, msg)

	return b.shouldFlushLocked()
}

func (b *MessageBuffer) Flush(streamSrv *response.Stream) error {
	b.mut.Lock()
	defer b.mut.Unlock()

	err := streamSrv.BlockScopedDatas(b.buf)
	if err != nil {
		return fmt.Errorf("flushing buffer: %w", err)
	}

	b.lastFlush = time.Now()
	b.DataSize = 0
	b.buf = &pbsubstreamsrpcv4.BlockScopedDatas{Items: []*pbsubstreamsrpcv2.BlockScopedData{}}

	return nil
}
