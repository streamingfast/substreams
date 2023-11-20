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
	"github.com/streamingfast/substreams/sink-server/context"
	"github.com/streamingfast/substreams/sink-server/printer"
)

func init() {
	serviceCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringArrayP("parameters", "p", []string{}, "Parameters to pass to the substreams")
}

var deployCmd = &cobra.Command{
	Use:   "deploy <package>",
	Short: "Deploy a substreams package with a sink",
	Long: cli.Dedent(`
        Sends a "deploy" request to a server. By default, it will talk to a local "substreams alpha service serve" instance.
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

	//request parameters
	// fmt.Println("request parameters")
	paramsMap := make(map[string]string)
	for _, param := range mustGetStringArray(cmd, "parameters") {
		parts := strings.SplitN(param, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid parameter format: %q", param)
		}
		paramsMap[parts[0]] = parts[1]
	}

	params := []*pbsinksvc.Parameter{}
	for k, v := range paramsMap {
		params = append(params, &pbsinksvc.Parameter{
			Key:   k,
			Value: v,
		})
	}

	req := connect.NewRequest(&pbsinksvc.DeployRequest{
		SubstreamsPackage: pkg,
		Parameters:        params,
	})
	if err := addHeaders(cmd, req); err != nil {
		return err
	}

	fmt.Printf("Deploying... (creating services, please wait)\n")
	cli := pbsinksvcconnect.NewProviderClient(http.DefaultClient, sflags.MustGetString(cmd, "endpoint"))
	ctx = context.WithDeployStatusPrinter(ctx, &printer.DeployStatusPrinter{})

	resp, err := cli.Deploy(ctx, req)
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
