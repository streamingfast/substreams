package pipeline

import (
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type blockID string
type moduleName string

type outputCache struct {
	currentBlockRange *blockRange
	cache             map[blockID]*bstream.Block
	stores            dstore.Store
}

type ModuleOutputCache struct {
	moduleCaches map[moduleName]*outputCache
}

func NewModuleOutputCache(modules []*pbsubstreams.Module) *ModuleOutputCache {
	//todo: init cache with module list
	return &ModuleOutputCache{}
}

func (c *ModuleOutputCache) update(blockRef bstream.BlockRef) {

	for _, cache := range c.moduleCaches {
		if cache.currentBlockRange == nil {
			//todo: find the closest start block relative to blockRef
			// maybe we should save a state file in each folder with the cache block size
			// for now we consider that all cache blocks file wild contains 100 block.
			sb := uint64(0) //todo ^^
			c.loadBlocks(sb)
			return
		}

		if !cache.currentBlockRange.contains(blockRef) {
			c.saveBlocks()
			//todo: clean cache
			c.loadBlocks(cache.currentBlockRange.exclusiveEndBlock)
		}

	}

}

//todo: filename should look like ...
// {padded-INCLUSIVE-start-block-num}-{padded-EXCLUSIVE-end-block-num}
// ex: 0001000000-001001000.dbin

func (c *ModuleOutputCache) saveBlocks() {
	//todo: state/kv storage path should look like .../{module_name}-{module_hash}/state
	//todo: output cache storage path should look like .../{module_name}-{module_hash}/outputs

	//todo: for each module
	//  use dbin to merge and store blocks in file
}

func (c *ModuleOutputCache) loadBlocks(inclusiveStartBlockNum uint64) {

	//todo: to find the file...
	// pad the inclusiveStartBlockNum and use it has prefix in a dstore walk func.
	// if more then 1 file found use the one with the highest end block num
	// ex: 0001000000-001001000.dbin
	// ex: 0001000000-001010000.dbin <-- use this one

	//todo: state/kv storage path should look like .../{module_name}-{module_hash}/state
	//todo: find merged block file and load cache map for each modules
	//todo: output cache storage path should look like .../{module_name}-{module_hash}/outputs
	// look at dbin code to load the merged file.
	// use block id as cache key and bstream.Block has value
}

func (c *ModuleOutputCache) set(moduleName string, blockRef bstream.BlockRef, data []byte) {
}

func (c *ModuleOutputCache) get(moduleName string, blockRef bstream.BlockRef) ([]byte, bool) {
	return nil, false
}

type blockRange struct {
	startBlock        uint64
	exclusiveEndBlock uint64
}

func (r *blockRange) contains(blockRef bstream.BlockRef) bool {
	//todo: end block are always exclusive.
	panic("todo")
}
