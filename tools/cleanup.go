package tools

import (
	"fmt"

	"github.com/abourget/llerrgroup"
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
		fileinfo, ok := state.ParseFileName(filename)
		if !ok {
			return nil
		}

		if fileinfo.Partial {
			partialFiles[fileinfo.EndBlock] = filename
		}

		if !fileinfo.Partial && fileinfo.EndBlock > highestKVBlock {
			highestKVBlock = fileinfo.EndBlock
			return nil
		}

		return nil
	})

	if len(partialFiles) == 0 {
		zlog.Info("no partial files found")
		return nil
	}

	eg := llerrgroup.New(len(partialFiles))

	for endBlock, filename := range partialFiles {
		if eg.Stop() {
			continue
		}

		eb := endBlock
		fn := filename

		eg.Go(func() error {
			if eb > highestKVBlock {
				return nil
			}

			err := store.DeleteObject(ctx, fn)
			if err != nil {
				zlog.Warn("error deleting file", zap.String("filename", fn), zap.String("store", dsn))
			}

			return nil
		})
	}

	err = eg.Wait()
	if err != nil {
		return fmt.Errorf("running deletes: %w", err)
	}

	return nil
}
