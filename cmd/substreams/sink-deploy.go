package main

import (
	"fmt"
	"net/http"
	"strings"

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
	Use:   "sink-deploy <package> [deploymentID]",
	Short: "Deploy a substreams package with a sink",
	Long: cli.Dedent(`
        Sends a "deploy" request to a server. By default, it will talk to a local "substreams alpha sink-serve" instance.
        The substreams must contain a "SinkConfig" section to be deployable.
        If a deploymentID is specified, the service should upgrade/replace that deployment.
			`),
	RunE:         deployE,
	Args:         cobra.RangeArgs(1, 2),
	SilenceUsage: true,
}

func deployE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	reader, err := manifest.NewReader(args[0])
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
    if len(args) == 2 {
        req.DeploymentId = &args[1]
    }

	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))

	resp, err := cli.Deploy(ctx, connect.NewRequest(req))
	if err != nil {
		return err
	}

	fmt.Printf("Deployed substreams sink %q:\n  Status: %v (%s)\n  Outputs:\n", resp.Msg.DeploymentId, resp.Msg.Status, resp.Msg.Reason)
	printOutputs(resp.Msg.Outputs)
	return nil
}

func printOutputs(outputs map[string]string) {
	for k, v := range outputs {
		lines := strings.Split(v, "\n")
        prefixLen := len(k) + 6
		var withMargin string
		for i, line := range lines {
            if i == 0 {
                withMargin = line + "\n"
                continue
            }
			withMargin += strings.Repeat(" ", prefixLen) + line + "\n"
		}
		fmt.Printf("  - %s: %s", k, withMargin)
	}

}
