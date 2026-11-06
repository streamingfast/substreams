package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

func opaqueCursor(num uint64) string {
	ref := bstream.NewBlockRef("aa", num)
	return (&bstream.Cursor{Step: bstream.StepNew, Block: ref, HeadBlock: ref, LIB: bstream.NewBlockRef("00", 1)}).ToOpaque()
}

func jsonNumber(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func protoClock(num uint64) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{Number: num, Id: "id" + jsonNumber(num)}
}

// fakePublisher fails its first fail publishes. fail < 0 fails every publish.
type fakePublisher struct {
	mu    sync.Mutex
	fail  int
	calls int
	msgs  []Message
}

func (f *fakePublisher) Publish(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.fail < 0 || (f.fail > 0 && f.calls <= f.fail) {
		return &DeliveryError{Attempts: 2, Err: errors.New("unavailable")}
	}
	copied := msg
	copied.Data = append([]byte(nil), msg.Data...)
	if msg.Attributes != nil {
		copied.Attributes = make(map[string]string, len(msg.Attributes))
		for k, v := range msg.Attributes {
			copied.Attributes[k] = v
		}
	}
	f.msgs = append(f.msgs, copied)
	return nil
}

func (f *fakePublisher) messages() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Message(nil), f.msgs...)
}

func (f *fakePublisher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newTestSink(t *testing.T, pub Publisher, onFailure OnFailure) *Sink {
	t.Helper()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.cursor")
	terminationLog := filepath.Join(dir, "termination-log")
	require.NoError(t, os.WriteFile(terminationLog, nil, 0o644))
	return &Sink{
		projectID:      "my-gcp-project",
		topicID:        "events",
		moduleName:     "map_events",
		orderingKey:    "map_events",
		stateFile:      stateFile,
		pendingFile:    pendingFilePath(stateFile),
		onFailure:      onFailure,
		terminationLog: terminationLog,
		fingerprint:    configFingerprint("my-gcp-project", "events", false),
		maxInterval:    time.Millisecond,
		publisher:      pub,
		logger:         zlogTest,
	}
}

func (s *Sink) enableUndo() {
	s.publishUndo = true
	s.fingerprint = configFingerprint(s.projectID, s.topicID, true)
}

func newPending(num uint64) *pendingDelivery {
	return &pendingDelivery{
		Batched:        true,
		Cursor:         opaqueCursor(num),
		BlockNumber:    num,
		Payload:        json.RawMessage(`{"manifest":{"moduleName":"map_events"},"blocks":[{"clock":{"number":` + jsonNumber(num) + `}}]}`),
		FirstAttemptAt: time.Now().Add(-time.Hour),
	}
}

func readStateCursor(t *testing.T, s *Sink) string {
	t.Helper()
	c, err := sink.ReadCursor(s.stateFile)
	require.NoError(t, err)
	if c == nil {
		return ""
	}
	return c.String()
}

func TestPendingFile_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.cursor.pending")

	got, err := readPending(path)
	require.NoError(t, err)
	assert.Nil(t, got)

	p := newPending(10)
	p.Fingerprint = "fp"
	require.NoError(t, writePending(path, p))

	got, err = readPending(path)
	require.NoError(t, err)
	assert.Equal(t, p.Cursor, got.Cursor)
	assert.Equal(t, uint64(10), got.BlockNumber)
	assert.JSONEq(t, string(p.Payload), string(got.Payload))
	assert.Equal(t, "fp", got.Fingerprint)
	assert.WithinDuration(t, p.FirstAttemptAt, got.FirstAttemptAt, time.Second)

	require.NoError(t, removePending(path))
	require.NoError(t, removePending(path), "removing twice is fine")
	assert.Empty(t, pendingFilePath(""))
}

func TestSendBlock_PublishesOneBlockInTheBatchJSON(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	clock := protoClock(10)
	data := json.RawMessage(`{"n":1}`)

	require.NoError(t, s.addToBatch(context.Background(), "map_events", "type.googleapis.com/sf.test.v1.Out", clock, data, sink.MustNewCursor(opaqueCursor(10)), false, time.Now()))

	msgs := pub.messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "map_events", msgs[0].OrderingKey)
	assert.Equal(t, TypeBatch, msgs[0].Attributes[AttributeType])

	var got BatchPayload
	require.NoError(t, json.Unmarshal(msgs[0].Data, &got))
	assert.Equal(t, "map_events", got.Manifest.ModuleName)
	assert.Equal(t, "sf.test.v1.Out", got.Manifest.Type)
	require.Len(t, got.Blocks, 1)
	assert.Equal(t, uint64(10), got.Blocks[0].Clock.Number)
	assert.JSONEq(t, string(data), string(got.Blocks[0].Data))
	assert.Equal(t, opaqueCursor(10), readStateCursor(t, s))
	assert.NoFileExists(t, s.pendingFile)
}

func TestSend_ExitModeKeepsPendingAndWritesTerminationReason(t *testing.T) {
	pub := &fakePublisher{fail: -1}
	s := newTestSink(t, pub, OnFailureExit)
	p := newPending(10)
	err := s.send(context.Background(), p)

	var failed *DeliveryFailedError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, uint64(10), failed.Delivery.BlockNumber)
	assert.Equal(t, "my-gcp-project", failed.Delivery.Project)
	assert.Equal(t, "events", failed.Delivery.Topic)
	assert.Equal(t, 2, failed.Delivery.Attempts)
	assert.Equal(t, p.FirstAttemptAt, failed.FirstAttemptAt)
	assert.Empty(t, readStateCursor(t, s), "cursor must not move past an unpublished block")

	kept, err := readPending(s.pendingFile)
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, uint64(10), kept.BlockNumber)

	msg, err := os.ReadFile(s.terminationLog)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(msg, &decoded))
	assert.Equal(t, "pubsub_delivery_failed", decoded["reason"])
	assert.Equal(t, "my-gcp-project", decoded["project"])
	assert.Equal(t, "events", decoded["topic"])
	assert.Equal(t, float64(10), decoded["block"])
	assert.Equal(t, float64(2), decoded["attempts"])
	assert.Equal(t, p.FirstAttemptAt.UTC().Format(time.RFC3339), decoded["first_attempt_at"])
	assert.NotEmpty(t, decoded["error"])
}

func TestSend_SkipModeDropsBlock(t *testing.T) {
	pub := &fakePublisher{fail: -1}
	s := newTestSink(t, pub, OnFailureSkip)
	require.NoError(t, s.send(context.Background(), newPending(10)))
	assert.NoFileExists(t, s.pendingFile)
	assert.Empty(t, readStateCursor(t, s))
	assert.Empty(t, pub.messages())

	msg, err := os.ReadFile(s.terminationLog)
	require.NoError(t, err)
	assert.Empty(t, msg, "skip mode is not an exit, nothing to report")
}

func TestDeliver_CountsOnlyASuccessfulPublish(t *testing.T) {
	publishes := PublishesCounter.Get()
	sent := PublishedBytes.Get()

	failing := &fakePublisher{fail: -1}
	s := newTestSink(t, failing, OnFailureSkip)
	pending := newPending(10)
	require.NoError(t, s.send(context.Background(), pending))
	assert.Equal(t, publishes, PublishesCounter.Get())
	assert.Equal(t, sent, PublishedBytes.Get())

	s.publisher = &fakePublisher{}
	require.NoError(t, s.send(context.Background(), pending))
	assert.Equal(t, publishes+1, PublishesCounter.Get())
	assert.Equal(t, sent+float64(len(pending.Payload)), PublishedBytes.Get())

	// Two failed undo rounds, then the publish. Only the publish is counted.
	s.enableUndo()
	undoPublisher := &fakePublisher{fail: 2}
	s.publisher = undoPublisher
	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 9, Id: "aa"}}
	require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(9))))
	assert.Equal(t, 3, undoPublisher.callCount())
	assert.Equal(t, publishes+2, PublishesCounter.Get())
	assert.Equal(t, sent+float64(len(pending.Payload)+len(undoPublisher.messages()[0].Data)), PublishedBytes.Get())
}

func TestSend_ShutdownIsNotADeliveryFailure(t *testing.T) {
	for _, onFailure := range []OnFailure{OnFailureExit, OnFailureSkip} {
		t.Run(string(onFailure), func(t *testing.T) {
			pub := &fakePublisher{fail: -1}
			s := newTestSink(t, pub, onFailure)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := s.send(ctx, newPending(10))

			require.ErrorIs(t, err, context.Canceled)
			var failed *DeliveryFailedError
			assert.False(t, errors.As(err, &failed))
			assert.NoFileExists(t, s.pendingFile)
		})
	}
}

func TestRecoverPending_DeliversBeforeStreaming(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	p := newPending(10)
	p.Fingerprint = s.fingerprint
	require.NoError(t, writePending(s.pendingFile, p))

	got, err := s.recoverPending(context.Background(), sink.MustNewCursor(opaqueCursor(9)))
	require.NoError(t, err)
	assert.Equal(t, opaqueCursor(10), got.String())
	assert.Equal(t, 1, pub.callCount())
	assert.Equal(t, TypeBatch, pub.messages()[0].Attributes[AttributeType])
	assert.Equal(t, "map_events", pub.messages()[0].OrderingKey)
	assert.NoFileExists(t, s.pendingFile)
}

func TestRecoverPending_SkipModeDropsAndContinues(t *testing.T) {
	pub := &fakePublisher{fail: -1}
	s := newTestSink(t, pub, OnFailureSkip)
	p := newPending(10)
	p.Fingerprint = s.fingerprint
	require.NoError(t, writePending(s.pendingFile, p))

	start := sink.MustNewCursor(opaqueCursor(9))
	got, err := s.recoverPending(context.Background(), start)
	require.NoError(t, err)
	assert.Same(t, start, got)
	assert.NoFileExists(t, s.pendingFile)
}

func TestRecoverPending_ConfigChangeResetsFirstAttempt(t *testing.T) {
	pub := &fakePublisher{fail: -1}
	s := newTestSink(t, pub, OnFailureExit)
	p := newPending(10)
	p.Fingerprint = "written-under-the-old-topic"
	require.NoError(t, writePending(s.pendingFile, p))

	_, err := s.recoverPending(context.Background(), nil)
	var failed *DeliveryFailedError
	require.ErrorAs(t, err, &failed)
	assert.WithinDuration(t, time.Now(), failed.FirstAttemptAt, 5*time.Second)

	kept, err := readPending(s.pendingFile)
	require.NoError(t, err)
	assert.Equal(t, s.fingerprint, kept.Fingerprint)
}

func TestConfigFingerprint_CoversTopicAndUndo(t *testing.T) {
	fp := configFingerprint("my-gcp-project", "events", false)
	assert.Equal(t, fp, configFingerprint("my-gcp-project", "events", false))
	assert.NotEqual(t, fp, configFingerprint("other", "events", false))
	assert.NotEqual(t, fp, configFingerprint("my-gcp-project", "other", false))
	assert.NotEqual(t, fp, configFingerprint("my-gcp-project", "events", true))
}

func TestUndo_DisabledOnlyMovesCursorBack(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	require.NoError(t, sink.WriteCursor(s.stateFile, sink.MustNewCursor(opaqueCursor(12))))

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 10, Id: "aa"}}
	require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(10))))

	assert.Empty(t, pub.messages())
	assert.Equal(t, opaqueCursor(10), readStateCursor(t, s))
	assert.NoFileExists(t, s.pendingFile)
}

func TestUndo_PublishesWebhookJSONWithUndoAttribute(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.enableUndo()

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 10, Id: "aa"}}
	require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(10))))

	msgs := pub.messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, TypeUndo, msgs[0].Attributes[AttributeType])
	assert.Equal(t, "map_events", msgs[0].OrderingKey)
	want, err := NewUndoPayload("map_events", &pbsubstreams.BlockRef{Number: 10, Id: "aa"}).ToJSON()
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(msgs[0].Data))
	assert.Equal(t, opaqueCursor(10), readStateCursor(t, s))
}

func TestUndo_ExitModeKeepsPending(t *testing.T) {
	pub := &fakePublisher{fail: -1}
	s := newTestSink(t, pub, OnFailureExit)
	s.enableUndo()
	require.NoError(t, sink.WriteCursor(s.stateFile, sink.MustNewCursor(opaqueCursor(12))))

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 10, Id: "aa"}}
	err := s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(10)))
	var failed *DeliveryFailedError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, DeliveryKindUndo, failed.Kind)
	assert.Equal(t, opaqueCursor(12), readStateCursor(t, s), "cursor stays until the subscriber knows about the reorg")

	kept, err := readPending(s.pendingFile)
	require.NoError(t, err)
	assert.Equal(t, DeliveryKindUndo, kept.Kind)
}

func TestUndo_SkipModeRetriesUntilPublished(t *testing.T) {
	pub := &fakePublisher{fail: 3}
	s := newTestSink(t, pub, OnFailureSkip)
	s.enableUndo()

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 9, Id: "aa"}}
	require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(9))))
	assert.Equal(t, opaqueCursor(9), readStateCursor(t, s))

	require.NoError(t, s.addToBatch(context.Background(), "map_events", "t", protoClock(10), json.RawMessage(`{}`), sink.MustNewCursor(opaqueCursor(10)), true, time.Now()))

	msgs := pub.messages()
	require.Len(t, msgs, 2, "the replacement block is published only after the undo notification")
	assert.Equal(t, TypeUndo, msgs[0].Attributes[AttributeType])
	assert.Equal(t, TypeBatch, msgs[1].Attributes[AttributeType])
	assert.Equal(t, "map_events", msgs[0].OrderingKey)
	assert.Equal(t, "map_events", msgs[1].OrderingKey)
}

func TestRecoverPending_UndoDisabledDropsNotification(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	p := newPending(10)
	p.Kind = DeliveryKindUndo
	p.Fingerprint = s.fingerprint
	require.NoError(t, writePending(s.pendingFile, p))

	got, err := s.recoverPending(context.Background(), sink.MustNewCursor(opaqueCursor(12)))
	require.NoError(t, err)
	assert.Equal(t, opaqueCursor(10), got.String())
	assert.Empty(t, pub.messages())
	assert.NoFileExists(t, s.pendingFile)
}

func addBlock(t *testing.T, s *Sink, num uint64, live bool, now time.Time) {
	t.Helper()
	data := json.RawMessage(`{"n":` + jsonNumber(num) + `}`)
	require.NoError(t, s.addToBatch(context.Background(), "map_events", "type.googleapis.com/sf.test.v1.Out", protoClock(num), data, sink.MustNewCursor(opaqueCursor(num)), live, now))
}

func batchBlockNumbers(t *testing.T, body []byte) []uint64 {
	t.Helper()
	var payload BatchPayload
	require.NoError(t, json.Unmarshal(body, &payload))
	var numbers []uint64
	for _, b := range payload.Blocks {
		numbers = append(numbers, b.Clock.Number)
	}
	return numbers
}

func TestBatch_FlushesWebhookShapeWhenFull(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.batchMaxBlocks, s.batchMaxWait = 2, time.Minute

	now := time.Now()
	addBlock(t, s, 10, false, now)
	assert.Empty(t, pub.messages())
	addBlock(t, s, 11, false, now)

	msgs := pub.messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, TypeBatch, msgs[0].Attributes[AttributeType])
	assert.Equal(t, "map_events", msgs[0].OrderingKey)
	assert.Equal(t, []uint64{10, 11}, batchBlockNumbers(t, msgs[0].Data))
	assert.Equal(t, opaqueCursor(11), readStateCursor(t, s))
	assert.Nil(t, s.batch)

	var got BatchPayload
	require.NoError(t, json.Unmarshal(msgs[0].Data, &got))
	assert.Equal(t, "map_events", got.Manifest.ModuleName)
	assert.Equal(t, "sf.test.v1.Out", got.Manifest.Type)
}

func addBlockWithData(t *testing.T, s *Sink, num uint64, data string) {
	t.Helper()
	require.NoError(t, s.addToBatch(context.Background(), "map_events", "type.googleapis.com/sf.test.v1.Out", protoClock(num), json.RawMessage(data), sink.MustNewCursor(opaqueCursor(num)), false, time.Now()))
}

func TestBatch_MaxBytesSendsOversizedBlockAlone(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.batchMaxBlocks, s.batchMaxWait, s.batchMaxBytes = 100, time.Minute, 200

	addBlockWithData(t, s, 10, `{}`)
	addBlockWithData(t, s, 11, `{"v":"`+strings.Repeat("x", 300)+`"}`)
	addBlockWithData(t, s, 12, `{}`)

	msgs := pub.messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, []uint64{10}, batchBlockNumbers(t, msgs[0].Data))
	assert.LessOrEqual(t, len(msgs[0].Data), s.batchMaxBytes)
	assert.Equal(t, []uint64{11}, batchBlockNumbers(t, msgs[1].Data))
	assert.Greater(t, len(msgs[1].Data), s.batchMaxBytes, "a block larger than the limit is published alone")

	require.NotNil(t, s.batch)
	rest, err := s.batch.payload.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, []uint64{12}, batchBlockNumbers(t, rest))
	assert.Equal(t, len(rest), s.batch.size)
}

func TestBatch_FlushesWhenLiveOrWaited(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.batchMaxBlocks, s.batchMaxWait = 100, time.Second

	now := time.Now()
	addBlock(t, s, 10, true, now)
	require.Len(t, pub.messages(), 1, "a live block goes out on its own")
	assert.Equal(t, []uint64{10}, batchBlockNumbers(t, pub.messages()[0].Data))

	addBlock(t, s, 11, false, now)
	addBlock(t, s, 12, false, now)
	addBlock(t, s, 13, true, now)
	msgs := pub.messages()
	require.Len(t, msgs, 3)
	assert.Equal(t, []uint64{11, 12}, batchBlockNumbers(t, msgs[1].Data))
	assert.Equal(t, []uint64{13}, batchBlockNumbers(t, msgs[2].Data), "a live block is not appended to the historical batch")

	addBlock(t, s, 14, false, now)
	addBlock(t, s, 15, false, now.Add(2*time.Second))
	msgs = pub.messages()
	require.Len(t, msgs, 4)
	assert.Equal(t, []uint64{14, 15}, batchBlockNumbers(t, msgs[3].Data))
}

func TestHandleBlockScopedData_NilOutputDoesNotPanic(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.batchMaxBlocks, s.batchMaxWait = 100, time.Second
	addBlock(t, s, 10, false, time.Now().Add(-2*time.Second))

	// --noop-mode replaces the output with an empty MapModuleOutput.
	noop := &pbsubstreamsrpc.BlockScopedData{Clock: protoClock(11), Output: &pbsubstreamsrpc.MapModuleOutput{}}
	require.NoError(t, s.handleBlockScopedData(context.Background(), noop, nil, sink.MustNewCursor(opaqueCursor(11))))
	require.NoError(t, s.handleBlockScopedData(context.Background(), &pbsubstreamsrpc.BlockScopedData{Clock: protoClock(12)}, nil, nil))

	msgs := pub.messages()
	require.Len(t, msgs, 1, "a block with no output still flushes a batch that has waited")
	assert.Equal(t, []uint64{10}, batchBlockNumbers(t, msgs[0].Data))
}

func TestBatch_SparseModuleFlushesOnEmptyBlock(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.batchMaxBlocks, s.batchMaxWait = 100, time.Second

	addBlock(t, s, 10, false, time.Now().Add(-2*time.Second))
	empty := &pbsubstreamsrpc.BlockScopedData{Clock: protoClock(11), Output: &pbsubstreamsrpc.MapModuleOutput{Name: "map_events", MapOutput: &anypb.Any{}}}
	require.NoError(t, s.handleBlockScopedData(context.Background(), empty, nil, sink.MustNewCursor(opaqueCursor(11))))

	msgs := pub.messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, []uint64{10}, batchBlockNumbers(t, msgs[0].Data))
	assert.Equal(t, opaqueCursor(10), readStateCursor(t, s))
}

func TestBatch_ValidPartPublishedBeforeUndo(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.enableUndo()
	s.batchMaxBlocks, s.batchMaxWait = 100, time.Minute
	for _, num := range []uint64{8, 9, 10, 11} {
		addBlock(t, s, num, false, time.Now())
	}

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 9, Id: "aa"}}
	require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(9))))

	msgs := pub.messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, TypeBatch, msgs[0].Attributes[AttributeType])
	assert.Equal(t, []uint64{8, 9}, batchBlockNumbers(t, msgs[0].Data))
	assert.Equal(t, TypeUndo, msgs[1].Attributes[AttributeType])
	assert.Nil(t, s.batch)
	assert.Equal(t, opaqueCursor(9), readStateCursor(t, s))
}

func TestBatch_UndoWithoutUndoDropsBlocksAboveLastValid(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.batchMaxBlocks, s.batchMaxWait = 100, time.Minute
	for _, num := range []uint64{8, 9, 10, 11} {
		addBlock(t, s, num, false, time.Now())
	}

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 9, Id: "aa"}}
	require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(9))))

	msgs := pub.messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, []uint64{8, 9}, batchBlockNumbers(t, msgs[0].Data))
	assert.Nil(t, s.batch)
	assert.Equal(t, opaqueCursor(9), readStateCursor(t, s))
}

func TestRecoverPending_DiscardsSingleBlockShape(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)

	p := newPending(10)
	p.Batched = false
	p.Fingerprint = s.fingerprint
	require.NoError(t, writePending(s.pendingFile, p))

	start := sink.MustNewCursor(opaqueCursor(9))
	got, err := s.recoverPending(context.Background(), start)
	require.NoError(t, err)
	assert.Same(t, start, got)
	assert.Empty(t, pub.messages())
	assert.NoFileExists(t, s.pendingFile)
}

func TestDefaultOnFailureIsExit(t *testing.T) {
	s := newSink(SinkConfig{}, "my-gcp-project", "events", &fakePublisher{}, nil, nil, nil, zlogTest)
	assert.Equal(t, OnFailureExit, s.onFailure)
}

func TestParseOnFailure(t *testing.T) {
	got, err := ParseOnFailure("exit")
	require.NoError(t, err)
	assert.Equal(t, OnFailureExit, got)
	got, err = ParseOnFailure("skip")
	require.NoError(t, err)
	assert.Equal(t, OnFailureSkip, got)
	_, err = ParseOnFailure("pause")
	assert.Error(t, err)
}

func TestTerminationReason_OnlyWrittenWhenFileExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	require.NoError(t, writeTerminationReason(path, []byte("x")))
	assert.NoFileExists(t, path)
	require.NoError(t, writeTerminationReason("", []byte("x")))
}
