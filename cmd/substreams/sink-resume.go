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
	alphaCmd.AddCommand(sinkResumeCmd)
	sinkResumeCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
}

var sinkResumeCmd = &cobra.Command{
	Use:   "sink-resume <deployment-id>",
	Short: "Resume a paused substreams sink",
	Long: cli.Dedent(`
        Sends an "Resume" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It will resume a paused substreams and returns information about the change of status.
		`),
	RunE:         sinkResumeE,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func sinkResumeE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	id := args[0]

	req := &pbsinksvc.ResumeRequest{
		DeploymentId: id,
	}

	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	resp, err := cli.Resume(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}
    fmt.Printf("Response for deployment %q:\n  Previous Status: %v, New Status: %v\n", id, resp.Msg.PreviousStatus, resp.Msg.NewStatus)

	return nil
}
