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

const defaultMessageBufferMaxDataSize = 1024 * 1024 * 10

// messageBufferMaxDataSize is resolved from MESSAGE_BUFFER_MAX_DATA_SIZE once at
// startup instead of on every NewMessageBuffer (i.e. every request). If the env
// var is set but unparseable, messageBufferMaxDataSizeInvalid holds the bad value
// so NewMessageBuffer can still warn about it.
var (
	messageBufferMaxDataSize        = defaultMessageBufferMaxDataSize
	messageBufferMaxDataSizeInvalid string
)

func init() {
	if v := os.Getenv("MESSAGE_BUFFER_MAX_DATA_SIZE"); v != "" {
		if parsed, err := strconv.Atoi(v); err != nil {
			messageBufferMaxDataSizeInvalid = v
		} else {
			messageBufferMaxDataSize = parsed
		}
	}
}

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
	if messageBufferMaxDataSizeInvalid != "" {
		logger.Warn("failed to parse MESSAGE_BUFFER_MAX_DATA_SIZE, using default value", zap.String("value", messageBufferMaxDataSizeInvalid))
	}

	return &MessageBuffer{
		buf:                &pbsubstreamsrpcv4.BlockScopedDatas{Items: []*pbsubstreamsrpcv2.BlockScopedData{}},
		maxBufferedMessage: maxBufferedMessage,
		maxDataSize:        messageBufferMaxDataSize,
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

// AppendAndFlushWhenNeeded appends a message and, when the buffer is full, flushes
// it — append, flush-check and flush all happen under a single lock, so the output
// hot path takes the buffer mutex exactly once per block. It returns the time spent
// flushing (0 if no flush happened).
func (b *MessageBuffer) AppendAndFlushWhenNeeded(msg *pbsubstreamsrpcv2.BlockScopedData, dataSize int, streamSrv *response.Stream) (time.Duration, error) {
	b.mut.Lock()
	defer b.mut.Unlock()

	b.DataSize += dataSize
	b.buf.Items = append(b.buf.Items, msg)

	if !b.shouldFlushLocked() {
		return 0, nil
	}

	start := time.Now()
	if err := b.flushLocked(streamSrv); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (b *MessageBuffer) Flush(streamSrv *response.Stream) error {
	b.mut.Lock()
	defer b.mut.Unlock()

	return b.flushLocked(streamSrv)
}

func (b *MessageBuffer) flushLocked(streamSrv *response.Stream) error {
	err := streamSrv.BlockScopedDatas(b.buf)
	if err != nil {
		return fmt.Errorf("flushing buffer: %w", err)
	}

	b.lastFlush = time.Now()
	b.DataSize = 0
	b.buf = &pbsubstreamsrpcv4.BlockScopedDatas{Items: []*pbsubstreamsrpcv2.BlockScopedData{}}

	return nil
}
