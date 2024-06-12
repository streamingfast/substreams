package service

import (
	"context"
	"fmt"
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
			expectedSegmentProcessed: []uint64{11},
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
			segmentProcessed := make(chan uint64)

			RequestBackProcessingTest := func(ctx context.Context, logger *zap.Logger, blockRange *block.Range, stageToProcess int, clientFactory client.InternalClientFactory, jobResult chan error) {
				var err error
				if c.errorBackProcessing {
					err = fmt.Errorf("fail")
				}

				segmentNumber := blockRange.ExclusiveEndBlock / c.segmentSize
				segmentProcessed <- segmentNumber

				jobResult <- err
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

			done := make(chan struct{})
			receivedSegmentProcessed := make([]uint64, 0)
			go func() {
				for process := range segmentProcessed {
					receivedSegmentProcessed = append(receivedSegmentProcessed, process)
					if len(receivedSegmentProcessed) == len(c.expectedSegmentProcessed) {
						close(done)
						return
					}
				}
				panic("should not reach here")
			}()

			select {
			case <-done:
			case <-time.After(1 * time.Second):
				fmt.Println("timeout")
				t.Fail()
			}

			require.Equal(t, c.expectedSegmentProcessed, receivedSegmentProcessed)
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

type testNextHandler struct {
}

func (t *testNextHandler) ProcessBlock(blk *pbbstream.Block, obj interface{}) (err error) {
	return nil
}
