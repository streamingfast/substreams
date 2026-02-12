package execout

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/streamingfast/substreams/orchestrator/response"
	pbsubstreamsrpcv2 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.uber.org/zap"
)

type MessageBuffer struct {
	mut                sync.RWMutex
	buf                *pbsubstreamsrpcv2.BlockScopedDatas
	lastFlush          time.Time
	maxBufferedMessage int
	DataSize           int
	logger             *zap.Logger
	maxDataSize        int
}

func NewMessageBuffer(maxBufferedMessage int, logger *zap.Logger) *MessageBuffer {
	maxDaraSize := 1024 * 1024 * 10
	maxDataSizeString := os.Getenv("MESSAGE_BUFFER_MAX_DATA_SIZE")

	if maxDataSizeString != "" {
		parsed, err := strconv.Atoi(maxDataSizeString)
		if err != nil {
			logger.Warn("failed to parse MESSAGE_BUFFER_MAX_DATA_SIZE, using default value", zap.Error(err))
		} else {
			maxDaraSize = parsed
		}
	}

	return &MessageBuffer{
		buf:                &pbsubstreamsrpcv2.BlockScopedDatas{Items: []*pbsubstreamsrpcv2.BlockScopedData{}},
		maxBufferedMessage: maxBufferedMessage,
		maxDataSize:        maxDaraSize,
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

	if b.DataSize > b.maxDataSize {
		b.logger.Debug("flushing due to large data size", zap.Int("data_size", b.DataSize), zap.Bool("keep", false))
		return true
	}

	if len(b.buf.Items) > b.maxBufferedMessage {
		return true
	}

	return false
}

func (b *MessageBuffer) Flush(streamSrv *response.Stream) error {
	b.mut.Lock()
	defer b.mut.Unlock()

	if b.maxBufferedMessage < 2 {
		for _, msg := range b.buf.Items {
			err := streamSrv.BlockScopedData(msg)
			if err != nil {
				return fmt.Errorf("flushing single block scope data: %w", err)
			}
		}
	} else {
		err := streamSrv.BlockScopedDatas(b.buf)
		if err != nil {
			return fmt.Errorf("flushing buffer: %w", err)
		}
	}

	b.lastFlush = time.Now()
	b.DataSize = 0
	b.buf = &pbsubstreamsrpcv2.BlockScopedDatas{Items: []*pbsubstreamsrpcv2.BlockScopedData{}}

	return nil
}
