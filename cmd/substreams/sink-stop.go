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
	server "github.com/streamingfast/substreams/sink-server"
)

func init() {
	alphaCmd.AddCommand(sinkStopCmd)
	sinkStopCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
	sinkStopCmd.Flags().Bool("strict", false, "Require deploymentID parameter to be set and complete")
}

var sinkStopCmd = &cobra.Command{
	Use:   "sink-stop [deployment-id]",
	Short: "Stop a running substreams sink",
	Long: cli.Dedent(`
        Sends an "Stop" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It will stop a substreams sink and returns information about the change of status.
        If deploymentID is not set or is incomplete, the CLI will try to guess (unless --strict is set).
		`),
	RunE:         sinkStopE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func sinkStopE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	var id string
	if len(args) == 1 {
		id = args[0]
	}

	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))
	if len(id) < server.DeploymentIDLength {
		if sflags.MustGetBool(cmd, "strict") {
			return fmt.Errorf("invalid ID provided: %q and '--strict' is set", id)
		}
		matching, err := fuzzyMatchDeployment(ctx, id, cli, fuzzyMatchPreferredStatusOrder)
		if err != nil {
			return err
		}
		id = matching.Id
	}

	req := &pbsinksvc.StopRequest{
		DeploymentId: id,
	}

	fmt.Printf("Stopping... (shutting down services, please wait)\n")
	resp, err := cli.Stop(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}
	fmt.Printf("Response for deployment %q:\n  Previous Status: %v, New Status: %v\n", id, resp.Msg.PreviousStatus, resp.Msg.NewStatus)

	return nil
}
