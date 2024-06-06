package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/dgrpc"
	"go.uber.org/zap"

	pbbuild "github.com/streamingfast/substreams/remotebuild/pb/sf/remotebuild/v1"
)

func init() {
	remoteBuildCmd.Flags().StringArray("env", nil, "Environment variables to pass to the build server")
	remoteBuildCmd.Flags().StringP("output-name", "o", "substreams.spkg", "Spkg name to collect")

	rootCmd.AddCommand(remoteBuildCmd)
}

var remoteBuildCmd = &cobra.Command{
	Use:   "remote-build <remote-build-url> <zipped_source_code>",
	Short: "Send request to remote build server to build and package a substreams",
	Long: cli.Dedent(`
	Call the remote build server to build and package a substream. You can also add in environment variables to pass to the build server.
	Which will be injected in your substreams build. Make sure you align the make package output command with the collect pattern. By default
	it will collect the substreams.spkg file, but if you gave it a different name with the -o or --output-file flag, you should pass it here too.
	
	Example of valide make package commands: substreams pack:

	.PHONE: package
		pack: build
		substreams pack -o my-substreams.spkg substreams.yaml
	
	Then you will need to pass in the --output-name="my-substreams.spkg" flag to the remote-build command.
	
	Examples: substreams remote-build localhost:9000 my-substream.zip --env="ENV0=test0,ENV1=test1" --env "ENV2=test2" --env "ENV3=test3" --output-name="my-substream.spkg"
	`),
	RunE:         remoteBuildE,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
}

func remoteBuildE(cmd *cobra.Command, args []string) error {
	cloudRunServiceURL := args[0]
	filepath := args[1]
	envs := sflags.MustGetStringArray(cmd, "env")
	spkgName := sflags.MustGetString(cmd, "output-name")

	plaintext := false
	if strings.HasPrefix(cloudRunServiceURL, "localhost") {
		plaintext = true
	}

	cloudRunServiceURL = strings.Replace(cloudRunServiceURL, "https://", "", 1)
	if len(strings.Split(cloudRunServiceURL, ":")) == 1 {
		if plaintext {
			cloudRunServiceURL = fmt.Sprintf("%s:9000", cloudRunServiceURL)
		} else {
			// add the port if it is missing
			cloudRunServiceURL = fmt.Sprintf("%s:443", cloudRunServiceURL)
		}
	}

	credsOption, err := dgrpc.WithAutoTransportCredentials(false, plaintext, false)
	cli.NoError(err, "unable to create auto transport credentials option")

	fmt.Printf("Connecting to remote build server at %s ...\n", cloudRunServiceURL)
	conn, err := dgrpc.NewClientConn(cloudRunServiceURL, credsOption)
	cli.NoError(err, "unable to create external client")
	defer func() {
		if err := conn.Close(); err != nil {
			zlog.Error("unable to close connection gracefully", zap.Error(err))
		}
	}()

	fmt.Printf("Reading file: %s ...\n", filepath)
	b, err := os.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fmt.Println("Sending build request ...")
	client := pbbuild.NewBuildServiceClient(conn)
	buildResponse, err := client.Build(context.Background(), &pbbuild.BuildRequest{
		SourceCode:     b,
		Env:            envs,
		CollectPattern: spkgName,
	})
	if err != nil {
		return fmt.Errorf("failed to build: %w", err)
	}

	for {
		resp, err := buildResponse.Recv()
		if err != nil {
			return fmt.Errorf("failed to build: %w", err)
		}

		if resp == nil {
			break
		}

		if resp.Error != "" {
			if resp.Logs != "" {
				fmt.Print(resp.Logs)
			}

			return fmt.Errorf("failed to build: %s", resp.Error)
		}

		if len(resp.Artifacts) != 0 {
			// printout the rest of the logs, if there are any
			if resp.Logs != "" {
				fmt.Print(resp.Logs)
			}
			for _, artifact := range resp.Artifacts {
				err = os.WriteFile(artifact.Filename, artifact.Content, 0644)
				if err != nil {
					return fmt.Errorf("failed to write file: %w", err)
				}
			}
			break
		}

		// print out the logs as we get them, if there are any
		if resp.Logs != "" {
			fmt.Print(resp.Logs)
		}
	}

	fmt.Println("All done!")
	return nil
}
