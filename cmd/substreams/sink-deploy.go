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
)

func init() {
	alphaCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
}

var deployCmd = &cobra.Command{
	Use:   "sink-deploy <package>",
	Short: "Deploy a substreams package with a sink",
	Long: cli.Dedent(`
        Sends a "deploy" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        The substreams must contain a "SinkConfig" section to be deployable.
	`),
	RunE:         deployE,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func deployE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	reader, err := manifest.NewReader(args[0], getReaderOpts(cmd)...)
	if err != nil {
		return err
	}
	pkg, err := reader.Read()
	if err != nil {
		return err
	}

	req := &pbsinksvc.DeployRequest{
		SubstreamsPackage: pkg,
	}

	fmt.Printf("Deploying... (creating services, please wait)\n")
	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	resp, err := cli.Deploy(ctx, connect.NewRequest(req))
	if err != nil {
		return interceptConnectionError(err)
	}

	reason := ""
	if resp.Msg.Reason != "" {
		reason = " (" + resp.Msg.Reason + ")"
	}
	fmt.Printf("Deployed substreams sink %q:\n  Status: %v%s\n", resp.Msg.DeploymentId, resp.Msg.Status, reason)
	printServices(resp.Msg.Services)
	return nil
}
