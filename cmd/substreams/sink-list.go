package main

import (
	"fmt"
	"net/http"

	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
	cli "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	"github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1/pbsinksvcconnect"
)

func init() {
	alphaCmd.AddCommand(sinkListCmd)
	sinkListCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
}

var sinkListCmd = &cobra.Command{
	Use:   "sink-list",
	Short: "Get list of deployed substreams sinks",
	Long: cli.Dedent(`
        Sends a "List" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It returns the id and the status of the substreams.
		`),
	RunE:         sinkListE,
	Args:         cobra.ExactArgs(0),
	SilenceUsage: true,
}

func sinkListE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	req := &pbsinksvc.ListRequest{}

	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	resp, err := cli.List(ctx, connect.NewRequest(req))
	if err != nil {
		return err
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
