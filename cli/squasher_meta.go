package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/state"
	"strings"
)

var squasherCreateCmd = &cobra.Command{
	Use:  "create [module_store_dsn]",
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
			highestKVFileName = filename
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking files: %w", err)
	}

	meta := SquasherMetadata{
		LastKVFile: highestKVFileName,
		RangeSize:  10_000, //TODO: parameterize this
	}

	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("could not marshal json: %w", err)
	}

	metaFilename := fmt.Sprintf("%s-squasher-metadata.json", highestKVFileInfo.ModuleName)
	err = store.WriteObject(ctx, metaFilename, bytes.NewReader(metaBytes))
	if err != nil {
		return fmt.Errorf("writing metadata file %s: %w", metaFilename, err)
	}

	return nil
}
