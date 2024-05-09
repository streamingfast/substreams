package index

import (
	"fmt"

	"github.com/RoaringBitmap/roaring/roaring64"
	pbindex "github.com/streamingfast/substreams/pb/sf/substreams/index/v1"
	"github.com/streamingfast/substreams/sqe"
	"google.golang.org/protobuf/proto"
)

func NewBlockIndex(expression sqe.Expression, indexModule string, bitmap *roaring64.Bitmap) *BlockIndex {
	return &BlockIndex{
		expression:  expression,
		IndexModule: indexModule,
		bitmap:      bitmap,
	}
}

type BlockIndex struct {
	expression  sqe.Expression // applied on-the-fly, from the block index module outputs
	IndexModule string
	bitmap      *roaring64.Bitmap // pre-applied
}

func (bi *BlockIndex) ExcludesAllBlocks() bool {
	return bi != nil && bi.bitmap != nil && bi.bitmap.IsEmpty()
}

func (bi *BlockIndex) Precomputed() bool {
	return bi.bitmap != nil
}

func (bi *BlockIndex) Skip(blk uint64) bool {
	return bi.bitmap != nil && !bi.bitmap.Contains(blk)
}

func (bi *BlockIndex) SkipFromKeys(indexedKeys []byte) bool {
	keys := &pbindex.Keys{}
	if err := proto.Unmarshal(indexedKeys, keys); err != nil {
		panic(fmt.Errorf("unmarshalling keys: %w", err))
	}
	return !sqe.KeysApply(bi.expression, sqe.NewFromIndexKeys(keys))
}
