package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
	"time"
)

func init() {
	rootCmd.AddCommand(mergeCmd)
}

var mergeCmd = &cobra.Command{
	Use:          "merge [store dsn]",
	Short:        "merge partial files in store",
	RunE:         runMerge,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func runMerge(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	storeDSN := args[0]

	var store dstore.Store
	store, err := dstore.NewStore(storeDSN, "", "", false)
	if err != nil {
		return fmt.Errorf("initializing store: %w", err)
	}

	zlog.Info("merging partial files", zap.String("store", storeDSN))

	for {
		select {
		case <-ctx.Done():
			break
		default:
			//
		}

		partialFileCount := 0
		var partialFiles []string

		starts := map[uint64]string{}
		ends := map[uint64]string{}

		greatestKVBlock := uint64(0)
		greatestPartialBlock := uint64(0)

		_ = store.Walk(ctx, "", "", func(filename string) (err error) {
			ok, s, e, partial := state.ParseFileName(filename)
			if !ok {
				return
			}

			if partial {
				starts[s] = filename //only check start blocks of partial files, these are the ones we want to connect to kv files

				partialFiles = append(partialFiles, filename)
				partialFileCount++
				if e > greatestPartialBlock {
					greatestPartialBlock = e
				}
			} else {
				if e > greatestKVBlock {
					greatestKVBlock = e
				}
			}

			ends[e] = filename

			return nil
		})

		if greatestKVBlock >= greatestPartialBlock {
			zlog.Info("full kv file already exists which covers range of partial files. deleting all partial files.")
			for _, pf := range partialFiles {
				zlog.Debug("deleting file", zap.String("filename", pf), zap.String("store", storeDSN))
				err := store.DeleteObject(ctx, pf)
				if err != nil {
					zlog.Warn("error deleting file", zap.String("filename", pf), zap.String("store", storeDSN), zap.Error(err))
				}
			}
			zlog.Info("partial files deleted, exiting.")

			break
		}

		if partialFileCount == 0 {
			zlog.Info("no partial files found. exiting.")
			break
		}

		zlog.Info("found partial files", zap.Int("count", partialFileCount))
		zlog.Info("merging...")

	Out:
		for {
			merged := 0
		Loop:
			for start, startFile := range starts {
				zlog.Debug("looking for files with end block", zap.Uint64("value", start))
				if endFile, endExists := ends[start]; endExists {
					ok, startBlock, _, _ := state.ParseFileName(startFile)
					ok, _, endBlock, _ := state.ParseFileName(endFile)
					if !ok {
						return fmt.Errorf("parsing file name %s", endFile)
					}

					zlog.Debug("getting builder from file", zap.String("filename", startFile))
					prev, err := state.BuilderFromFile(ctx, startFile, store)
					if err != nil {
						return fmt.Errorf("parsing file %s into builder: %w", startFile, err)
					}

					zlog.Debug("getting builder from file", zap.String("filename", endFile))
					next, err := state.BuilderFromFile(ctx, endFile, store)
					if err != nil {
						return fmt.Errorf("parsing file %s into builder: %w", endFile, err)
					}

					zlog.Info(fmt.Sprintf("merging %s and %s", startFile, endFile))
					err = next.Merge(prev)
					if err != nil {
						return fmt.Errorf("merging %s and %s: %w", startFile, endFile, err)
					}

					zlog.Info("saving merged file")
					filename, err := next.WriteState(ctx, endBlock)
					if err != nil {
						return fmt.Errorf("writing merged file state at block %d: %w", endBlock, err)
					}

					zlog.Debug("deleting file", zap.String("filename", startFile))
					err = store.DeleteObject(ctx, startFile)
					if err != nil {
						zlog.Warn("error deleting file",
							zap.String("filename", startFile),
							zap.String("store", storeDSN),
							zap.Error(err),
						)
					}

					zlog.Debug("deleting file", zap.String("filename", endFile))
					err = store.DeleteObject(ctx, endFile)
					if err != nil {
						zlog.Warn("error deleting file",
							zap.String("filename", endFile),
							zap.String("store", storeDSN),
							zap.Error(err),
						)
					}

					delete(starts, start)
					delete(ends, start)

					starts[startBlock] = filename
					ends[endBlock] = filename

					merged++
					break Loop
				}
			}

			if merged == 0 {
				zlog.Info("nothing merged.")
				break Out
			}
		}

		time.Sleep(10 * time.Second)
	}

	return nil
}
