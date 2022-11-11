package integration

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/streamingfast/bstream/forkable"

	"github.com/streamingfast/bstream"
	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
)

type BlockCursor struct {
	blockNum uint64
	blockID  string
	step     bstream.StepType
}

func NewBlockCursor(blockID string, step bstream.StepType) *BlockCursor {
	var blockNum uint64
	lastItem := strings.Split(blockID, "")
	prefix := lastItem[len(lastItem)-1]
	blkNum, err := strconv.Atoi(strings.TrimSuffix(blockID, prefix))
	if err != nil {
		panic(fmt.Sprintf("block id invalid %s", blockID))
	}
	blockNum = uint64(blkNum)
	return &BlockCursor{
		blockID:  blockID,
		blockNum: blockNum,
		step:     step,
	}
}

type TestBlockGenerator interface {
	Generate() []*GeneratedBlock
}

type ForkBlockRef struct {
	previousID  string
	libBlockRef bstream.BlockRef
	blockRef    bstream.BlockRef
}

type ForkBlockGenerator struct {
	forkBlockRefs []*ForkBlockRef
	initialLIB    bstream.BlockRef
}

func (g *ForkBlockGenerator) Generate() []*GeneratedBlock {
	var generatedBlocks []*GeneratedBlock
	options := []forkable.Option{
		forkable.HoldBlocksUntilLIB(),
		forkable.WithWarnOnUnlinkableBlocks(100),
	}
	options = append(options, forkable.WithInclusiveLIB(g.initialLIB))

	forquable := forkable.New(bstream.HandlerFunc(func(blk *bstream.Block, obj interface{}) error {
		forkableObject := obj.(*forkable.ForkableObject)
		generatedBlocks = append(generatedBlocks, &GeneratedBlock{
			block: blk,
			obj: &Obj{
				cursor: forkableObject.Cursor(),
				step:   forkableObject.Step(),
			},
		})
		return nil
	}), options...)

	for _, f := range g.forkBlockRefs {
		block := &pbsubstreamstest.Block{
			Id:     f.blockRef.ID(),
			Number: f.blockRef.Num(),
		}
		bytesBlock, err := proto.Marshal(block)
		if err != nil {
			panic("bad block")
		}
		bsBlock := &bstream.Block{
			Id:         block.Id,
			Number:     block.Number,
			PreviousId: f.previousID,
			Timestamp:  time.Now(),
			LibNum:     f.libBlockRef.Num(),
		}
		bsBlock, err = bstream.MemoryBlockPayloadSetter(bsBlock, bytesBlock)
		if err != nil {
			panic("block bytes not good")
		}
		err = forquable.ProcessBlock(bsBlock, nil)
		if err != nil {
			panic(fmt.Sprintf("processing block: %q", bsBlock.String()))
		}
	}
	return generatedBlocks
}

// 1    2    3    4    5
//   ,- E ,- H
// A <- B <- C <- D <- I
//   `- F <- G
//func (g *ForkBlockGenerator) GenerateProto() []*GeneratedBlock {
//	var generatedBlocks []*GeneratedBlock
//	forkDB := forkable.NewForkDB()
//	forkDB.InitLIB(g.initialLIB)
//	var lastBlockAdded bstream.BlockRef
//	for _, f := range g.forkBlockRefs {
//		exists := forkDB.AddLink(f.blockRef, f.previousID, f)
//		if exists {
//			continue
//		}
//
//		if lastBlockAdded == nil || lastBlockAdded.Num() < f.blockRef.Num() {
//			if lastBlockAdded != nil {
//				undo, redo := forkDB.ChainSwitchSegments(lastBlockAdded.ID(), f.previousID)
//				for _, u := range undo {
//					b := forkDB.BlockForID(u)
//					headBlock := b.AsRef() // FIXME: find correct value
//					generatedBlocks = append(generatedBlocks, newGeneratedTestBlock(headBlock, b.AsRef(), bstream.StepUndo))
//				}
//				for _, r := range redo {
//					b := forkDB.BlockForID(r)
//					headBlock := b.AsRef() // FIXME: find correct value
//					generatedBlocks = append(generatedBlocks, newGeneratedTestBlock(headBlock, b.AsRef(), bstream.StepNew))
//				}
//			}
//
//			generatedBlocks = append(generatedBlocks, newGeneratedTestBlock(
//				f.blockRef, f.blockRef, bstream.StepNew),
//			)
//			lastBlockAdded = f.blockRef
//
//		}
//		hasNew, irreversibleSegment, stalledBlocks := forkDB.HasNewIrreversibleSegment(f.libBlockRef)
//		forkDB.MoveLIB(f.libBlockRef)
//		if hasNew {
//			for _, i := range irreversibleSegment {
//				b := forkDB.BlockForID(i.AsRef().ID())
//				headBlock := b.AsRef() // FIXME: find correct value
//				generatedBlocks = append(generatedBlocks, newGeneratedTestBlock(headBlock, b.AsRef(), bstream.StepIrreversible))
//			}
//		}
//
//		for _, s := range stalledBlocks {
//			b := forkDB.BlockForID(s.AsRef().ID())
//			headBlock := b.AsRef() // FIXME: find correct value
//			generatedBlocks = append(generatedBlocks, newGeneratedTestBlock(headBlock, b.AsRef(), bstream.StepStalled))
//		}
//	}
//
//	return generatedBlocks
//}

type GeneratedBlock struct {
	block *bstream.Block
	obj   *Obj
}

type LinearBlockGenerator struct {
	startBlock         uint64
	inclusiveStopBlock uint64
}

func (g LinearBlockGenerator) Generate() []*GeneratedBlock {
	var generatedBlocks []*GeneratedBlock
	for i := g.startBlock; i <= g.inclusiveStopBlock; i++ {
		libNum := i - 1
		if i == 0 {
			libNum = 0
		}
		blockLIBRef := bstream.NewBlockRef("block-"+strconv.FormatUint(libNum, 10), libNum)
		blockRef := bstream.NewBlockRef("block-"+strconv.FormatUint(i, 10), i)
		block := &pbsubstreamstest.Block{
			Id:     blockRef.ID(),
			Number: blockRef.Num(),
		}
		bytesBlock, err := proto.Marshal(block)
		if err != nil {
			panic("bad block")
		}
		bsBlock := &bstream.Block{
			Id:         block.Id,
			Number:     block.Number,
			PreviousId: "",
			Timestamp:  time.Now(),
			LibNum:     blockLIBRef.Num(),
		}
		bsBlock, err = bstream.MemoryBlockPayloadSetter(bsBlock, bytesBlock)
		generatedBlocks = append(generatedBlocks, &GeneratedBlock{
			block: bsBlock,
			obj: &Obj{
				cursor: &bstream.Cursor{
					Step:      bstream.StepNewIrreversible,
					Block:     blockRef,
					LIB:       blockLIBRef,
					HeadBlock: blockRef,
				},
				step: bstream.StepNewIrreversible,
			},
		})
	}
	return generatedBlocks
}

type BlockGeneratorFactory func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator
