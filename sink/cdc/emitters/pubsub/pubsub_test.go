package pubsub

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/streamingfast/substreams/sink/cdc"
	sinkpubsub "github.com/streamingfast/substreams/sink/pubsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakePublisher struct {
	mu   sync.Mutex
	msgs []sinkpubsub.Message
	fail error
}

func (f *fakePublisher) Publish(_ context.Context, msg sinkpubsub.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.msgs = append(f.msgs, msg)
	return nil
}

func event(id, table string, op cdc.Op, key string, block uint64) cdc.Event {
	return cdc.Event{ID: id, Op: op, Key: map[string]string{"id": key}, Source: cdc.Source{Table: table, Block: cdc.Block{Number: block}}}
}

func perKey(msgs []sinkpubsub.Message, key string) []string {
	var out []string
	for _, m := range msgs {
		if m.OrderingKey == key {
			out = append(out, m.Attributes["id"])
		}
	}
	return out
}

func TestEmitter_AttributesAndOrderingByTable(t *testing.T) {
	pub := &fakePublisher{}
	e, err := New(pub, nil, OrderingTable, 4)
	require.NoError(t, err)

	b := &cdc.Batch{Events: []cdc.Event{
		event("1", "t1", cdc.OpCreate, "a", 7),
		event("2", "t2", cdc.OpCreate, "a", 7),
		event("3", "t1", cdc.OpCreate, "b", 7),
	}}
	require.NoError(t, e.EmitBatch(t.Context(), b))
	require.Len(t, pub.msgs, 3)

	assert.Equal(t, []string{"1", "3"}, perKey(pub.msgs, "t1"))
	for _, m := range pub.msgs {
		if m.Attributes["id"] == "2" {
			assert.Equal(t, map[string]string{"op": "c", "table": "t2", "block_num": "7", "id": "2"}, m.Attributes)
			assert.Contains(t, string(m.Data), `"id":"2"`)
		}
	}
}

func TestEmitter_TombstoneSharesKeyWithOriginal(t *testing.T) {
	for _, ordering := range []Ordering{OrderingTable, OrderingKey} {
		t.Run(string(ordering), func(t *testing.T) {
			pub := &fakePublisher{}
			e, err := New(pub, nil, ordering, 4)
			require.NoError(t, err)

			created := event("1", "t1", cdc.OpCreate, "a", 7)
			tomb := event("2", "t1", cdc.OpDelete, "a", 7)
			tomb.RevertOf = "1"
			require.NoError(t, e.EmitBatch(t.Context(), &cdc.Batch{Events: []cdc.Event{created}}))
			require.NoError(t, e.EmitCompensation(t.Context(), &cdc.Batch{Events: []cdc.Event{tomb}}))

			require.Len(t, pub.msgs, 2)
			assert.Equal(t, pub.msgs[0].OrderingKey, pub.msgs[1].OrderingKey)
			assert.NotEmpty(t, pub.msgs[1].OrderingKey)
			assert.Equal(t, "1", pub.msgs[1].Attributes["revert_of"])
		})
	}
}

func TestEmitter_OrderingKeyAndNone(t *testing.T) {
	e, err := New(&fakePublisher{}, nil, OrderingKey, 1)
	require.NoError(t, err)
	a, b := event("1", "t", cdc.OpCreate, "a", 1), event("2", "t", cdc.OpCreate, "b", 1)
	assert.NotEqual(t, e.OrderingKey(&a), e.OrderingKey(&b))

	e, err = New(&fakePublisher{}, nil, OrderingNone, 1)
	require.NoError(t, err)
	assert.Empty(t, e.OrderingKey(&a))
}

func TestEmitter_PublishErrorIsReturned(t *testing.T) {
	pub := &fakePublisher{fail: errors.New("down")}
	e, err := New(pub, nil, OrderingTable, 2)
	require.NoError(t, err)
	require.ErrorContains(t, e.EmitBatch(t.Context(), &cdc.Batch{Events: []cdc.Event{event("1", "t", cdc.OpCreate, "a", 1)}}), "down")
}

func TestFactory(t *testing.T) {
	pub := &fakePublisher{}
	var gotProject, gotTopic string
	old := publisherFactory
	publisherFactory = func(_ context.Context, project, topic string, _ sinkpubsub.RetryConfig, _ *zap.Logger) (sinkpubsub.Publisher, closer, error) {
		gotProject, gotTopic = project, topic
		return pub, nil, nil
	}
	defer func() { publisherFactory = old }()

	e, err := cdc.NewEmitter(t.Context(), "pubsub://my-proj/my-topic?ordering=key", cdc.EmitterOptions{})
	require.NoError(t, err)
	assert.Equal(t, "my-proj", gotProject)
	assert.Equal(t, "my-topic", gotTopic)
	require.NoError(t, e.Close())

	_, err = cdc.NewEmitter(t.Context(), "pubsub://my-proj/my-topic?ordering=bogus", cdc.EmitterOptions{})
	require.Error(t, err)
	_, err = cdc.NewEmitter(t.Context(), "pubsub://my-proj", cdc.EmitterOptions{})
	require.Error(t, err)
}
