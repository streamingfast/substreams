package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/state"
)

var squasherCreateCmd = &cobra.Command{
	Use:  "create <module_store_dsn>",
	Args: cobra.ExactArgs(1),
	RunE: createSquahserMetaE,
}

func init() {
	squasherCmd.AddCommand(squasherCreateCmd)
}

func createSquahserMetaE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	store, err := dstore.NewStore(args[0], "", "", false)
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}

	var highestKVFileInfo *state.FileInfo
	var highestKVFileName string
	var highestKVSavedBlock uint64

	err = store.Walk(ctx, "", "", func(filename string) (err error) {
		if !strings.HasSuffix(filename, ".kv") {
			return nil
		}

		fileinfo, ok := state.ParseFileName(filename)
		if !ok {
			return fmt.Errorf("could not parse filename %s", filename)
		}

		if highestKVFileInfo == nil || fileinfo.EndBlock > highestKVFileInfo.EndBlock {
			highestKVFileInfo = fileinfo
			highestKVSavedBlock = fileinfo.EndBlock
			highestKVFileName = filename
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking files: %w", err)
	}

	panic("fix this: remove hard coded value: 10_000")
	meta := state.Info{
		LastKVFile:        highestKVFileName,
		LastKVSavedBlock:  highestKVSavedBlock,
		RangeIntervalSize: 10_000,
	}

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("could not marshal json: %w", err)
	}

	metaFilename := "squasher-metadata.json"
	err = store.WriteObject(ctx, metaFilename, bytes.NewReader(metaBytes))
	if err != nil {
		return fmt.Errorf("writing metadata file %s: %w", metaFilename, err)
	}

	return nil
}
