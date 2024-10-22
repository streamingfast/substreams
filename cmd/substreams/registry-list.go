package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"go.uber.org/zap"
)

var registryListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List available packages in the Substreams registry",
	SilenceUsage: true,
	Example: cli.Dedent(`
		# List all packages in the registry
		substreams registry list

		# Display only featured packages
		substreams registry list --featured

		# Display only featured solana packages
		substreams registry list --featured --search-filter=solana
	`),
	RunE: runRegistryListE,
}

func init() {
	registryListCmd.Flags().String("registry", "https://api.substreams.dev", "Substreams registry URL")
	registryListCmd.Flags().String("search-filter", "", "Filter packages by organization, packages name or slug")
	registryListCmd.Flags().Bool("featured", false, "List only featured packages")

	registryCmd.AddCommand(registryListCmd)
}

func runRegistryListE(cmd *cobra.Command, args []string) error {
	registryURL, err := cmd.Flags().GetString("registry")
	if err != nil {
		return fmt.Errorf("could not get registry URL: %w", err)
	}

	searchFilter, err := cmd.Flags().GetString("search-filter")
	if err != nil {
		return fmt.Errorf("could not get search filter: %w", err)
	}

	featured, err := cmd.Flags().GetBool("featured")
	if err != nil {
		return fmt.Errorf("could not get featured flag: %w", err)
	}

	listPackagesEndpoint := fmt.Sprintf("%s/sf.substreams.dev.Api/Packages", registryURL)
	zlog.Debug("listing packages", zap.String("registry_url", listPackagesEndpoint))

	request := &listRequest{
		Featured:     featured,
		SearchFilter: searchFilter,
	}
	jsonRequest, _ := json.Marshal(request)
	requestBody := bytes.NewBuffer(jsonRequest)

	req, err := http.NewRequest(http.MethodPost, listPackagesEndpoint, requestBody)
	if err != nil {
		return fmt.Errorf("could not create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not perform http request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("failed to list packages: %s", res.Status)
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("could not read response body: %w", err)
	}

	fmt.Println(string(resBody))

	return nil
}

type listRequest struct {
	Featured     bool   `json:"featured"`
	SearchFilter string `json:"search_filter"`
}
