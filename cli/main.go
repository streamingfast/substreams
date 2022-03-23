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

func init() {
	rootCmd.PersistentFlags().String("rpc-endpoint", "http://localhost:8546", "RPC endpoint of blockchain node")
	rootCmd.PersistentFlags().String("state-store-url", "./localdata", "URL of state store")
	rootCmd.PersistentFlags().String("blocks-store-url", "./localblocks", "URL of blocks store")
	rootCmd.PersistentFlags().String("irr-indexes-url", "./localirr", "URL of blocks store")

	rootCmd.PersistentFlags().Int64P("start-block", "s", 0, "Start block for blockchain firehose")
	rootCmd.PersistentFlags().Int64P("stop-block", "t", 0, "Stop block for blockchain firehose")
	rootCmd.PersistentFlags().BoolP("partial", "p", false, "Start block for blockchain firehose")
}
