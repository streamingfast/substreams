package pipeline

import (
	"fmt"
	"os"
	"testing"

	pbeth "github.com/streamingfast/firehose-ethereum/types/pb/sf/ethereum/type/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const ethBlockType = "sf.ethereum.type.v2.Block"

var ethPartialFixtures = []string{
	"../wasm/bench/testdata/ethereum_mainnet_block_16021772.binpb",
	"../wasm/bench/testdata/polygon_mainnet_block_82340855.binpb",
	"../wasm/bench/testdata/bnb_mainnet_block_78347457.binpb",
}

func loadEthFixture(tb testing.TB, path string) ([]byte, int) {
	tb.Helper()
	data, err := os.ReadFile(path)
	require.NoError(tb, err)
	block := &pbeth.Block{}
	require.NoError(tb, proto.Unmarshal(data, block))
	return data, len(block.TransactionTraces)
}

func newPartialExecOutput(tb testing.TB, blockData []byte) execout.ExecutionOutput {
	tb.Helper()
	buf, err := execout.NewBuffer(ethBlockType, nil, &pbsubstreams.Clock{})
	require.NoError(tb, err)
	require.NoError(tb, buf.Set(ethBlockType, blockData))
	return buf
}

// BenchmarkSplitPartialBlockEthereum simulates the partial-block flow on real
// chain blocks: the first partial (nothing sent yet) and a later partial where
// half of the transactions were already sent.
func BenchmarkSplitPartialBlockEthereum(b *testing.B) {
	for _, path := range ethPartialFixtures {
		data, trxCount := loadEthFixture(b, path)

		_, halfHash, err := splitPartialblock(newPartialExecOutput(b, data), ethBlockType, 0, nil)
		require.NoError(b, err)
		// Hash of the first trxCount/2 transactions, computed from a block truncated to that point.
		truncated := &pbeth.Block{}
		require.NoError(b, proto.Unmarshal(data, truncated))
		truncated.TransactionTraces = truncated.TransactionTraces[:trxCount/2]
		truncatedData, err := proto.Marshal(truncated)
		require.NoError(b, err)
		_, halfHash, err = splitPartialblock(newPartialExecOutput(b, truncatedData), ethBlockType, 0, nil)
		require.NoError(b, err)

		cases := []struct {
			name      string
			prevCount int
			prevHash  []byte
		}{
			{"first", 0, nil},
			{"half_sent", trxCount / 2, halfHash},
		}
		for _, c := range cases {
			b.Run(fmt.Sprintf("%s/trx=%d/%s", fixtureName(path), trxCount, c.name), func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				for b.Loop() {
					out := newPartialExecOutput(b, data)
					if _, _, err := splitPartialblock(out, ethBlockType, c.prevCount, c.prevHash); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func fixtureName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			path = path[i+1:]
			break
		}
	}
	for i := 0; i < len(path); i++ {
		if path[i] == '_' {
			return path[:i]
		}
	}
	return path
}
