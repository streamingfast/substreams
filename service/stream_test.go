package service

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/hub"
	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRecentFinalBlockFromHub(t *testing.T) {
	// one-block files 1000 to 3110, each block final two blocks later: LIB is 3108
	oneBlocksStore := dstore.NewMockStore(nil)
	id := func(num uint64) string { return fmt.Sprintf("%08x", num) }
	for num := uint64(1000); num <= 3110; num++ {
		blk := bstream.TestBlockWithLIBNum(id(num), id(num-1), num-2)
		buf := bytes.NewBuffer(nil)
		writer, err := bstream.NewDBinBlockWriter(buf)
		require.NoError(t, err)
		require.NoError(t, writer.Write(blk))
		oneBlocksStore.SetFile(bstream.BlockFileName(blk), buf.Bytes())
	}

	cases := []struct {
		name        string
		mergedBases []uint64
		expected    uint64
	}{
		{"merger keeping up", []uint64{0, 1000, 2000}, 2000},
		{"merger behind", []uint64{0, 1000}, 1000},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mergedBlocksStore := dstore.NewMockStore(nil)
			for _, base := range c.mergedBases {
				mergedBlocksStore.SetFile(fmt.Sprintf("%010d", base), []byte("merged"))
			}

			// 1000-block files while bstream.DefaultMergedBlocksBundleSize stays at 100
			forkableHub := hub.NewForkableHubWithOptions(bstream.NewTestSourceFactory().NewSource, 0, oneBlocksStore,
				[]hub.Option{hub.WithMergedBlocksStore(mergedBlocksStore, 1000, 20000)})
			go forkableHub.Run()
			t.Cleanup(func() { forkableHub.Shutdown(nil) })
			<-forkableHub.Ready

			sf := &StreamFactory{hub: forkableHub, mergedBlocksBundleSize: 1000}
			recent, err := sf.GetRecentFinalBlock()
			require.NoError(t, err)
			assert.Equal(t, c.expected, recent)
			assert.Equal(t, forkableHub.LowestBlockNum(), recent, "the joining source hands off to the hub at its lowest block")
		})
	}
}

func TestRoundToBundleFinalBlock(t *testing.T) {
	cases := []struct {
		name                 string
		finalBlockNum        uint64
		bundleSize           uint64
		firstStreamableBlock uint64
		expected             uint64
	}{
		{"parity with previous behavior at 100", 12345, 100, 0, 12200},
		{"exact boundary at 100", 12300, 100, 0, 12200},
		{"one full bundle behind rounds to previous", 150, 100, 0, 0},
		{"bundle size 1000", 12345, 1000, 0, 11000},
		{"exact boundary at 1000", 12000, 1000, 0, 11000},
		{"one full bundle behind at 1000 rounds to previous", 1500, 1000, 0, 0},
		{"first complete bundle rounds down to first streamable block", 10150, 100, 10000, 10000},
		{"above first complete bundle with first streamable block", 10345, 100, 10000, 10200},
		{"less than one bundle final is untouched", 10050, 100, 10000, 10050},
		{"result floored at unaligned first streamable block", 10250, 100, 10150, 10150},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, roundToBundleFinalBlock(c.finalBlockNum, c.bundleSize, c.firstStreamableBlock))
		})
	}
}

func TestCapToLastMergedFile(t *testing.T) {
	cases := []struct {
		name                 string
		recentFinalBlock     uint64
		mergedUpTo           uint64
		firstStreamableBlock uint64
		expected             uint64
	}{
		{"merger keeping up", 1100, 1200, 0, 1100},
		{"merger keeping up, next file not found yet", 1200, 1200, 0, 1100},
		{"merger behind", 1100, 600, 0, 500},
		{"only the first file", 1100, 100, 0, 0},
		{"nothing merged yet", 1100, 0, 0, 0},
		{"nothing merged yet, chain starting above 0", 1100, 100, 150, 150},
		{"only the first file, chain starting above 0", 1100, 200, 150, 150},
		{"young chain", 30, 0, 0, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.expected, capToLastMergedFile(c.recentFinalBlock, c.mergedUpTo, 100, c.firstStreamableBlock))
		})
	}
}
