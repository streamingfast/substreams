package app

import (
	"bytes"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBurstFromLIB(t *testing.T) {
	t.Run("hub without a head", func(t *testing.T) {
		assert.Equal(t, int64(2), burstFromLIB(nil))
	})

	t.Run("ready hub", func(t *testing.T) {
		store := dstore.NewMockStore(nil)
		for _, blk := range []*pbbstream.Block{
			bstream.TestBlockWithLIBNum("00000003", "00000002", 2),
			bstream.TestBlockWithLIBNum("00000004", "00000003", 2),
			bstream.TestBlockWithLIBNum("00000005", "00000004", 2),
			bstream.TestBlockWithLIBNum("00000008", "00000005", 3),
			bstream.TestBlockWithLIBNum("00000009", "00000008", 3),
		} {
			buffer := bytes.NewBuffer(nil)
			writer, err := bstream.NewDBinBlockWriter(buffer)
			require.NoError(t, err)
			require.NoError(t, writer.Write(blk))
			store.SetFile(bstream.BlockFileName(blk), buffer.Bytes())
		}

		forkableHub := hub.NewForkableHub(bstream.NewTestSourceFactory().NewSource, 0, store)
		go forkableHub.Run()
		defer forkableHub.Shutdown(nil)

		select {
		case <-forkableHub.Ready:
		case <-time.After(5 * time.Second):
			require.Fail(t, "hub never became ready")
		}

		_, _, _, libNum, err := forkableHub.HeadInfo()
		require.NoError(t, err)
		require.Equal(t, uint64(3), libNum)

		assert.Equal(t, int64(-3), burstFromLIB(forkableHub))
	})
}
