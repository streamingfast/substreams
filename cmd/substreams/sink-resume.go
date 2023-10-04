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
	alphaCmd.AddCommand(sinkResumeCmd)
	sinkResumeCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
	sinkResumeCmd.Flags().Bool("strict", false, "Require deploymentID parameter to be set and complete")
}

var sinkResumeCmd = &cobra.Command{
	Use:   "sink-resume [deployment-id]",
	Short: "Resume a paused substreams sink",
	Long: cli.Dedent(`
        Sends an "Resume" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It will resume a paused substreams and returns information about the change of status.
        If deploymentID is not set or is incomplete, the CLI will try to guess (unless --strict is set).
		`),
	RunE:         sinkResumeE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func sinkResumeE(cmd *cobra.Command, args []string) error {
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
		matching, err := fuzzyMatchDeployment(ctx, id, cli, []pbsinksvc.DeploymentStatus{
			pbsinksvc.DeploymentStatus_PAUSED,
			pbsinksvc.DeploymentStatus_STOPPED,
		})
		if err != nil {
			return err
		}
		id = matching.Id
	}

	req := &pbsinksvc.ResumeRequest{
		DeploymentId: id,
	}

	fmt.Printf("Resuming... (creating services, please wait)\n")

	resp, err := cli.Resume(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}
	fmt.Printf("Response for deployment %q:\n  Previous Status: %v, New Status: %v\n", id, resp.Msg.PreviousStatus, resp.Msg.NewStatus)

	return nil
}
