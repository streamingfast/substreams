package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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

type warningsConfig struct {
	Indent              string
	DisableImageWarning bool
}

func warnIncompletePackage(spkg *pbsubstreams.Package, config warningsConfig) (warned bool) {
	var warnings []string

	if len(spkg.PackageMeta) > 0 {
		if spkg.PackageMeta[0].Doc == "" {
			warnings = append(warnings, "README (package.doc) not found")
		}

		if spkg.PackageMeta[0].Url == "" {
			warnings = append(warnings, "URL (package.url) is not set")
		}

		if spkg.PackageMeta[0].Description == "" {
			warnings = append(warnings, "Description (package.description) is not set")
		}
	}

	if spkg.Image == nil && !config.DisableImageWarning {
		warnings = append(warnings, "Image (package.image) is not set")
	}

	if spkg.Network == "" {
		warnings = append(warnings, "Network (network) is not set")
	}

	if len(warnings) > 0 {
		fmt.Println()
		fmt.Println(config.Indent + "⚠️ Detected Substreams Package warnings:")
		for _, warning := range warnings {
			fmt.Print(config.Indent + "   • " + warning + "\n")
		}
	}

	return len(warnings) > 0
}

func printPackageDetails(spkg *pbsubstreams.Package) {
	if len(spkg.PackageMeta) <= 0 {
		return
	}

	fmt.Println("📦 Package Details")

	meta := spkg.PackageMeta[0]

	fmt.Printf("  %s %s\n", cli.PurpleStyle.Render("Name:"), meta.Name)
	fmt.Printf("  %s %s\n", cli.PurpleStyle.Render("Version:"), meta.Version)

	if meta.Description != "" {
		fmt.Printf("  %s %s\n", cli.PurpleStyle.Render("Description:"), meta.Description)
	}

	if meta.Url != "" {
		fmt.Printf("  %s %s\n", cli.PurpleStyle.Render("URL:"), meta.Url)
	}

	if spkg.Network != "" {
		fmt.Printf("  %s %s\n", cli.PurpleStyle.Render("Network:"), spkg.Network)
	}

	fmt.Println()
}
