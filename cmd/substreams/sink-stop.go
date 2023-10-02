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
	alphaCmd.AddCommand(sinkStopCmd)
	sinkStopCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
}

var sinkStopCmd = &cobra.Command{
	Use:   "sink-stop <deployment-id>",
	Short: "Stop a running substreams sink",
	Long: cli.Dedent(`
        Sends an "Stop" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It will stop a substreams sink and returns information about the change of status.
		`),
	RunE:         sinkStopE,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func sinkStopE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	id := args[0]

	req := &pbsinksvc.StopRequest{
		DeploymentId: id,
	}

	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	resp, err := cli.Stop(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}
    fmt.Printf("Response for deployment %q:\n  Previous Status: %v, New Status: %v\n", id, resp.Msg.PreviousStatus, resp.Msg.NewStatus)

	return nil
}
