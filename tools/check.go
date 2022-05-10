package tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/state"
)

var checkCmd = &cobra.Command{
	Use:   "check {store_url}",
	Short: "checks the integrity of the kv files in a given store",
	Args:  cobra.ExactArgs(1),
	RunE:  checkE,
}

func init() {
	Cmd.AddCommand(checkCmd)
}

func checkE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	store, err := dstore.NewStore(args[0], "", "", false)
	if err != nil {
		return fmt.Errorf("could not create store from %s: %w", args[0], err)
	}

	panic("remove hardcode value:10_000")
	var intervalSize uint64 = 10_000 //todo: parameterize this

	var fileInfos []*state.FileInfo
	err = store.Walk(ctx, "", "", func(filename string) (err error) {
		if !strings.HasSuffix(filename, ".kv") {
			return nil
		}

		currentFileInfo, ok := state.ParseFileName(filename)
		if !ok {
			err = fmt.Errorf("could not parse filename %s", filename)
		}

		fileInfos = append(fileInfos, currentFileInfo)
		return nil
	})

	if err != nil {
		return fmt.Errorf("walking file store: %w", err)
	}

	sort.Slice(fileInfos, func(i, j int) bool {
		return fileInfos[i].EndBlock < fileInfos[j].EndBlock
	})

	var prevFileInfo *state.FileInfo
	for _, info := range fileInfos {
		if prevFileInfo == nil {
			prevFileInfo = info
			continue
		}

		if info.EndBlock-prevFileInfo.EndBlock > intervalSize {
			return fmt.Errorf("**hole found** between %d and %d", prevFileInfo.EndBlock, info.EndBlock)
		}

		prevFileInfo = info
	}

	return err
}
