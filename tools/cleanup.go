package tools

import (
	"fmt"

	"github.com/abourget/llerrgroup"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var cleanUpCmd = &cobra.Command{
	Use:   "cleanup <store_url>",
	Short: "Checks for partial files which have already merged into a full kv store and purges them",
	Args:  cobra.ExactArgs(1),
	RunE:  cleanUpE,
}

func init() {
	Cmd.AddCommand(cleanUpCmd)
}

//delete all partial files which are already merged into the kv store
func cleanUpE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	dsn := args[0]
	store, remoteStore, err := newStore(dsn)
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}

	highestKVBlock := uint64(0)
	partialFiles := map[uint64]string{}

	files, err := store.ListSnapshotFiles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	for _, file := range files {
		if file.Partial {
			partialFiles[file.EndBlock] = file.Filename
		}

		if !file.Partial && file.EndBlock > highestKVBlock {
			highestKVBlock = file.EndBlock
			return nil
		}
	}

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

			err := remoteStore.DeleteObject(ctx, fn)
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
