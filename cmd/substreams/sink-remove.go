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
	alphaCmd.AddCommand(sinkRemoveCmd)
	sinkRemoveCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
	sinkRemoveCmd.Flags().Bool("strict", false, "Require deploymentID parameter to be set and complete")
}

var sinkRemoveCmd = &cobra.Command{
	Use:   "sink-remove [deployment-id]",
	Short: "Remove a deployed substreams sink",
	Long: cli.Dedent(`
        Sends an "Remove" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It will remove a substreams sink completely, including its data. Use "pause" instead if you want to keep the data.
        If deploymentID is not set or is incomplete, the CLI will try to guess (unless --strict is set).
		`),
	RunE:         sinkRemoveE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func sinkRemoveE(cmd *cobra.Command, args []string) error {
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
			pbsinksvc.DeploymentStatus_STOPPED,
			pbsinksvc.DeploymentStatus_PAUSED,
			pbsinksvc.DeploymentStatus_FAILING,
			pbsinksvc.DeploymentStatus_RUNNING,
			pbsinksvc.DeploymentStatus_UNKNOWN,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Found deployment %q (%s-%s) from 'fuzzy search'. Do you really want to delete its data ? (y/n): ", matching.Id, matching.PackageInfo.Name, matching.PackageInfo.Version)
		if !userConfirm() {
			return fmt.Errorf("cancelled by user")
		}
		id = matching.Id
	}

	req := &pbsinksvc.RemoveRequest{
		DeploymentId: id,
	}

	fmt.Printf("Stopping... (shutting down services and removing data, please wait)\n")
	resp, err := cli.Remove(ctx, connect.NewRequest(req))
	if err != nil {
		return interceptConnectionError(err)
	}
	fmt.Printf("Deployment %q successfully deleted.\nPrevious Status: %v\nNew Status: DELETED.", id, resp.Msg.PreviousStatus)

	return nil
}
