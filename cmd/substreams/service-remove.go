package main

import (
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	cli "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	"github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1/pbsinksvcconnect"
	server "github.com/streamingfast/substreams/sink-server"
)

func init() {
	serviceCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:   "remove [deployment-id]",
	Short: "Remove a deployed service",
	Long: cli.Dedent(`
        Sends an "Remove" request to a server. By default, it will talk to a local "substreams alpha service serve" instance.
        It will remove a service completely, including its data. Use "pause" instead if you want to keep the data.
        If deploymentID is not set or is incomplete, the CLI will try to guess (unless --strict is set).
		`),
	RunE:         removeE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func removeE(cmd *cobra.Command, args []string) error {
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
		matching, err := fuzzyMatchDeployment(ctx, id, cli, cmd, []pbsinksvc.DeploymentStatus{
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

	req := connect.NewRequest(&pbsinksvc.RemoveRequest{
		DeploymentId: id,
	})
	if err := addHeaders(cmd, req); err != nil {
		return err
	}

	fmt.Printf("Stopping... (shutting down services and removing data, please wait)\n")
	resp, err := cli.Remove(ctx, req)
	if err != nil {
		return interceptConnectionError(err)
	}
	fmt.Printf("Deployment %q successfully deleted.\nPrevious Status: %v\nNew Status: DELETED.", id, resp.Msg.PreviousStatus)

	return nil
}
