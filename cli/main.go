package cli

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func Main() {
	setup()

	cobra.OnInitialize(func() {
		autoBind(rootCmd, "SUBSTREAMS")
	})

	err := rootCmd.Execute()
	if err != nil {
		zlog.Error("running cmd", zap.Error(err))
	}
}

var (
	pprofListenAddr = "localhost:6060"
)

const (
	defaultRPCURL        = "http://localhost:8546"
	defaultStateStore    = "./localdata"
	defaultBlocksStore   = "./localblocks"
	defaultIrrIndexStore = "./localirr"
	defaultStartBlock    = 6810700
	genesisBlock         = 6810700
)

func init() {
	rootCmd.PersistentFlags().String("rpc-endpoint", defaultRPCURL, "RPC endpoint of blockchain node")
	rootCmd.PersistentFlags().String("state-store-url", defaultStateStore, "URL of state store")
	rootCmd.PersistentFlags().String("blocks-store-url", defaultBlocksStore, "URL of blocks store")
	rootCmd.PersistentFlags().String("irr-indexes-url", defaultIrrIndexStore, "URL of blocks store")

	rootCmd.PersistentFlags().Int64P("start-block", "s", defaultStartBlock, "Start block for blockchain firehose")
}
