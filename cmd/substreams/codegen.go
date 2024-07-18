package main

import "github.com/streamingfast/substreams/codegen"

func init() {
	rootCmd.AddCommand(codegen.Cmd)
}
