package app

import (
	"context"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/bstream/blockstream"
	"github.com/streamingfast/bstream/hub"
	pbbstream "github.com/streamingfast/bstream/pb/sf/bstream/v1"
	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
)

// Unlinkable live blocks in a row that mean the hub is wedged for good. Resets on
// any linkable block and only armed once ready; same value the relayer uses.
const maxConsecutiveUnlinkableBlocks = 5

// LiveHubConfig configures NewLiveHub.
type LiveHubConfig struct {
	BlockStreamAddr        string
	MergedBlocksBundleSize uint64
	OneBlocksStore         dstore.Store

	// Requester names the hub to the relayer.
	Requester string
	Logger    *zap.Logger

	// OnBlock, when set, is called with every block of the live source before the
	// hub processes it, typically to update head metrics.
	OnBlock func(blk *pbbstream.Block)
}

// HubKeepFinalBlocks is how many final blocks a hub keeps below its LIB: at least
// two merged-blocks files worth, so the joining source can hand off from a file
// boundary.
func HubKeepFinalBlocks(mergedBlocksBundleSize uint64) int {
	return int(max(200, 2*mergedBlocksBundleSize))
}

// NewLiveHub builds the forkable hub tier1 streams live blocks from, fed by the
// relayer at BlockStreamAddr, partial blocks included. The caller runs it.
//
// Several apps of one process can share it, tier1 receiving it through
// Tier1Modules.ForkableHub, so that the live blocks are held in memory once.
func NewLiveHub(conf LiveHubConfig) *hub.ForkableHub {
	var forkableHub *hub.ForkableHub

	liveSourceFactory := bstream.SourceFactory(func(h bstream.Handler) bstream.Source {
		return blockstream.NewSource(
			context.Background(),
			conf.BlockStreamAddr,
			2,
			bstream.HandlerFunc(func(blk *pbbstream.Block, obj any) error {
				if conf.OnBlock != nil {
					conf.OnBlock(blk)
				}
				return h.ProcessBlock(blk, obj)
			}),
			blockstream.WithRequester(conf.Requester),
			blockstream.WithPartialBlocks(),
			blockstream.WithBurstFunc(func() int64 { return burstFromLIB(forkableHub) }),
		)
	})

	forkableHub = hub.NewForkableHubWithOptions(
		liveSourceFactory,
		HubKeepFinalBlocks(conf.MergedBlocksBundleSize),
		conf.OneBlocksStore,
		[]hub.Option{
			hub.WithLogger(conf.Logger),
			hub.WithMaxConsecutiveUnlinkableBlocks(maxConsecutiveUnlinkableBlocks),
		},
	)
	return forkableHub
}

// burstFromLIB is the burst the hub's live source asks the relayer for: every block from
// the hub's LIB onward, forks included, so the gap left by a disconnect is filled from the
// relayer's memory instead of from the one-block store. When the LIB is older than what
// the relayer holds, the relayer starts at its lowest block and the hub fills the rest from
// the one-block store. A hub without a head yet asks for the last 2 blocks.
func burstFromLIB(h *hub.ForkableHub) int64 {
	_, _, _, libNum, err := h.HeadInfo()
	// a burst of -1 means "from the relayer's LIB", -N (N > 1) means "from block N"
	if err != nil || libNum < 2 {
		return 2
	}
	return -int64(libNum)
}
