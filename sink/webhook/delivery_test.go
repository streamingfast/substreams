package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func opaqueCursor(num uint64) string {
	ref := bstream.NewBlockRef("aa", num)
	return (&bstream.Cursor{Step: bstream.StepNew, Block: ref, HeadBlock: ref, LIB: bstream.NewBlockRef("00", 1)}).ToOpaque()
}

// togglingServer answers with the status held in status, and counts calls.
type togglingServer struct {
	*httptest.Server
	status atomic.Int32
	calls  atomic.Int32
}

func newTogglingServer(t *testing.T, status int) *togglingServer {
	t.Helper()
	s := &togglingServer{}
	s.status.Store(int32(status))
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		w.WriteHeader(int(s.status.Load()))
	}))
	t.Cleanup(s.Close)
	return s
}

func newTestSink(t *testing.T, url string, onFailure OnFailure) *Sink {
	t.Helper()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.cursor")
	terminationLog := filepath.Join(dir, "termination-log")
	require.NoError(t, os.WriteFile(terminationLog, nil, 0o644))

	cfg := Config{Timeout: time.Second, MaxRetries: 1, MaxInterval: 5 * time.Millisecond, AuthHeaderValue: "Bearer x"}
	return &Sink{
		webhookURL:     url,
		stateFile:      stateFile,
		pendingFile:    pendingFilePath(stateFile),
		onFailure:      onFailure,
		terminationLog: terminationLog,
		fingerprint:    configFingerprint(url, "", cfg),
		client:         NewClient(cfg, zap.NewNop()),
		logger:         zap.NewNop(),
	}
}

func newPending(num uint64) *pendingDelivery {
	return &pendingDelivery{
		Cursor:         opaqueCursor(num),
		BlockNumber:    num,
		Payload:        json.RawMessage(`{"clock":{"number":` + jsonNumber(num) + `}}`),
		FirstAttemptAt: time.Now().Add(-time.Hour),
	}
}

func jsonNumber(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
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

	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temp file left behind")

	require.NoError(t, removePending(path))
	require.NoError(t, removePending(path), "removing twice is fine")
	assert.Empty(t, pendingFilePath(""))
}

func TestSend_SuccessCommitsCursorAndClearsPending(t *testing.T) {
	server := newTogglingServer(t, http.StatusOK)
	s := newTestSink(t, server.URL, OnFailureExit)

	p := newPending(10)
	s.fingerprint = "fp"
	p.Fingerprint = "fp"
	require.NoError(t, s.send(context.Background(), p))

	assert.Equal(t, int32(1), server.calls.Load())
	assert.Equal(t, opaqueCursor(10), readStateCursor(t, s))
	assert.NoFileExists(t, s.pendingFile)
}

func TestSend_ExitModeKeepsPendingAndWritesTerminationMessage(t *testing.T) {
	server := newTogglingServer(t, http.StatusServiceUnavailable)
	s := newTestSink(t, server.URL, OnFailureExit)

	p := newPending(10)
	err := s.send(context.Background(), p)

	var failed *DeliveryFailedError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, uint64(10), failed.Delivery.BlockNumber)
	assert.Equal(t, http.StatusServiceUnavailable, failed.Delivery.StatusCode)
	assert.Equal(t, 2, failed.Delivery.Attempts)
	assert.Equal(t, p.FirstAttemptAt, failed.FirstAttemptAt)

	assert.Empty(t, readStateCursor(t, s), "cursor must not move past an undelivered block")

	kept, err := readPending(s.pendingFile)
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, uint64(10), kept.BlockNumber)

	msg, err := os.ReadFile(s.terminationLog)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(msg, &decoded))
	assert.Equal(t, "webhook_delivery_failed", decoded["reason"])
	assert.Equal(t, server.URL, decoded["url"])
	assert.Equal(t, float64(10), decoded["block"])
	assert.Equal(t, float64(http.StatusServiceUnavailable), decoded["status"])
	assert.Equal(t, float64(2), decoded["attempts"])
	assert.Equal(t, p.FirstAttemptAt.UTC().Format(time.RFC3339), decoded["first_attempt_at"])
	assert.NotEmpty(t, decoded["error"])
}

func TestSend_SkipModeDropsBlock(t *testing.T) {
	server := newTogglingServer(t, http.StatusServiceUnavailable)
	s := newTestSink(t, server.URL, OnFailureSkip)

	require.NoError(t, s.send(context.Background(), newPending(10)))
	assert.NoFileExists(t, s.pendingFile)
	assert.Empty(t, readStateCursor(t, s))

	msg, err := os.ReadFile(s.terminationLog)
	require.NoError(t, err)
	assert.Empty(t, msg, "skip mode is not an exit, nothing to report")
}

func TestTerminationMessage_OnlyWrittenWhenFileExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	require.NoError(t, writeTerminationMessage(path, []byte("x")))
	assert.NoFileExists(t, path)
	require.NoError(t, writeTerminationMessage("", []byte("x")))
}

func TestRecoverPending_NothingPending(t *testing.T) {
	server := newTogglingServer(t, http.StatusOK)
	s := newTestSink(t, server.URL, OnFailureExit)

	start := sink.MustNewCursor(opaqueCursor(9))
	got, err := s.recoverPending(context.Background(), start)
	require.NoError(t, err)
	assert.Same(t, start, got)
	assert.Equal(t, int32(0), server.calls.Load())
}

func TestRecoverPending_DeliversBeforeStreaming(t *testing.T) {
	server := newTogglingServer(t, http.StatusOK)
	s := newTestSink(t, server.URL, OnFailureExit)

	p := newPending(10)
	p.Fingerprint = s.fingerprint
	require.NoError(t, writePending(s.pendingFile, p))
	require.NoError(t, sink.WriteCursor(s.stateFile, sink.MustNewCursor(opaqueCursor(9))))

	got, err := s.recoverPending(context.Background(), sink.MustNewCursor(opaqueCursor(9)))
	require.NoError(t, err)
	assert.Equal(t, opaqueCursor(10), got.String(), "stream resumes after the recovered block")
	assert.Equal(t, int32(1), server.calls.Load())
	assert.Equal(t, opaqueCursor(10), readStateCursor(t, s))
	assert.NoFileExists(t, s.pendingFile)
}

func TestRecoverPending_StillFailingExitsWithOriginalFirstAttempt(t *testing.T) {
	server := newTogglingServer(t, http.StatusServiceUnavailable)
	s := newTestSink(t, server.URL, OnFailureExit)

	p := newPending(10)
	p.Fingerprint = s.fingerprint
	require.NoError(t, writePending(s.pendingFile, p))

	_, err := s.recoverPending(context.Background(), nil)
	var failed *DeliveryFailedError
	require.ErrorAs(t, err, &failed)
	assert.WithinDuration(t, p.FirstAttemptAt, failed.FirstAttemptAt, time.Second, "a retry never moves the first attempt forward")
	assert.FileExists(t, s.pendingFile)
}

func TestRecoverPending_SkipModeDropsAndContinues(t *testing.T) {
	server := newTogglingServer(t, http.StatusServiceUnavailable)
	s := newTestSink(t, server.URL, OnFailureSkip)

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
	server := newTogglingServer(t, http.StatusServiceUnavailable)
	s := newTestSink(t, server.URL, OnFailureExit)

	p := newPending(10)
	p.Fingerprint = "written-under-the-old-url"
	require.NoError(t, writePending(s.pendingFile, p))

	_, err := s.recoverPending(context.Background(), nil)
	var failed *DeliveryFailedError
	require.ErrorAs(t, err, &failed)
	assert.WithinDuration(t, time.Now(), failed.FirstAttemptAt, 5*time.Second)

	kept, err := readPending(s.pendingFile)
	require.NoError(t, err)
	assert.Equal(t, s.fingerprint, kept.Fingerprint)
	assert.WithinDuration(t, time.Now(), kept.FirstAttemptAt, 5*time.Second)
}

func TestConfigFingerprint_CoversURLsAndSecrets(t *testing.T) {
	base := Config{AuthHeaderValue: "a", SigningSecret: "s"}
	fp := configFingerprint("https://example.com/hook", "", base)
	assert.Equal(t, fp, configFingerprint("https://example.com/hook", "", base))
	assert.NotEqual(t, fp, configFingerprint("https://example.com/other", "", base))
	assert.NotEqual(t, fp, configFingerprint("https://example.com/hook", "https://example.com/undo", base))
	assert.NotEqual(t, fp, configFingerprint("https://example.com/hook", "", Config{AuthHeaderValue: "b", SigningSecret: "s"}))
	assert.NotEqual(t, fp, configFingerprint("https://example.com/hook", "", Config{AuthHeaderValue: "a", SigningSecret: "t"}))
	assert.NotEqual(t, fp, configFingerprint("https://example.com/hook", "", Config{AuthHeaderName: "X", AuthHeaderValue: "a", SigningSecret: "s"}))
}

func TestUndoPayload_JSON(t *testing.T) {
	out, err := NewUndoPayload("map_events", &pbsubstreams.BlockRef{Number: 41, Id: "0xabc"}).ToJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"lastValidBlock":{"number":41,"id":"0xabc"},"manifest":{"moduleName":"map_events"}}`, string(out))

	out, err = NewUndoPayload("map_events", nil).ToJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"lastValidBlock":{"number":0,"id":""},"manifest":{"moduleName":"map_events"}}`, string(out))
}

func TestUndo_WithoutUndoURLOnlyMovesCursorBack(t *testing.T) {
	server := newTogglingServer(t, http.StatusOK)
	s := newTestSink(t, server.URL, OnFailureExit)
	require.NoError(t, sink.WriteCursor(s.stateFile, sink.MustNewCursor(opaqueCursor(12))))

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 10, Id: "aa"}, LastValidCursor: opaqueCursor(10)}
	require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(10))))

	assert.Equal(t, int32(0), server.calls.Load())
	assert.Equal(t, opaqueCursor(10), readStateCursor(t, s))
	assert.NoFileExists(t, s.pendingFile)
}

func TestUndo_GoesToUndoURLAndCommitsCursor(t *testing.T) {
	blocks := newTogglingServer(t, http.StatusOK)
	var undoBodies [][]byte
	undos := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		undoBodies = append(undoBodies, body)
		assert.Equal(t, "Bearer x", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(undos.Close)

	s := newTestSink(t, blocks.URL, OnFailureExit)
	s.undoURL = undos.URL
	s.moduleName = "map_events"
	require.NoError(t, sink.WriteCursor(s.stateFile, sink.MustNewCursor(opaqueCursor(12))))

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 10, Id: "aa"}}
	require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(10))))

	assert.Equal(t, int32(0), blocks.calls.Load(), "undo must not hit the block URL")
	require.Len(t, undoBodies, 1)
	assert.JSONEq(t, `{"lastValidBlock":{"number":10,"id":"aa"},"manifest":{"moduleName":"map_events"}}`, string(undoBodies[0]))
	assert.Equal(t, opaqueCursor(10), readStateCursor(t, s))
	assert.NoFileExists(t, s.pendingFile)
}

func TestUndo_FailureFollowsOnFailurePolicy(t *testing.T) {
	blocks := newTogglingServer(t, http.StatusOK)
	undos := newTogglingServer(t, http.StatusServiceUnavailable)

	s := newTestSink(t, blocks.URL, OnFailureExit)
	s.undoURL = undos.URL
	require.NoError(t, sink.WriteCursor(s.stateFile, sink.MustNewCursor(opaqueCursor(12))))

	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 10, Id: "aa"}}
	err := s.handleBlockUndoSignal(context.Background(), undo, sink.MustNewCursor(opaqueCursor(10)))

	var failed *DeliveryFailedError
	require.ErrorAs(t, err, &failed)
	assert.Equal(t, pendingKindUndo, failed.Kind)
	assert.Equal(t, undos.URL, failed.Delivery.URL)
	assert.Equal(t, opaqueCursor(12), readStateCursor(t, s), "cursor stays until the receiver knows about the reorg")

	kept, err := readPending(s.pendingFile)
	require.NoError(t, err)
	assert.Equal(t, pendingKindUndo, kept.Kind)

	msg, err := os.ReadFile(s.terminationLog)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(msg, &decoded))
	assert.Equal(t, "undo", decoded["kind"])

	// Recovery routes the pending undo to the undo URL once it accepts.
	undos.status.Store(http.StatusOK)
	got, err := s.recoverPending(context.Background(), sink.MustNewCursor(opaqueCursor(12)))
	require.NoError(t, err)
	assert.Equal(t, opaqueCursor(10), got.String())
	assert.Equal(t, int32(0), blocks.calls.Load())
	assert.Equal(t, int32(3), undos.calls.Load())
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
