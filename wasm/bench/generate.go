package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/dgrpc"
	pbfirehose "github.com/streamingfast/pbgo/sf/firehose/v2"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
)

//go:generate go run . mainnet.eth.streamingfast.io:443 16021772 testdata/ethereum_mainnet_block_16021772.binpb
//go:generate go run . mainnet.sol.streamingfast.io:443 396976773 testdata/solana_mainnet_block_396976773.binpb
//go:generate go run . bnb.streamingfast.io:443 78347457 testdata/bnb_mainnet_block_78347457.binpb
//go:generate go run . polygon.streamingfast.io:443 82340855 testdata/polygon_mainnet_block_82340855.binpb

// usage: SUBSTREAMS_API_TOKEN=<token> go run main.go <endpoint> <block_number> <output>
func main() {
	jwt := os.Getenv("SUBSTREAMS_API_TOKEN")
	cli.Ensure(jwt != "", "SUBSTREAMS_API_TOKEN env variable must be set")

	endpoint := os.Args[1]
	blockNum, err := strconv.ParseUint(os.Args[2], 0, 64)
	cli.NoError(err, "unable to parse <block_number>")
	output := os.Args[3]

	fmt.Printf("Fetching block %d (at %s)\n", blockNum, endpoint)

	conn, err := dgrpc.NewExternalClient(endpoint, grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: jwt})}))
	cli.NoError(err, "unable to create external gRPC client")
	defer conn.Close()

	client := pbfirehose.NewFetchClient(conn)
	response, err := client.Block(context.Background(), &pbfirehose.SingleBlockRequest{
		Reference: &pbfirehose.SingleBlockRequest_BlockNumber_{BlockNumber: &pbfirehose.SingleBlockRequest_BlockNumber{Num: blockNum}},
	})
	cli.NoError(err, "fetching block %d", blockNum)

	err = os.WriteFile(output, response.Block.Value, os.ModePerm)
	cli.NoError(err, "write file")

	fmt.Println("Wrote", output)
}
