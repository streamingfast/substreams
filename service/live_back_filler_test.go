package service

import (
	"context"
	"testing"
	"time"

	"github.com/streamingfast/substreams/block"

	"github.com/streamingfast/bstream"

	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/test-go/testify/require"

	"github.com/streamingfast/substreams/client"
	"go.uber.org/zap"
)

func TestBackFiller(t *testing.T) {
	cases := []struct {
		name                     string
		segmentSize              uint64
		blocks                   []*pbbstream.Block
		linearHandoff            uint64
		stageToProcess           int
		errorBackProcessing      bool
		expectedSegmentProcessed []uint64
	}{
		{
			name:        "sunny path",
			segmentSize: 10,
			blocks: []*pbbstream.Block{
				{Number: 199},
				{Number: 200},
				{Number: 201},
				{Number: 202},
				{Number: 223},
			},
			linearHandoff:            100,
			errorBackProcessing:      false,
			expectedSegmentProcessed: []uint64{11},
		},
		{
			name:        "with job failing",
			segmentSize: 10,
			blocks: []*pbbstream.Block{
				{Number: 199},
				{Number: 200},
				{Number: 201},
				{Number: 202},
				{Number: 223},
			},
			linearHandoff:            100,
			errorBackProcessing:      true,
			expectedSegmentProcessed: []uint64{},
		},

		{
			name:        "processing multiple segments",
			segmentSize: 10,
			blocks: []*pbbstream.Block{
				{Number: 199},
				{Number: 200},
				{Number: 201},
				{Number: 202},
				{Number: 223},
				{Number: 260},
			},
			linearHandoff:            100,
			errorBackProcessing:      false,
			expectedSegmentProcessed: []uint64{11, 12, 13, 14},
		},
	}

	testContext := context.Background()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testHandler := &testNextHandler{}
			testLogger := zap.NewNop()
			segmentProcessed := make([]uint64, 0)

			RequestBackProcessingTest := func(ctx context.Context, logger *zap.Logger, blockRange *block.Range, stageToProcess int, clientFactory client.InternalClientFactory, jobCompleted chan struct{}, jobFailed *bool) {
				if c.errorBackProcessing {
					*jobFailed = true
				} else {
					segmentNumber := blockRange.ExclusiveEndBlock / c.segmentSize
					segmentProcessed = append(segmentProcessed, segmentNumber)
				}

				jobCompleted <- struct{}{}
			}

			testLiveBackFiller := NewLiveBackFiller(testHandler, testLogger, c.stageToProcess, c.segmentSize, c.linearHandoff, nil, RequestBackProcessingTest)

			go testLiveBackFiller.Start(testContext)

			testSource := bstream.NewTestSource(testLiveBackFiller)

			go testSource.Run()

			for _, block := range c.blocks {
				obj := &testObject{step: bstream.StepIrreversible}
				err := testSource.Push(block, obj)
				require.NoError(t, err)
			}

			require.Equal(t, c.expectedSegmentProcessed, segmentProcessed)
		})
	}
}

type testObject struct {
	step bstream.StepType
}

func (t *testObject) Step() bstream.StepType {
	return t.step
}
func (t *testObject) FinalBlockHeight() uint64 {
	return 0
}

func (t *testObject) ReorgJunctionBlock() bstream.BlockRef {
	return nil
}

type testNextHandler struct{}

func (t *testNextHandler) ProcessBlock(blk *pbbstream.Block, obj interface{}) (err error) {
	time.Sleep(1 * time.Millisecond)
	return nil
}
