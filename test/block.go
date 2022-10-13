package pipeline

//
//import (
//	"github.com/streamingfast/bstream"
//	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
//	"strconv"
//)
//
//type LinearBlockGenerator struct {
//	startBlock         uint64
//	inclusiveStopBlock uint64
//}
//
//func (g LinearBlockGenerator) Generate() []*pbsubstreams.Block {
//	var blocks []*pbsubstreams.Block
//	for i := g.startBlock; i <= g.inclusiveStopBlock; i++ {
//		blocks = append(blocks, &pbsubstreams.Block{
//			Id:     "block-" + strconv.FormatUint(i, 10),
//			Number: i,
//			Step:   int32(bstream.StepNewIrreversible),
//		})
//	}
//	return blocks
//}
//
//type TestBlockGenerator interface {
//	Generate() []*pbsubstreams.Block
//}
