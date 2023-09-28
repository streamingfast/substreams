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
	alphaCmd.AddCommand(sinkRemoveCmd)
	sinkRemoveCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
}

var sinkRemoveCmd = &cobra.Command{
	Use:   "sink-remove <deployment-id>",
	Short: "Remove a deployed substreams sink",
	Long: cli.Dedent(`
        Sends an "Remove" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It will remove a substreams sink completely, including its data. Use "pause" instead if you want to keep the data.
		`),
	RunE:         sinkRemoveE,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func sinkRemoveE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	id := args[0]

	req := &pbsinksvc.RemoveRequest{
		DeploymentId: id,
	}

	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	resp, err := cli.Remove(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}
    fmt.Printf("Deployment %q successfully deleted.\nPrevious Status: %v\n", id, resp.Msg.PreviousStatus)

	return nil
}
