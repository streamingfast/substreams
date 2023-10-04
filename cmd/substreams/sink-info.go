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
	alphaCmd.AddCommand(sinkInfoCmd)
	sinkInfoCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
	sinkInfoCmd.Flags().Bool("strict", false, "Require deploymentID parameter to be set and complete")
}

var sinkInfoCmd = &cobra.Command{
	Use:   "sink-info [deployment-id]",
	Short: "Get info on a deployed substreams sink",
	Long: cli.Dedent(`
        Sends an "Info" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It returns information about the status of the substreams and its available services.
        If deploymentID is not set or is incomplete, the CLI will try to guess (unless --strict is set).
		`),
	RunE:         sinkInfoE,
	Args:         cobra.RangeArgs(0, 1),
	SilenceUsage: true,
}

func sinkInfoE(cmd *cobra.Command, args []string) error {
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

	req := &pbsinksvc.InfoRequest{
		DeploymentId: id,
	}

	resp, err := cli.Info(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}
	reason := ""
	if resp.Msg.Reason != "" {
		reason = " (" + resp.Msg.Reason + ")"
	}
	fmt.Printf("Response for deployment %q:\n  Name: %s (%s)\n  Output module: %s (%s)\n  Status: %v%s\n ", id, resp.Msg.PackageInfo.Name, resp.Msg.PackageInfo.Version, resp.Msg.PackageInfo.OutputModuleName, resp.Msg.PackageInfo.OutputModuleHash, resp.Msg.Status, reason)
	if resp.Msg.Progress != nil {
		fmt.Printf("Last processed block: %d\n", resp.Msg.Progress.LastProcessedBlock)
	}
	printServices(resp.Msg.Services)

	return nil
}
