package main

import (
	"fmt"
	"net/http"

	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
	cli "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/manifest"
	pbsinksvc "github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1"
	"github.com/streamingfast/substreams/pb/sf/substreams/sink/service/v1/pbsinksvcconnect"
	server "github.com/streamingfast/substreams/sink-server"
)

func init() {
	alphaCmd.AddCommand(updateCmd)
	updateCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "Specify the endpoint to connect to")
	updateCmd.Flags().Bool("strict", false, "Require deploymentID parameter to be set and complete")
	updateCmd.Flags().BoolP("reset", "r", false, "Reset the deployment by DELETING ALL ITS DATA")
}

var updateCmd = &cobra.Command{
	Use:   "sink-update <package> [deploymentID]",
	Short: "Update a substreams package with a sink",
	Long: cli.Dedent(`
        Sends a "update" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        The substreams must contain a "SinkConfig" section to be deployable.
        If deploymentID is not set or is incomplete, the CLI will try to guess (unless --strict is set).
     	`),
	RunE:         updateE,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func updateE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	reader, err := manifest.NewReader(args[0], getReaderOpts(cmd)...)
	if err != nil {
		return err
	}
	pkg, err := reader.Read()
	if err != nil {
		return err
	}

	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	var id string
	if len(args) == 2 {
		id = args[1]
	}

	if len(id) < server.DeploymentIDLength {
		if sflags.MustGetBool(cmd, "strict") {
			return fmt.Errorf("invalid ID provided: %q and '--strict' is set", id)
		}
		matching, err := fuzzyMatchDeployment(ctx, id, cli, fuzzyMatchPreferredStatusOrder)
		if err != nil {
			return err
		}
		fmt.Printf("Found deployment %q (%s-%s) from 'fuzzy search'. Do you really want to update this one ? (y/n): ", matching.Id, matching.PackageInfo.Name, matching.PackageInfo.Version)
		if !userConfirm() {
			return fmt.Errorf("cancelled by user")
		}
		id = matching.Id
	}

	req := &pbsinksvc.UpdateRequest{
		SubstreamsPackage: pkg,
		DeploymentId:      id,
		Reset_:            sflags.MustGetBool(cmd, "reset"),
	}
	deletingString := ""
	if req.Reset_ {
		deletingString = " deleting data,"
	}

	fmt.Printf("Updating sink %q... (restarting services,%s please wait)\n", req.DeploymentId, deletingString)
	resp, err := cli.Update(ctx, connect.NewRequest(req))
	if err != nil {
		return interceptConnectionError(err)
	}

	reason := ""
	if resp.Msg.Reason != "" {
		reason = " (" + resp.Msg.Reason + ")"
	}
	fmt.Printf("Update complete for sink %q:\n  Status: %v%s\n", req.DeploymentId, resp.Msg.Status, reason)
	printServices(resp.Msg.Services)
	return nil
}
