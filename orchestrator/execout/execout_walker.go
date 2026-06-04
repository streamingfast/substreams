package execout

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging/zapx"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/loop"
	"github.com/streamingfast/substreams/orchestrator/response"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

type Walker struct {
	ctx context.Context
	*block.Range
	fileWalker       *execout.FileWalker
	streamOut        *response.Stream
	module           *pbsubstreams.Module
	logger           *zap.Logger
	working          bool
	workingAt        time.Time // Timestamp when working was set to true
	noopMode         bool
	buffer           *MessageBuffer
	supportBuffering bool
}

func NewWalker(
	ctx context.Context,
	module *pbsubstreams.Module,
	fileWalker *execout.FileWalker,
	walkRange *block.Range,
	stream *response.Stream,
	noopMode bool,
	bufferSize int,
	supportBuffering bool,
) *Walker {
	logger := reqctx.Logger(ctx).Named("execout_walker")

	var buffer *MessageBuffer
	if supportBuffering {
		buffer = NewMessageBuffer(bufferSize, logger)
	}

	return &Walker{
		ctx:              ctx,
		module:           module,
		fileWalker:       fileWalker,
		Range:            walkRange,
		streamOut:        stream,
		noopMode:         noopMode,
		logger:           logger,
		buffer:           buffer,
		supportBuffering: supportBuffering,
	}
}

func (r *Walker) IsNoopMode() bool {
	return r.noopMode
}

func (r *Walker) MarkNotWorking() {
	r.working = false
	r.workingAt = time.Time{}
}

func (r *Walker) MarkWorking() {
	r.working = true
	r.workingAt = time.Now()
}

func (r *Walker) IsWorking() bool {
	return r.working
}

// WorkingDuration returns how long the walker has been in working state.
// Returns 0 if not currently working.
func (r *Walker) WorkingDuration() time.Duration {
	if !r.working || r.workingAt.IsZero() {
		return 0
	}
	return time.Since(r.workingAt)
}

func (r *Walker) CmdDownloadCurrentSegment(waitBefore time.Duration) loop.Cmd {
	first, current, last := r.fileWalker.Progress()

	r.logger.Debug("starting download command",
		zap.Int("segment", current),
		zap.Int("first_segment", first),
		zap.Int("last_segment", last),
		zap.Duration("wait_before", waitBefore),
		zap.Bool("noop_mode", r.noopMode),
	)

	return func() (msg loop.Msg) {
		downloadStartTime := time.Now()
		r.logger.Debug("download goroutine started",
			zap.Int("segment", current),
			zap.Bool("noop_mode", r.noopMode),
		)

		// Log what message we're returning to help debug message delivery
		defer func() {
			r.logger.Debug("download goroutine returning message",
				zap.Int("segment", current),
				zap.String("msg_type", fmt.Sprintf("%T", msg)),
				zap.Duration("elapsed", time.Since(downloadStartTime)),
			)
		}()

		// Recover from any panic to prevent the event loop from getting stuck
		defer func() {
			if rec := recover(); rec != nil {
				r.logger.Error("panic during download, returning quit message",
					zap.Int("segment", current),
					zap.Any("panic", rec),
					zap.Duration("elapsed", time.Since(downloadStartTime)),
				)
				msg = loop.NewQuitMsg(fmt.Errorf("panic during execout download for segment %d: %v", current, rec))
			}
		}()

		time.Sleep(waitBefore)

		// Check if context is cancelled before attempting download
		if r.ctx.Err() != nil {
			r.logger.Info("context cancelled before download",
				zap.Int("segment", current),
				zap.Error(r.ctx.Err()),
			)
			return loop.NewQuitMsg(r.ctx.Err())
		}

		s := time.Now()
		file, err := r.fileWalker.FileReader(r.ctx)
		fileReaderCreationDuration := time.Since(s)

		if errors.Is(err, dstore.ErrNotFound) {
			return MsgFileNotPresent{NextWait: computeNewWait(waitBefore, r.fileWalker.IsLocal)}
		}
		if err != nil {
			if isTransientStorageError(err) {
				return MsgFileReadTransientError{NextWait: computeNewWait(waitBefore, r.fileWalker.IsLocal), Error: err}
			}

			// Fatal error - fail the request
			r.logger.Error("fatal error loading file",
				zap.Int("segment", current),
				zap.Error(err),
			)
			// file might be nil if error occurred, so we can't use file.ModuleName()/Filename()
			return loop.NewQuitMsg(fmt.Errorf("loading cache for segment %d: %w", current, err))
		}
		if file == nil {
			r.logger.Error("file reader is nil without error",
				zap.Int("segment", current),
			)
			return loop.NewQuitMsg(fmt.Errorf("file reader is nil for segment %d", current))
		}

		r.logger.Debug("file found, sending items",
			zap.Int("segment", current),
			zap.String("filename", file.Filename()),
		)

		if err := r.sendItems(file); err != nil && err != io.EOF {
			if errors.Is(err, context.Canceled) {
				return loop.NewQuitMsg(err)
			}
			r.logger.Error("error sending items",
				zap.Int("segment", current),
				zap.Error(err),
			)
			return loop.NewQuitMsg(err)
		}

		r.logger.Debug("file download completed, returning MsgFileDownloaded",
			zap.Int("segment", current),
			zap.Bool("noop_mode", r.noopMode),
			zapx.HumanDuration("download_elapsed", time.Since(downloadStartTime)),
			zapx.HumanDuration("file_reader_creation_duration", fileReaderCreationDuration),
			zap.Bool("keep", false),
		)
		return MsgFileDownloaded{}
	}
}

// isTransientStorageError checks if an error is a transient network/HTTP2 error
// that should be retried rather than treated as fatal
func isTransientStorageError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// HTTP/2 stream errors from GCS (e.g., "stream error: stream ID 971; INTERNAL_ERROR")
	if strings.Contains(errStr, "stream error") && strings.Contains(errStr, "INTERNAL_ERROR") {
		return true
	}

	// Other common transient errors
	if strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "temporary failure") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") {
		return true
	}

	return false
}

func computeNewWait(previousWait time.Duration, storeIsLocal bool) time.Duration {
	if storeIsLocal {
		return 50 * time.Millisecond
	}
	if previousWait == 0 {
		return 250 * time.Millisecond
	}
	newWait := previousWait + 50*time.Millisecond
	if newWait > 2*time.Second {
		return 2 * time.Second
	}
	return newWait
}

func (r *Walker) sendItems(reader execout.FileReader) error {
	defer reader.Close()
	itemCount := 0
	flushingDuration := time.Duration(0)
	iterationStartTime := time.Now()
	firstItemTime := time.Duration(0)
	cumulativePayloadSize := 0
	for item, err := range reader.Iter() {
		if err != nil {
			return fmt.Errorf("iterating on execout reader: %w", err)
		}

		if itemCount == 0 {
			firstItemTime = time.Since(iterationStartTime)
		}

		cumulativePayloadSize += len(item.Payload)
		if item.BlockNum < r.StartBlock {
			continue
		}
		if item.BlockNum >= r.ExclusiveEndBlock {
			r.logger.Debug("reached exclusive end block in sendItems",
				zap.Uint64("item_block_num", item.BlockNum),
				zap.Uint64("exclusive_end_block", r.ExclusiveEndBlock),
				zap.Int("items_sent", itemCount),
				zap.Bool("keep", false),
			)
			if r.supportBuffering {
				if err := r.buffer.Flush(r.streamOut); err != nil {
					return fmt.Errorf("flushing buffer on end block: %w", err)
				}
			}
			return nil
		}

		blockScopedData, err := toBlockScopedData(r.module, item)
		if err != nil {
			return fmt.Errorf("converting to block scoped data: %w", err)
		}

		if r.supportBuffering {
			d, err := r.buffer.AppendAndFlushWhenNeeded(blockScopedData, len(item.Payload), r.streamOut)
			if err != nil {
				return fmt.Errorf("flushing buffer on 'should flush': %w", err)
			}
			flushingDuration += d
		} else {
			s := time.Now()
			err := r.streamOut.BlockScopedData(blockScopedData)
			if err != nil {
				return fmt.Errorf("sending block scoped data: %w", err)
			}
			flushingDuration += time.Since(s)
		}
		itemCount++

		if r.noopMode {
			// only a single message per bundle is sent in noop mode. The sender function will take care of removing the content
			r.logger.Debug("noop mode, returning after single item",
				zap.Uint64("block_num", blockScopedData.Clock.Number),
				zap.Int("items_sent", itemCount),
				zap.Bool("keep", false),
			)
			return nil
		}

	}

	s := time.Now()
	if r.supportBuffering {
		if err := r.buffer.Flush(r.streamOut); err != nil {
			return fmt.Errorf("flushing buffer on end of iteration: %w", err)
		}
	}
	flushingDuration += time.Since(s)

	totalDuration := time.Since(iterationStartTime)
	r.logger.Debug("finished iterating all items",
		zap.Int("items_sent", itemCount),
		zapx.HumanDuration("flushing_duration", flushingDuration),
		zapx.HumanDuration("first_item_duration", firstItemTime),
		zap.Duration("first_item_duration_raw", firstItemTime),
		zapx.HumanDuration("total_duration", totalDuration),
		zap.String("payload size", humanize.Bytes(uint64(cumulativePayloadSize))),
		zap.Bool("keep", false),
	)

	return nil
}

func (r *Walker) Progress() (first, current, last int) {
	return r.fileWalker.Progress()
}

func (r *Walker) NextSegment() {
	r.fileWalker.Next()
}

func (r *Walker) IsCompleted() bool {
	return r.fileWalker.IsDone()
}

func toBlockScopedData(module *pbsubstreams.Module, cacheItem *pboutput.Item) (*pbsubstreamsrpc.BlockScopedData, error) {
	clock := toClock(cacheItem)
	blockRef := bstream.NewBlockRef(clock.Id, clock.Number)
	cursor := bstream.Cursor{
		Step:      bstream.StepNewIrreversible,
		Block:     blockRef,
		LIB:       blockRef,
		HeadBlock: blockRef,
	}
	out := &pbsubstreamsrpc.BlockScopedData{
		Cursor:           cursor.ToOpaque(),
		Clock:            clock,
		FinalBlockHeight: blockRef.Num(),
	}

	m, err := toModuleOutput(module, cacheItem)
	if err != nil {
		return nil, fmt.Errorf("module output: %w", err)
	}
	out.Output = m

	return out, nil
}

func toModuleOutput(module *pbsubstreams.Module, cacheItem *pboutput.Item) (*pbsubstreamsrpc.MapModuleOutput, error) {
	outputType := strings.TrimPrefix(module.Output.Type, "proto:")

	return &pbsubstreamsrpc.MapModuleOutput{
		Name:      module.Name,
		MapOutput: &anypb.Any{TypeUrl: "type.googleapis.com/" + outputType, Value: cacheItem.Payload},
	}, nil
}

func toClock(item *pboutput.Item) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{
		Id:        item.BlockId,
		Number:    item.BlockNum,
		Timestamp: item.Timestamp,
	}
}
