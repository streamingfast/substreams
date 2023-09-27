package main

import (
	"fmt"
	"github.com/spf13/cobra"
	"strconv"
	"strings"
)

func readStartBlockFlag(cmd *cobra.Command, flagName string) (int64, bool, error) {
	val, err := cmd.Flags().GetString(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	if val == "" {
		return 0, true, nil
	}

	startBlock, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("start block is invalid: %w", err)
	}

	return startBlock, false, nil
}

func readStopBlockFlag(cmd *cobra.Command, startBlock int64, flagName string, withCursor bool) (uint64, error) {
	val, err := cmd.Flags().GetString(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}

	isRelative := strings.HasPrefix(val, "+")
	if isRelative {
		if withCursor {
			return 0, fmt.Errorf("relative stop block is not supported with a cursor")
		}

		if startBlock < 0 {
			return 0, fmt.Errorf("relative end block is supported only with an absolute start block")
		}

		val = strings.TrimPrefix(val, "+")
	}

	endBlock, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("end block is invalid: %w", err)
	}

	if isRelative {
		return uint64(startBlock) + endBlock, nil
	}

	return endBlock, nil
}
