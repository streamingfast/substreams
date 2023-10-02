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
	alphaCmd.AddCommand(sinkInfoCmd)
	sinkInfoCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
}

var sinkInfoCmd = &cobra.Command{
	Use:   "sink-info <deployment-id>",
	Short: "Get info on a deployed substreams sink",
	Long: cli.Dedent(`
        Sends an "Info" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        It returns information about the status of the substreams and its available services.
		`),
	RunE:         sinkInfoE,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func sinkInfoE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	id := args[0]

	req := &pbsinksvc.InfoRequest{
		DeploymentId: id,
	}

	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	resp, err := cli.Info(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}
	fmt.Printf("Response for deployment %q:\n  Name: %s (%s)\n  Output module: %s (%s)\n  Status: %v (%s)\n ", id, resp.Msg.PackageInfo.Name, resp.Msg.PackageInfo.Version, resp.Msg.PackageInfo.OutputModuleName, resp.Msg.PackageInfo.OutputModuleHash, resp.Msg.Status, resp.Msg.Reason)
	if resp.Msg.Progress != nil {
		fmt.Printf("Last processed block: %d\n", resp.Msg.Progress.LastProcessedBlock)
	}
	fmt.Printf("Outputs:\n")
	printOutputs(resp.Msg.Outputs)

	return nil
}
