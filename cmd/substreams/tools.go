package main

import (
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/tools"
	"go.uber.org/zap/zapcore"
)

func init() {
	tools.Cmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		Setup(cmd, zapcore.InfoLevel)
	}
	rootCmd.AddCommand(tools.Cmd)
}
