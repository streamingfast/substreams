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
		startRange               uint64
		endRange                 uint64
		linearHandoff            uint64
		stageToProcess           int
		errorBackProcessing      bool
		expectedSegmentProcessed []uint64
	}{
		// In those cases, from startRange to endRange, blocks are processed using a testSource.
		// In the back filler, once a block is processed above next current segment + 120, the current segment should be requested to tier2 (be cached).
		// That's why in the `sunny path` test case, only the segment 11 is processed. From linearHandOff=100, the first request is sent to tier2,
		// once block 231 > 11 (next segment) * 10 (segment size)  + 120 (finalBlockDelay) is processed.

		{
			name:                     "sunny path",
			segmentSize:              10,
			startRange:               101,
			endRange:                 231,
			linearHandoff:            100,
			errorBackProcessing:      false,
			expectedSegmentProcessed: []uint64{11},
		},
		{
			name:                     "with job failing",
			segmentSize:              10,
			startRange:               101,
			endRange:                 231,
			linearHandoff:            100,
			errorBackProcessing:      true,
			expectedSegmentProcessed: []uint64{11},
		},

		{
			name:                     "processing multiple segments",
			segmentSize:              10,
			startRange:               101,
			endRange:                 261,
			linearHandoff:            100,
			errorBackProcessing:      false,
			expectedSegmentProcessed: []uint64{11, 12, 13, 14},
		},

		{
			name:                     "big segment size",
			segmentSize:              1000,
			startRange:               101,
			endRange:                 2021,
			linearHandoff:            100,
			errorBackProcessing:      false,
			expectedSegmentProcessed: []uint64{1},
		},

		{
			name:                     "multiple big segment size",
			segmentSize:              1000,
			startRange:               101,
			endRange:                 4023,
			linearHandoff:            100,
			errorBackProcessing:      false,
			expectedSegmentProcessed: []uint64{1, 2, 3},
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

			//Start from fromBlocks, to toBlocks
			for currentBlockNum := c.startRange; currentBlockNum <= c.endRange; currentBlockNum++ {
				fmt.Println("pushing block", currentBlockNum)
				block := &pbbstream.Block{
					Number: currentBlockNum}
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
