package sink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// scriptedStream answers each Blocks call with a session, then its responses, then ends the stream.
type scriptedStream struct {
	pbsubstreamsrpc.UnimplementedStreamServer
	responses []*pbsubstreamsrpc.Response
}

func (s *scriptedStream) Blocks(req *pbsubstreamsrpc.Request, stream pbsubstreamsrpc.Stream_BlocksServer) error {
	if err := stream.Send(&pbsubstreamsrpc.Response{Message: &pbsubstreamsrpc.Response_Session{Session: &pbsubstreamsrpc.SessionInit{
		TraceId: "test", ResolvedStartBlock: uint64(req.StartBlockNum), LinearHandoffBlock: uint64(req.StartBlockNum), MaxParallelWorkers: 1,
	}}}); err != nil {
		return err
	}
	for _, r := range s.responses {
		if err := stream.Send(r); err != nil {
			return err
		}
	}
	return nil
}

func testCursor(num, lib uint64) string {
	blk := bstream.NewBlockRef(fmt.Sprintf("%d", num), num)
	return (&bstream.Cursor{Step: bstream.StepNew, Block: blk, LIB: bstream.NewBlockRef(fmt.Sprintf("%d", lib), lib), HeadBlock: blk}).ToOpaque()
}

func testBlock(t *testing.T, num, lib uint64) *pbsubstreamsrpc.Response {
	out, err := anypb.New(&emptypb.Empty{})
	require.NoError(t, err)
	return &pbsubstreamsrpc.Response{Message: &pbsubstreamsrpc.Response_BlockScopedData{BlockScopedData: &pbsubstreamsrpc.BlockScopedData{
		Output:           &pbsubstreamsrpc.MapModuleOutput{Name: "out", MapOutput: out},
		Clock:            &pbsubstreams.Clock{Id: fmt.Sprintf("%d", num), Number: num},
		Cursor:           testCursor(num, lib),
		FinalBlockHeight: lib,
	}}}
}

var (
	streamTestLoggerOnce sync.Once
	streamTestLogger     *zap.Logger
	streamTestTracer     logging.Tracer
)

func newStreamingTestSinker(t *testing.T, responses []*pbsubstreamsrpc.Response, configure func(*SinkerConfig)) *Sinker {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	pbsubstreamsrpc.RegisterStreamServer(server, &scriptedStream{responses: responses})
	go server.Serve(listener)
	t.Cleanup(server.Stop)

	streamTestLoggerOnce.Do(func() { streamTestLogger, streamTestTracer = logging.ApplicationLogger("sink-test", "sink-test") })
	logger, tracer := streamTestLogger, streamTestTracer
	pkg := &pbsubstreams.Package{Modules: &pbsubstreams.Modules{Modules: []*pbsubstreams.Module{{
		Name: "out", Kind: &pbsubstreams.Module_KindMap_{}, Output: &pbsubstreams.Module_Output{Type: "proto:google.protobuf.Empty"},
	}}}}
	config := &SinkerConfig{
		Pkg:              pkg,
		OutputModule:     pkg.Modules.Modules[0],
		OutputModuleHash: manifest.ModuleHash([]byte("out")),
		ClientConfig: client.NewSubstreamsClientConfig(client.SubstreamsClientConfigOptions{
			Endpoint: listener.Addr().String(), AuthType: client.None, PlainText: true, Agent: "sink-test",
		}),
		Mode:    SubstreamsModeDevelopment,
		Logger:  logger,
		Tracer:  tracer,
		BackOff: backoff.WithMaxRetries(backoff.NewConstantBackOff(50*time.Millisecond), 5),
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
		t.Fatal("sinker did not return")
	}
}

// With an undo buffer, the stream's cursor is ahead of the blocks handed to the handler: the
// completion handler must get the cursor of the last block it handled.
func TestSinker_CompletionGetsLastHandledCursor(t *testing.T) {
	var responses []*pbsubstreamsrpc.Response
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
	assert.False(t, s.LastBlockMessageAt().IsZero(), "blocks kept by the buffer count as received")
}

// A handler failing while the sinker stops must be reported, and the completion handler must not run
// on top of the block it did not finish.
func TestSinker_HandlerFailureWhileStopping(t *testing.T) {
	responses := []*pbsubstreamsrpc.Response{testBlock(t, 10, 10), testBlock(t, 11, 11)}

	t.Run("failure is reported", func(t *testing.T) {
		s := newStreamingTestSinker(t, responses, nil)
		ctx, cancel := context.WithCancel(context.Background())
		h := &recordingHandler{onBlock: func(num uint64) error {
			cancel() // the process is being stopped while this block is handled
			return errors.New("write failed")
		}}
		runWithTimeout(t, s, ctx, h)

		require.ErrorContains(t, s.Err(), "write failed")
		assert.False(t, h.completionCalled)
	})

	t.Run("an error caused by the stop itself is not, and completion is skipped too", func(t *testing.T) {
		s := newStreamingTestSinker(t, responses, nil)
		ctx, cancel := context.WithCancel(context.Background())
		h := &recordingHandler{onBlock: func(num uint64) error {
			cancel()
			return fmt.Errorf("write aborted: %w", context.Canceled)
		}}
		runWithTimeout(t, s, ctx, h)

		require.NoError(t, s.Err())
		assert.False(t, h.completionCalled)
	})
}

// A handler error wrapping io.EOF (a lost database connection, say) is a failure, not the end of the
// stream: the sinker must not exit as if it reached its stop block.
func TestSinker_HandlerEOFIsNotEndOfStream(t *testing.T) {
	s := newStreamingTestSinker(t, []*pbsubstreamsrpc.Response{testBlock(t, 10, 10), testBlock(t, 11, 11)}, nil)
	h := &recordingHandler{onBlock: func(num uint64) error {
		return fmt.Errorf("failed to begin db transaction: %w", io.EOF)
	}}
	runWithTimeout(t, s, context.Background(), h)

	require.ErrorIs(t, s.Err(), io.EOF)
	assert.False(t, h.completionCalled)
}
