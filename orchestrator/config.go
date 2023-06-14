package orchestrator

import "github.com/streamingfast/substreams/block"

// Config lays out the configuration of the components to accomplish
// the work of the ParallelProcessor. Different conditions put different
// constraints on the output of the parallel processor.
type Config struct {
	StoresSegmenter *block.Segmenter
	MapperUpToBlock uint64

	StoresToProducePartial
	RequireFullStoresAtBlock uint64

	// Whether or not the parallel processor needs to produce
	// stores
	PrepareForLinearProcessing bool

	// Whether to process the last map stage.
	// In development mode,
	// we only care about processing the stores up to the handoff block,
	// which then kicks in the linear mode, which will then output its
	// results.
	// In production mode, we will want that mapper to be produced
	// to generate the ExecOut files, and kick off the ExecOutWalker
	// here to output the results.
	ProcessLastMapStage bool

	// Whether or not to save a Full Store snapshot to the storage.
	// This will be useful in development mode, where the user wants
	// to iterate multiple times at the same start block, without
	// needing to sync from say 1000 to 1565, wasting 565 blocks
	// of processing each time.
	// We would not save those in production mode, because the chances
	// of being reused are very low. You don't iterate in production mode.
	//
	// If the caller does not intend to start the linear processing
	// after the parallel processing (PrepareForLinearProcessing),
	// this flag is ignored, and no snapshots are saved off of
	// normal boundaries.
	BuildStores    *block.Range
	MapProduce     *block.Range
	MapRead        *block.Range
	LinearPipeline *block.Range
	// ref: /docs/assets/range_planning.png

	SnapshotFullStoresAtHandoff   bool
	ProduceOffBoundsStoreSnapshot bool
	FlushIncompleteStoreToStorage bool
	StoresSnapshotsUpToBlock      uint64 // if this is off-bound, we'd be okay
	MapperProducesFromBlock       uint64
	// but the segmenter would be need to be different, if we use
	// only the `bool`, we can ignore the scheduling of the last
	// segment if it `IsPartial(segmentIndex)`

	// TODO: how will we manage the fact that
	// each store has its own initial module, and will probably
	// want its own segmenter, to write to its files, and those
	// can start at different points in time.. not necessarily
	// aligned with the graph first initial block.
}

func BuildConfig(productionMode bool, graphInitBlock, resolvedStartBlock, linearHandoffBlock, exclusiveEndBlock uint64) *Config {
	c := &Config{}
	return c
}
