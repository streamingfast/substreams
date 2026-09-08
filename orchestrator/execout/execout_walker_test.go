package execout

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/orchestrator/response"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreamsrpcv4 "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v4"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/execout"
	pboutput "github.com/streamingfast/substreams/storage/execout/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// itemsReader is a FileReader over an in-memory list of items.
type itemsReader struct {
	items []*pboutput.Item
	pos   int
}

func (r *itemsReader) ReadNext() (*pboutput.Item, error) {
	if r.pos >= len(r.items) {
		return nil, errors.New("eof")
	}
	item := r.items[r.pos]
	r.pos++
	return item, nil
}

func (r *itemsReader) Iter() iter.Seq2[*pboutput.Item, error] {
	return func(yield func(*pboutput.Item, error) bool) {
		for r.pos < len(r.items) {
			item := r.items[r.pos]
			r.pos++
			if !yield(item, nil) {
				return
			}
		}
	}
}

func (r *itemsReader) Get(context.Context, uint64) ([]byte, bool, error) { return nil, false, nil }
func (r *itemsReader) ModuleName() string                                { return "map" }
func (r *itemsReader) Filename() string                                  { return "test" }
func (r *itemsReader) Close() error                                      { return nil }

var _ execout.FileReader = (*itemsReader)(nil)

func newItems(from, to uint64) []*pboutput.Item {
	var out []*pboutput.Item
	for i := from; i < to; i++ {
		out = append(out, &pboutput.Item{BlockNum: i, BlockId: "id", Payload: []byte{byte(i)}})
	}
	return out
}

// sink records the blocks it receives and can fail or stall on demand.
type sink struct {
	mu       sync.Mutex
	blocks   []uint64
	messages int
	failAt   int // message index whose send fails, -1 for never
	delay    time.Duration
}

func (s *sink) respFunc(resp substreams.ResponseFromAnyTier) error {
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.messages == s.failAt {
		s.messages++
		return errors.New("client went away")
	}
	s.messages++
	switch r := resp.(type) {
	case *pbsubstreamsrpc.Response:
		s.blocks = append(s.blocks, r.GetBlockScopedData().Clock.Number)
	case *pbsubstreamsrpcv4.Response:
		for _, d := range r.GetBlockScopedDatas().Items {
			s.blocks = append(s.blocks, d.Clock.Number)
		}
	}
	return nil
}

func (s *sink) received() (blocks []uint64, messages int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.blocks...), s.messages
}

func newTestWalker(t *testing.T, s *sink, buffering bool, bufferSize int) *Walker {
	t.Helper()
	module := &pbsubstreams.Module{Name: "map", Output: &pbsubstreams.Module_Output{Type: "proto:x.Y"}}
	return NewWalker(context.Background(), module, nil, block.NewRange(0, 1000), response.New(s.respFunc), false, bufferSize, buffering)
}

func TestSendItems_KeepsBlockOrderWhileSendsOverlap(t *testing.T) {
	for _, buffering := range []bool{true, false} {
		t.Run(map[bool]string{true: "buffered", false: "unbuffered"}[buffering], func(t *testing.T) {
			s := &sink{failAt: -1, delay: 200 * time.Microsecond}
			w := newTestWalker(t, s, buffering, 10)

			require.NoError(t, w.sendItems(&itemsReader{items: newItems(0, 250)}))

			blocks, _ := s.received()
			require.Len(t, blocks, 250)
			for i, b := range blocks {
				assert.Equal(t, uint64(i), b, "block at position %d", i)
			}
		})
	}
}

func TestSendItems_SendErrorStopsTheSegment(t *testing.T) {
	s := &sink{failAt: 1}
	w := newTestWalker(t, s, true, 10)

	err := w.sendItems(&itemsReader{items: newItems(0, 250)})
	require.ErrorContains(t, err, "client went away")

	blocks, messages := s.received()
	assert.Len(t, blocks, 11, "only the first batch made it to the client")
	assert.LessOrEqual(t, messages, 2, "nothing is sent after the failed message")
}

func TestSendItems_PanicInSendFailsTheSegment(t *testing.T) {
	calls := 0
	panicking := func(substreams.ResponseFromAnyTier) error {
		calls++
		if calls == 2 {
			panic("stream is gone")
		}
		return nil
	}
	module := &pbsubstreams.Module{Name: "map", Output: &pbsubstreams.Module_Output{Type: "proto:x.Y"}}
	w := NewWalker(context.Background(), module, nil, block.NewRange(0, 1000), response.New(panicking), false, 10, true)

	err := w.sendItems(&itemsReader{items: newItems(0, 250)})
	require.ErrorContains(t, err, "panic while sending execout data: stream is gone")
	assert.Equal(t, 2, calls, "nothing is sent after the panic")
}

func TestSendItems_StopsAtExclusiveEndBlock(t *testing.T) {
	s := &sink{failAt: -1}
	w := newTestWalker(t, s, true, 10)
	w.Range = block.NewRange(5, 20)

	require.NoError(t, w.sendItems(&itemsReader{items: newItems(0, 250)}))

	blocks, _ := s.received()
	require.Len(t, blocks, 15)
	assert.Equal(t, uint64(5), blocks[0])
	assert.Equal(t, uint64(19), blocks[14])
}
