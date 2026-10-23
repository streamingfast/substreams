package sink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink/sinktest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

// With an undo buffer, the stream's cursor is ahead of the blocks handed to the handler: the
// completion handler must get the cursor of the last block it handled.
func TestSinker_CompletionGetsLastHandledCursor(t *testing.T) {
	var responses []any
	for n := uint64(10); n <= 14; n++ {
		responses = append(responses, testBlock(t, n, 5)) // reversible: the buffer keeps the last 2
	}
	s := newStreamingTestSinker(t, responses, func(c *SinkerConfig) {
		c.StartBlock = 10
		c.StopBlock = 13 // the request is extended by the buffer size
		c.UndoBufferSize = 2
	})

	h := &recordingHandler{}
	runWithTimeout(t, s, context.Background(), h)

	require.NoError(t, s.Err())
	assert.Equal(t, []uint64{10, 11, 12}, h.handled)
	require.True(t, h.completionCalled)
	assert.Equal(t, uint64(12), h.completionAtBlock, "the stream's cursor (14) is ahead of the handled blocks")
}

// A handler failing while the sinker stops is taken as caused by the stop, a cancelled database call
// does not always wrap context.Canceled, but the completion handler must not run on top of the block
// it did not finish.
func TestSinker_HandlerFailureWhileStopping(t *testing.T) {
	responses := sinktest.StreamMock(testBlock(t, 10, 10), testBlock(t, 11, 11))

	for name, handlerErr := range map[string]error{
		"wrapping context.Canceled":     fmt.Errorf("write aborted: %w", context.Canceled),
		"not wrapping context.Canceled": errors.New("driver: bad connection"),
	} {
		t.Run(name, func(t *testing.T) {
			s := newStreamingTestSinker(t, responses, nil)
			ctx, cancel := context.WithCancel(context.Background())
			h := &recordingHandler{onBlock: func(num uint64) error {
				cancel() // the process is being stopped while this block is handled
				return handlerErr
			}}
			runWithTimeout(t, s, ctx, h)

			require.NoError(t, s.Err())
			assert.False(t, h.completionCalled)
		})
	}
}

// A handler error wrapping io.EOF (a lost database connection, say) is a failure, not the end of the
// stream: the sinker must not exit as if it reached its stop block.
func TestSinker_HandlerEOFIsNotEndOfStream(t *testing.T) {
	s := newStreamingTestSinker(t, sinktest.StreamMock(testBlock(t, 10, 10), testBlock(t, 11, 11)), nil)
	h := &recordingHandler{onBlock: func(num uint64) error {
		return fmt.Errorf("failed to begin db transaction: %w", io.EOF)
	}}
	runWithTimeout(t, s, context.Background(), h)

	require.ErrorIs(t, s.Err(), io.EOF)
	assert.False(t, h.completionCalled)
}

// A stop terminates the sinker before Run is done: a failure of the completion handler that follows
// cannot be its termination error anymore, AwaitRun must report it.
func TestSinker_AwaitRunReportsFailureAfterStop(t *testing.T) {
	s := newStreamingTestSinker(t, sinktest.StreamMock(testBlock(t, 10, 5), testBlock(t, 11, 5)), nil)
	require.NoError(t, s.AwaitRun(context.Background()), "Run not started")

	h := &recordingHandler{
		onBlock: func(num uint64) error {
			s.Shutdown(nil) // the process is being stopped
			return nil
		},
		onCompletion: func() error { return errors.New("final flush failed") },
	}
	runWithTimeout(t, s, context.Background(), h)

	require.NoError(t, s.Err())
	require.True(t, h.completionCalled)
	require.ErrorContains(t, s.AwaitRun(context.Background()), "final flush failed")
}

// testBlock is block num with final block lib, both with their number as id.
func testBlock(t *testing.T, num, lib uint64) *pbsubstreamsrpc.Response {
	return sinktest.BlockScopedData(t, strconv.FormatUint(num, 10), &emptypb.Empty{}, sinktest.FinalBlock(strconv.FormatUint(lib, 10)))
}

var (
	streamTestLoggerOnce sync.Once
	streamTestLogger     *zap.Logger
	streamTestTracer     logging.Tracer
)

func newStreamingTestSinker(t *testing.T, responses []any, configure func(*SinkerConfig)) *Sinker {
	t.Helper()
	clientConfig := sinktest.SetupFakeSubstreamsServer(t, responses...)

	streamTestLoggerOnce.Do(func() { streamTestLogger, streamTestTracer = logging.ApplicationLogger("sink-test", "sink-test") })
	logger, tracer := streamTestLogger, streamTestTracer
	pkg := &pbsubstreams.Package{Modules: &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{
		Name: sinktest.DefaultOutputModuleName, Kind: &pbsubstreams.Module_KindMap_{}, Output: &pbsubstreams.Module_Output{Type: "proto:google.protobuf.Empty"},
	}}}}
	config := &SinkerConfig{
		Pkg:              pkg,
		OutputModule:     pkg.Modules.Modules[0],
		OutputModuleHash: manifest.ModuleHash([]byte("out")),
		ClientConfig:     clientConfig,
		Mode:             SubstreamsModeDevelopment,
		Logger:           logger,
		Tracer:           tracer,
		BackOff:          backoff.WithMaxRetries(backoff.NewConstantBackOff(50*time.Millisecond), 5),
	}
	if configure != nil {
		configure(config)
	}
	s, err := NewFromConfig(config)
	require.NoError(t, err)
	return s
}

type recordingHandler struct {
	mu                sync.Mutex
	handled           []uint64
	completionCalled  bool
	completionAtBlock uint64
	onBlock           func(num uint64) error
	onCompletion      func() error
}

func (h *recordingHandler) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *Cursor) error {
	if h.onBlock != nil {
		if err := h.onBlock(data.Clock.Number); err != nil {
			return err
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handled = append(h.handled, data.Clock.Number)
	return nil
}

func (h *recordingHandler) HandleBlockUndoSignal(ctx context.Context, undo *pbsubstreamsrpc.BlockUndoSignal, cursor *Cursor) error {
	return nil
}

func (h *recordingHandler) HandleBlockRangeCompletion(ctx context.Context, cursor *Cursor) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.completionCalled = true
	h.completionAtBlock = cursor.Block().Num()
	if h.onCompletion != nil {
		return h.onCompletion()
	}
	return nil
}

func runWithTimeout(t *testing.T, s *Sinker, ctx context.Context, h SinkerHandler) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		s.Run(ctx, NewBlankCursor(), h)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		require.Fail(t, "sinker did not return")
	}
}
