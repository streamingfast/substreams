package tools

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/state"
)

var checkCmd = &cobra.Command{
	Use:   "check <store_url>",
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

	stateStore := state.Store{
		Store: store,
	}

	snapshots, err := stateStore.ListSnapshots(ctx)
	if err != nil {
		return fmt.Errorf("listing snapshots: %w", err)
	}

	var prevRange *block.Range
	for _, curRange := range snapshots.Partials {
		if prevRange == nil {
			prevRange = curRange
			continue
		}

		if curRange.StartBlock != prevRange.ExclusiveEndBlock {
			return fmt.Errorf("**hole found** between %d and %d", prevRange.ExclusiveEndBlock, curRange.ExclusiveEndBlock)
		}

		prevRange = curRange
	}

	return err
}
