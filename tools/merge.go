package tools

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/state"
	"github.com/yourbasic/graph"
	"go.uber.org/zap"
)

var mergeCmd = &cobra.Command{
	Use:   "merge {store_url}",
	Short: "finds the largest contiguous block range possible and merges it",
	Args:  cobra.ExactArgs(1),
	RunE:  mergePartialFilesE,
}

func init() {
	Cmd.AddCommand(mergeCmd)
}

func mergePartialFilesE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	var store dstore.Store

	dsn := args[0]
	store, err := dstore.NewStore(dsn, "", "", false)
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}

	type partialFile struct {
		filename string
		endBlock uint64
	}
	var partialFiles []partialFile

	var ranges blockRangeItems

	//get all partial files
	err = store.Walk(ctx, "", "", func(filename string) error {
		ok, start, end, partial := state.ParseFileName(filename)
		if !ok {
			return fmt.Errorf("could not parse filename %s", filename)
		}

		if partial {
			partialFiles = append(partialFiles, partialFile{
				filename: filename,
				endBlock: end,
			})
		}

		bri := blockRangeItem{
			partial:  partial,
			start:    start,
			end:      end,
			filename: filename,
		}
		ranges = append(ranges, bri)

		return nil
	})

	if err != nil {
		return fmt.Errorf("walking store: %w", err)
	}

	sort.Sort(ranges)

	//reverse sort by end block
	sort.Slice(partialFiles, func(i, j int) bool {
		return partialFiles[i].endBlock > partialFiles[j].endBlock
	})

	filesListToBuildersList := func(in []string) (out []*state.Builder, err error) {
		for _, filename := range in {
			b, err := state.BuilderFromFile(ctx, filename, store)
			if err != nil {
				return nil, fmt.Errorf("parsing file %s: %w", filename, err)
			}
			out = append(out, b)
		}
		return
	}

	//merge chain
	for _, pf := range partialFiles {
		ok, files, err := contiguousFilesToTargetBlock(ranges, pf.endBlock)
		if err != nil {
			return fmt.Errorf("getting files: %w", err)
		}

		if !ok {
			continue
		}

		switch len(files) {
		case 0:
			return fmt.Errorf("something went really wrong")
		case 1:
			zlog.Info("files already merged", zap.Strings("files", files))
			return nil //seems like everything is merged into one file already
		default:
			builders, err := filesListToBuildersList(files)
			if err != nil {
				return fmt.Errorf("creating builders list: %w", err)
			}

			zlog.Info("found files to be merged", zap.Strings("files", files))

			for i := 0; i < len(builders)-1; i++ {
				prev := builders[i]
				next := builders[i+1]

				err := next.Merge(prev)
				if err != nil {
					return fmt.Errorf("merging state for %s: %w", next.Name, err)
				}
			}

			_, _, end, _ := state.ParseFileName(files[len(files)-1])
			lastMergedBuilder := builders[len(builders)-1]

			fileWritten, err := lastMergedBuilder.WriteState(ctx, end)
			if err != nil {
				return fmt.Errorf("writing file: %w", err)
			}

			zlog.Info("merge written to disk", zap.String("merged file", fileWritten))
			zlog.Info("exiting.")

			return nil
		}
	}

	zlog.Info("nothing done.")
	return nil
}

func contiguousFilesToTargetBlock(ranges blockRangeItems, targetBlock uint64) (bool, []string, error) {
	fulls := map[int]struct{}{}
	targets := map[int]struct{}{}
	for i, x := range ranges {
		if !x.partial {
			fulls[i] = struct{}{}
		}

		if x.end == targetBlock {
			targets[i] = struct{}{}
		}
	}

	if len(fulls) == 0 {
		return false, nil, nil // no files which start at the beginning
	}

	if len(targets) == 0 { // no files which reach the target block
		return false, nil, nil
	}

	var ends []int
	for _, br := range ranges {
		ends = append(ends, int(br.end))
	}

	// construct a graph with all the paths of ranges
	g := graph.New(len(ranges))
	for i, e := range ends {
		for j, br := range ranges {
			if uint64(e) == br.start {
				g.AddCost(i, j, 1)
			}
		}
	}

	//check if there is a path from any of the full snapshots (start = 0) to our target block
	var paths [][]int
	var distances []int64
	var path []int
	for t := range targets {
		for f := range fulls {
			p, d := graph.ShortestPath(g, f, t)
			if len(p) >= 0 && d >= 0 {
				paths = append(paths, p)
				distances = append(distances, d)
			}
		}
	}

	if len(paths) == 0 {
		return false, nil, nil
	}

	sort.Slice(paths, func(i, j int) bool {
		return distances[i] < distances[j]
	})

	path = paths[0]

	var pathFileNames []string
	for _, p := range path {
		pathFileNames = append(pathFileNames, ranges[p].filename)
	}

	return true, pathFileNames, nil
}

type blockRangeItem struct {
	partial bool

	start uint64
	end   uint64

	filename string
}

type blockRangeItems []blockRangeItem

func (b blockRangeItems) Len() int {
	return len(b)
}

func (b blockRangeItems) Less(i, j int) bool {
	if b[i].start == b[j].start {
		return b[i].end < b[j].end
	}
	return b[i].end < b[j].start
}

func (b blockRangeItems) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
