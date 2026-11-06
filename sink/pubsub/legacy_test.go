package pubsub

import (
	"context"
	"strconv"
	"testing"
	"time"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/anypb"
)

func appendBytesField(b []byte, num protowire.Number, v []byte) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendBytes(b, v)
}

func appendStringField(b []byte, num protowire.Number, s string) []byte {
	b = protowire.AppendTag(b, num, protowire.BytesType)
	return protowire.AppendString(b, s)
}

func legacyAttributeBytes(key, value string) []byte {
	var b []byte
	b = appendStringField(b, 1, key)
	return appendStringField(b, 2, value)
}

// legacyMessageBytes builds one sf.substreams.sink.pubsub.v1.Message.
// includeData false leaves field 1 unset, which the sink publishes as nil data.
func legacyMessageBytes(data []byte, includeData bool, attrs ...[2]string) []byte {
	var b []byte
	if includeData {
		b = appendBytesField(b, 1, data)
	}
	for _, attr := range attrs {
		b = appendBytesField(b, 2, legacyAttributeBytes(attr[0], attr[1]))
	}
	return b
}

func legacyPublishBytes(messages ...[]byte) []byte {
	var b []byte
	for _, msg := range messages {
		b = appendBytesField(b, 1, msg)
	}
	return b
}

func publishBlock(typeURL string, value []byte, num uint64) *pbsubstreamsrpc.BlockScopedData {
	return &pbsubstreamsrpc.BlockScopedData{
		Clock: protoClock(num),
		Output: &pbsubstreamsrpc.MapModuleOutput{
			Name: "map_clocks",
			MapOutput: &anypb.Any{
				TypeUrl: typeURL,
				Value:   value,
			},
		},
	}
}

func TestIsLegacyPublishType(t *testing.T) {
	for _, typeURL := range []string{
		legacyPublishType,
		"type.googleapis.com/" + legacyPublishType,
		"proto:" + legacyPublishType,
		"proto:type.googleapis.com/" + legacyPublishType,
	} {
		assert.True(t, isLegacyPublishType(typeURL), typeURL)
	}
	assert.False(t, isLegacyPublishType("sf.example.v1.Events"))
	assert.False(t, isLegacyPublishType(""))
}

func TestLegacyBlockMessages_MatchesSubstreamsSinkPubsub(t *testing.T) {
	raw := legacyPublishBytes(
		legacyMessageBytes([]byte{0x00, 0x00, 0x00, 0x0c}, true, [2]string{"timestamp", "test"}, [2]string{"Cursor", "from-the-module"}),
		legacyMessageBytes(nil, false, [2]string{"k", "first"}, [2]string{"k", "second"}),
	)
	raw = protowire.AppendTag(raw, 3, protowire.VarintType)
	raw = protowire.AppendVarint(raw, 7)

	cursor := opaqueCursor(12)
	msgs, err := legacyBlockMessages(12, cursor, raw)
	require.NoError(t, err)
	require.Len(t, msgs, 2)

	assert.Equal(t, []byte{0x00, 0x00, 0x00, 0x0c}, msgs[0].Data)
	assert.Equal(t, "test", msgs[0].Attributes["timestamp"])
	assert.Equal(t, cursor, msgs[0].Attributes["Cursor"])
	assert.Equal(t, "000000012_00000", msgs[0].OrderingKey)
	assert.NotContains(t, msgs[0].Attributes, AttributeType)

	assert.Nil(t, msgs[1].Data)
	assert.Equal(t, "second", msgs[1].Attributes["k"])
	assert.Equal(t, cursor, msgs[1].Attributes["Cursor"])
	assert.Equal(t, "000000012_00001", msgs[1].OrderingKey)
}

func TestLegacyBlockMessages_EmptyDataStaysEmpty(t *testing.T) {
	msgs, err := legacyBlockMessages(1, "c", legacyPublishBytes(legacyMessageBytes([]byte{}, true)))
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.NotNil(t, msgs[0].Data)
	assert.Empty(t, msgs[0].Data)
}

func TestLegacyBlockMessages_EmptyPublish(t *testing.T) {
	msgs, err := legacyBlockMessages(4, "c", nil)
	require.NoError(t, err)
	assert.Empty(t, msgs)

	msgs, err = legacyBlockMessages(4, "c", []byte{})
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestLegacyBlockMessages_Malformed(t *testing.T) {
	_, err := legacyBlockMessages(1, "c", []byte{0x0a, 0x05})
	require.Error(t, err)
	assert.ErrorContains(t, err, "reading Publish output")
}

func TestLegacyUndoMessage(t *testing.T) {
	msg := legacyUndoMessage(10, "cursor-value")
	assert.Nil(t, msg.Data)
	assert.Empty(t, msg.OrderingKey)
	assert.Equal(t, map[string]string{
		"LastValidBlock": "10",
		"Step":           "Undo",
		"Cursor":         "cursor-value",
	}, msg.Attributes)
}

func TestHandleBlockScopedData_PublishModuleUsesLegacyWireFormat(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	s.batchMaxBlocks = 10

	beforePublishes := PublishesCounter.Get()
	beforeBytes := PublishedBytes.Get()

	cursor := sink.MustNewCursor(opaqueCursor(12))
	raw := legacyPublishBytes(
		legacyMessageBytes([]byte("one"), true, [2]string{"timestamp", "test"}),
		legacyMessageBytes([]byte("two"), true),
	)
	require.NoError(t, s.handleBlockScopedData(context.Background(), publishBlock("type.googleapis.com/"+legacyPublishType, raw, 12), nil, cursor))

	msgs := pub.messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, []byte("one"), msgs[0].Data)
	assert.Equal(t, "test", msgs[0].Attributes["timestamp"])
	assert.Equal(t, cursor.String(), msgs[0].Attributes["Cursor"])
	assert.Equal(t, "000000012_00000", msgs[0].OrderingKey)
	assert.NotContains(t, msgs[0].Attributes, AttributeType)
	assert.Equal(t, []byte("two"), msgs[1].Data)
	assert.Equal(t, "000000012_00001", msgs[1].OrderingKey)
	assert.NotContains(t, msgs[1].Attributes, AttributeType)
	assert.Equal(t, cursor.String(), readStateCursor(t, s))
	assert.True(t, s.legacyPublish)
	assert.Nil(t, s.batch)
	assert.Equal(t, beforePublishes+2, PublishesCounter.Get())
	assert.Equal(t, beforeBytes+float64(len("one")+len("two")), PublishedBytes.Get())
}

func TestHandleBlockScopedData_ProtoPrefixIsLegacy(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	raw := legacyPublishBytes(legacyMessageBytes([]byte{1}, true))
	require.NoError(t, s.handleBlockScopedData(context.Background(), publishBlock("proto:"+legacyPublishType, raw, 3), nil, sink.MustNewCursor(opaqueCursor(3))))
	require.Len(t, pub.messages(), 1)
	assert.Equal(t, "000000003_00000", pub.messages()[0].OrderingKey)
	assert.Equal(t, []byte{1}, pub.messages()[0].Data)
}

func TestHandleBlockScopedData_EmptyPublishCommitsCursor(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	before := PublishesCounter.Get()
	cursor := sink.MustNewCursor(opaqueCursor(8))
	require.NoError(t, s.handleBlockScopedData(context.Background(), publishBlock("type.googleapis.com/"+legacyPublishType, []byte{}, 8), nil, cursor))
	assert.Empty(t, pub.messages())
	assert.Equal(t, cursor.String(), readStateCursor(t, s))
	assert.Equal(t, before, PublishesCounter.Get())
	assert.True(t, s.legacyPublish)
}

func TestHandleBlockScopedData_NilPublishValueIsIgnored(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	data := publishBlock("type.googleapis.com/"+legacyPublishType, nil, 8)
	require.NoError(t, s.handleBlockScopedData(context.Background(), data, nil, sink.MustNewCursor(opaqueCursor(8))))
	assert.Empty(t, pub.messages())
	assert.Empty(t, readStateCursor(t, s))
	assert.False(t, s.legacyPublish)
}

func TestHandleBlockScopedData_MalformedPublish(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	err := s.handleBlockScopedData(context.Background(), publishBlock("type.googleapis.com/"+legacyPublishType, []byte{0x0a, 0x05}, 8), nil, sink.MustNewCursor(opaqueCursor(8)))
	require.Error(t, err)
	assert.Empty(t, pub.messages())
	assert.Empty(t, readStateCursor(t, s))
}

func TestHandleBlockUndoSignal_PublishModuleAlwaysUsesLegacyUndo(t *testing.T) {
	undo := &pbsubstreamsrpc.BlockUndoSignal{LastValidBlock: &pbsubstreams.BlockRef{Number: 10, Id: "aa"}}
	for _, undoFlag := range []bool{false, true} {
		t.Run(strconv.FormatBool(undoFlag), func(t *testing.T) {
			pub := &fakePublisher{}
			s := newTestSink(t, pub, OnFailureExit)
			if undoFlag {
				s.enableUndo()
			}
			s.legacyPublish = true
			cursor := sink.MustNewCursor(opaqueCursor(10))
			require.NoError(t, s.handleBlockUndoSignal(context.Background(), undo, cursor))

			msgs := pub.messages()
			require.Len(t, msgs, 1)
			assert.Nil(t, msgs[0].Data)
			assert.Empty(t, msgs[0].OrderingKey)
			assert.Equal(t, map[string]string{
				"LastValidBlock": "10",
				"Step":           "Undo",
				"Cursor":         cursor.String(),
			}, msgs[0].Attributes)
			assert.Equal(t, cursor.String(), readStateCursor(t, s))
		})
	}
}

func TestLegacyPending_RoundTripAndRecover(t *testing.T) {
	pub := &fakePublisher{fail: -1}
	s := newTestSink(t, pub, OnFailureExit)
	s.batchMaxBlocks = 4
	cursor := sink.MustNewCursor(opaqueCursor(12))
	raw := legacyPublishBytes(legacyMessageBytes([]byte("one"), true, [2]string{"timestamp", "test"}))
	err := s.handleBlockScopedData(context.Background(), publishBlock("type.googleapis.com/"+legacyPublishType, raw, 12), nil, cursor)
	var failed *DeliveryFailedError
	require.ErrorAs(t, err, &failed)

	kept, err := readPending(s.pendingFile)
	require.NoError(t, err)
	require.True(t, kept.Legacy)
	require.Len(t, kept.Messages, 1)
	assert.Equal(t, []byte("one"), kept.Messages[0].Data)
	assert.Equal(t, "000000012_00000", kept.Messages[0].OrderingKey)
	assert.Equal(t, "test", kept.Messages[0].Attributes["timestamp"])
	assert.Equal(t, cursor.String(), kept.Messages[0].Attributes["Cursor"])

	// Batching is on and undo publishing is off, which is how a Publish module
	// restarts. The pending messages are published again in the same format.
	restartPub := &fakePublisher{}
	restarted := newTestSink(t, restartPub, OnFailureExit)
	restarted.batchMaxBlocks = 4
	restarted.stateFile = s.stateFile
	restarted.pendingFile = s.pendingFile
	restarted.fingerprint = s.fingerprint
	got, err := restarted.recoverPending(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, cursor.String(), got.String())
	require.Len(t, restartPub.messages(), 1)
	assert.Equal(t, []byte("one"), restartPub.messages()[0].Data)
	assert.Equal(t, "000000012_00000", restartPub.messages()[0].OrderingKey)
	assert.True(t, restarted.legacyPublish)
	assert.NoFileExists(t, s.pendingFile)
}

func TestRecoverPending_LegacyUndoPublishesWhenUndoFlagIsOff(t *testing.T) {
	pub := &fakePublisher{}
	s := newTestSink(t, pub, OnFailureExit)
	cursor := opaqueCursor(9)
	pending := &pendingDelivery{
		Kind:           DeliveryKindUndo,
		Legacy:         true,
		Messages:       []Message{legacyUndoMessage(9, cursor)},
		Cursor:         cursor,
		BlockNumber:    9,
		FirstAttemptAt: time.Now().Add(-time.Hour),
		Fingerprint:    s.fingerprint,
	}
	require.NoError(t, writePending(s.pendingFile, pending))

	got, err := s.recoverPending(context.Background(), sink.MustNewCursor(opaqueCursor(12)))
	require.NoError(t, err)
	assert.Equal(t, cursor, got.String())
	require.Len(t, pub.messages(), 1)
	assert.Equal(t, "Undo", pub.messages()[0].Attributes["Step"])
	assert.Equal(t, "9", pub.messages()[0].Attributes["LastValidBlock"])
	assert.NotContains(t, pub.messages()[0].Attributes, AttributeType)
	assert.True(t, s.legacyPublish)
	assert.NoFileExists(t, s.pendingFile)
}
