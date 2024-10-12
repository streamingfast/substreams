package metering

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/streamingfast/bstream"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/dmetering"
	"github.com/streamingfast/dstore"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestWithBlockBytesReadMeteringOptions(t *testing.T) {
	meter := dmetering.NewBytesMeter()

	opts := WithBlockBytesReadMeteringOptions(meter, nil)

	store, err := dstore.NewStore("memory://test", ".test", "zstd", false, opts...)
	if err != nil {
		t.Fatal(err)
	}

	err = store.WriteObject(nil, "test", bytes.NewReader([]byte("1111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")))
	if err != nil {
		t.Fatal(err)
	}

	r, err := store.OpenObject(nil, "test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()

	assert.Equal(t, 24, meter.GetCount(MeterFileCompressedReadBytes))
	assert.Equal(t, 0, meter.GetCount(MeterFileUncompressedReadBytes))
	assert.Equal(t, 0, meter.GetCount(MeterFileUncompressedWriteBytes))
	assert.Equal(t, 0, meter.GetCount(MeterFileCompressedWriteBytes))
	assert.Equal(t, 0, meter.GetCount(MeterLiveUncompressedReadBytes))
}

func TestWithBytesReadMeteringOptions(t *testing.T) {
	meter := dmetering.NewBytesMeter()

	opts := WithBytesMeteringOptions(meter, nil)

	store, err := dstore.NewStore("memory://test", ".test", "zstd", false, opts...)
	if err != nil {
		t.Fatal(err)
	}

	err = store.WriteObject(nil, "test", bytes.NewReader([]byte("1111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111111")))
	if err != nil {
		t.Fatal(err)
	}

	r, err := store.OpenObject(nil, "test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()

	assert.Equal(t, 24, meter.GetCount(MeterFileCompressedReadBytes))
	assert.Equal(t, 727, meter.GetCount(MeterFileUncompressedReadBytes))
	assert.Equal(t, 727, meter.GetCount(MeterFileUncompressedWriteBytes))
	assert.Equal(t, 24, meter.GetCount(MeterFileCompressedWriteBytes))
	assert.Equal(t, 0, meter.GetCount(MeterLiveUncompressedReadBytes))
}

func TestFileSourceMiddlewareHandlerFactory(t *testing.T) {
	type test struct {
		Name            string
		Block           *pbsubstreamstest.Block
		Obj             bstream.Stepable
		ExpectedMetrics map[string]int
	}

	testBlock := &pbsubstreamstest.Block{
		Id:     "abc",
		Number: 123,
	}

	for _, tt := range []test{
		{
			Name:  "step new",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepNew,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  7,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  0,
			},
		},
		{
			Name:  "step new irreversible",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepNewIrreversible,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  7,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  0,
			},
		},
		{
			Name:  "step undo not metered",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepUndo,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  0,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  0,
			},
		},
		{
			Name:  "step stalled not metered",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepStalled,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  0,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  0,
			},
		},
		{
			Name:  "step irreversible not metered",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepIrreversible,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  0,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  0,
			},
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			ctx := dmetering.WithBytesMeter(context.Background())
			meter := dmetering.GetBytesMeter(ctx)

			handler := bstream.HandlerFunc(func(blk *pbbstream.Block, obj interface{}) error {
				return nil
			})

			testHandler := FileSourceMiddlewareHandlerFactory(ctx)(handler)

			err := testHandler.ProcessBlock(bstreamBlk(t, tt.Block), tt.Obj)
			assert.NoError(t, err)

			for k, v := range tt.ExpectedMetrics {
				assert.Equal(t, v, meter.GetCount(k))
			}
		})
	}
}

func TestLiveSourceMiddlewareHandlerFactory(t *testing.T) {
	type test struct {
		Name            string
		Block           *pbsubstreamstest.Block
		Obj             bstream.Stepable
		ExpectedMetrics map[string]int
	}

	testBlock := &pbsubstreamstest.Block{
		Id:     "abc",
		Number: 123,
	}

	for _, tt := range []test{
		{
			Name:  "step new",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepNew,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  0,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  7,
			},
		},
		{
			Name:  "step new irreversible",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepNew,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  0,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  7,
			},
		},
		{
			Name:  "step undo not metered",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepUndo,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  0,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  0,
			},
		},
		{
			Name:  "step stalled not metered",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepStalled,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  0,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  0,
			},
		},
		{
			Name:  "step irreversible not metered",
			Block: testBlock,
			Obj: &testStepableObject{
				bstream.StepIrreversible,
			},
			ExpectedMetrics: map[string]int{
				MeterFileCompressedReadBytes:    0,
				MeterFileUncompressedReadBytes:  0,
				MeterFileUncompressedWriteBytes: 0,
				MeterFileCompressedWriteBytes:   0,
				MeterLiveUncompressedReadBytes:  0,
			},
		},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			ctx := dmetering.WithBytesMeter(context.Background())
			meter := dmetering.GetBytesMeter(ctx)

			handler := bstream.HandlerFunc(func(blk *pbbstream.Block, obj interface{}) error {
				return nil
			})

			testHandler := LiveSourceMiddlewareHandlerFactory(ctx)(handler)

			err := testHandler.ProcessBlock(bstreamBlk(t, tt.Block), tt.Obj)
			assert.NoError(t, err)

			for k, v := range tt.ExpectedMetrics {
				assert.Equal(t, v, meter.GetCount(k))
			}
		})
	}
}

func bstreamBlk(t *testing.T, blk *pbsubstreamstest.Block) *pbbstream.Block {
	payload, err := anypb.New(blk)
	assert.NoError(t, err)

	bb := &pbbstream.Block{
		Id:      blk.Id,
		Number:  blk.Number,
		Payload: payload,
	}

	return bb
}

type testStepableObject struct {
	step bstream.StepType
}

func (t *testStepableObject) Step() bstream.StepType {
	return t.step
}
func (t *testStepableObject) FinalBlockHeight() uint64 {
	return 0
}
func (t *testStepableObject) ReorgJunctionBlock() bstream.BlockRef {
	return nil
}
