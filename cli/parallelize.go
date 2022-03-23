package cli

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/manifest"
)

var parallelizeCmd = &cobra.Command{
	Use:  "parallelize [manifest] [stream_name]",
	Args: cobra.ExactArgs(2),
	RunE: runParallelizeE,
}

func init() {
	rootCmd.AddCommand(parallelizeCmd)
}

func runParallelizeE(cmd *cobra.Command, args []string) error {
	manifestPath := args[0]
	streamName := args[1]

	manif, err := manifest.New(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	stores, err := manif.Graph.StoresDownTo(streamName)
	res, err := json.Marshal(manifest.ModuleMarshaler(stores))
	if err != nil {
		return err
	}

	fmt.Println(string(res))

	return nil
}
