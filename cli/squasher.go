package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/abourget/llerrgroup"
	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/state"
	"go.uber.org/zap"
)

var squasherCmd = &cobra.Command{
	Use:  "squasher [block_range_file] [base_store_dsn]",
	Args: cobra.ExactArgs(2),
	RunE: runSquashE,
}

func init() {
	rootCmd.AddCommand(squasherCmd)
}

type blockRange struct {
	StartBlock uint64 `json:"start_block"`
	EndBlock   uint64 `json:"end_block"`
}

type storeRange struct {
	StoreName   string       `json:"store_name"`
	BlockRanges []blockRange `json:"block_ranges"`
}

type Squasher struct {
	storeRanges []storeRange
}

func NewSquasher(scheduleFile string) (*Squasher, error) {
	schedulerBytes, err := os.ReadFile(scheduleFile)
	if err != nil {
		return nil, fmt.Errorf("reading schedule file %s: %w", scheduleFile, err)
	}

	var ranges []storeRange
	err = json.Unmarshal(schedulerBytes, &ranges)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling json: %w", err)
	}

	return &Squasher{storeRanges: ranges}, nil
}

func (s *Squasher) run(ctx context.Context, baseStore dstore.Store) error {
	eg := llerrgroup.New(len(s.storeRanges))

	for _, sr := range s.storeRanges {
		if eg.Stop() {
			continue
		}

		sr := sr
		eg.Go(func() error {
			sort.Slice(sr.BlockRanges, func(i, j int) bool {
				return sr.BlockRanges[i].EndBlock < sr.BlockRanges[j].EndBlock
			})

			for _, blockRange := range sr.BlockRanges {
				zlog.Info("looking for file", zap.String("prefix", state.FilePrefix(sr.StoreName, blockRange.EndBlock)))

				var filename string
				var fullKVFound bool

			FileFound: //loop waiting for file with our given end block to exist
				for {
					files, err := baseStore.ListFiles(ctx, state.FilePrefix(sr.StoreName, blockRange.EndBlock), "", 2) // max=2 because we might have a partial AND a full kv for this prefix
					if err != nil {
						return fmt.Errorf("listing files: %w", err)
					}

					switch len(files) {
					case 0:
						zlog.Info("file not found. sleeping", zap.String("file prefix", state.FilePrefix(sr.StoreName, blockRange.EndBlock)))
						time.Sleep(5 * time.Second)
						continue
					case 1:
						filename = files[0]
						_, _, _, partial := state.ParseFileName(filename)
						if !partial {
							fullKVFound = true
							zlog.Info("found full kv file for this range already", zap.String("filename", filename))
						}
						break FileFound
					case 2:
						var fullFound bool
						for _, f := range files {
							_, _, _, partial := state.ParseFileName(f)
							filename = f
							if !partial {
								fullFound = true
								break
							}
						}
						if fullFound {
							fullKVFound = true
							zlog.Info("found full kv file for this range already", zap.String("filename", filename))
						}
						break FileFound
					}
				}

				if fullKVFound {
					continue
				}

				builder, err := state.BuilderFromFile(ctx, filename, baseStore)
				if err != nil {
					return fmt.Errorf("loading builder from file %s: %w", filename, err)
				}

				zlog.Info("squashing",
					zap.String("store", sr.StoreName),
					zap.Uint64("up_to_block", blockRange.EndBlock),
				)

				err = builder.Squash(ctx, baseStore, blockRange.EndBlock)
				if err != nil {
					return fmt.Errorf("squashing: %w", err)
				}

				_, err = builder.WriteState(ctx, blockRange.EndBlock, false)
				if err != nil {
					return fmt.Errorf("writing state to store: %w", err)
				}
			}

			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return fmt.Errorf("running scheduler: %w", err)
	}

	return nil
}

func runSquashE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	blockRangesFile := args[0]

	squasher, err := NewSquasher(blockRangesFile)
	if err != nil {
		return err
	}

	store, err := dstore.NewStore(args[1], "", "", false)
	if err != nil {
		return err
	}

	err = squasher.run(ctx, store)
	if err != nil {
		return err
	}
	return nil
}
