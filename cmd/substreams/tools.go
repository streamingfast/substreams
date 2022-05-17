package main

import "github.com/streamingfast/substreams/tools"

func init() {
	rootCmd.AddCommand(tools.Cmd)
}
