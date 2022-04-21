package tools

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

var cleanUpCmd = &cobra.Command{
	Use:   "cleanup {store_url}",
	Short: "Checks for partial files which have already merged into a full KV store and purges them",
	Args:  cobra.ExactArgs(1),
	RunE:  cleanUpE,
}

func init() {
	Cmd.AddCommand(cleanUpCmd)
}

//delete all partial files which are already merged into the kv store
func cleanUpE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	var store dstore.Store

	dsn := args[0]
	store, err := dstore.NewStore(dsn, "", "", false)
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}

	highestKVBlock := uint64(0)
	partialFiles := map[uint64]string{}

	_ = store.Walk(ctx, "", "", func(filename string) (err error) {
		ok, _, end, partial := state.ParseFileName(filename)
		if !ok {
			return nil
		}

		if partial {
			partialFiles[end] = filename
		}

		if !partial && end > highestKVBlock {
			highestKVBlock = end
			return nil
		}

		return nil
	})

	for endBlock, filename := range partialFiles {
		if endBlock > highestKVBlock {
			continue
		}

		err := store.DeleteObject(ctx, filename)
		if err != nil {
			zlog.Warn("error deleting file", zap.String("filename", filename), zap.String("store", dsn))
		}
	}

	return nil
}
