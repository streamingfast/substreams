package main

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
)

func init() {
	SinkCmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		Setup(cmd, zapcore.InfoLevel)
	}
	rootCmd.AddCommand(SinkCmd)
}

var SinkCmd = &cobra.Command{Use: "sink", Short: "Launch a sink"}
