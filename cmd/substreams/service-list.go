package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	cli "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	"github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1/pbsinksvcconnect"
)

func init() {
	serviceCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Get list of deployed services",
	Long: cli.Dedent(`
        Sends a "List" request to a server. By default, it will talk to a local "substreams alpha service serve" instance.
        It returns the id and the status of the substreams.
		`),
	RunE:         listE,
	Args:         cobra.ExactArgs(0),
	SilenceUsage: true,
}

func addHeaders(cmd *cobra.Command, req connect.AnyRequest) error {
	envVar := sflags.MustGetString(cmd, "substreams-api-token-envvar")
	if value := os.Getenv(envVar); value != "" {
		req.Header().Add("authorization", fmt.Sprintf("bearer %s", value))
	}

	for _, header := range sflags.MustGetStringSlice(cmd, "header") {
		parts := strings.Split(header, ": ")
		if len(parts) != 2 {
			return fmt.Errorf("invalid value for header: %s. Only one occurence of ': ' is permitted", header)
		}
		req.Header().Add(parts[0], parts[1])
	}
	return nil
}

func listE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	req := connect.NewRequest(&pbsinksvc.ListRequest{})
	if err := addHeaders(cmd, req); err != nil {
		return err
	}

	resp, err := cli.List(ctx, req)
	if err != nil {
		return interceptConnectionError(err)
	}

	if len(resp.Msg.Deployments) == 0 {
		fmt.Printf("No deployments found.\n")
		return nil
	}

	fmt.Printf("List of deployments:\n")
	for _, v := range resp.Msg.Deployments {
		fmt.Printf("  - %s (%s-%s): %s (%s)\n", v.Id, v.PackageInfo.Name, v.PackageInfo.Version, v.Status.String(), v.Reason)
	}

	return nil
}
