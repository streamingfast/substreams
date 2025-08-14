package main

import (
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
)

var verifyCmd = &cobra.Command{
	Use:   "verify [github_release_url | https_spkg_path | local_spkg_path | local_substreams_path]",
	Short: "Verify a package is ready for publishing to the Substreams.dev registry. Alias for `substreams registry verify`",
	Long: cli.Dedent(`
		Verify a package is ready for publishing to the Substreams.dev registry. This command performs
		all the validation checks that would be done during publishing, but does not actually publish
		the package.

		You can specify a GitHub release URL, HTTPS spkg path, local spkg path, or local substreams path.
		If no argument is provided, it will look for a substreams.yaml file in the current directory.
		You can use "-" to read the manifest from standard input.
	`),
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryVerify,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

