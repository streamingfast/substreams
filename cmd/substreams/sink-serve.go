package main

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/sink-server/docker"
	"github.com/streamingfast/substreams/sink-server/kubernetes"
	"os"
	"regexp"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	server "github.com/streamingfast/substreams/sink-server"
	"go.uber.org/zap/zapcore"
)

func init() {
	alphaCmd.AddCommand(serveCmd)

	serveCmd.Flags().String("data-dir", "./sink-data", "Store data to this folder")
	serveCmd.Flags().String("listen-addr", "localhost:8000", "Listen for GRPC connections on this address")
	serveCmd.Flags().String("cors-host-regex-allow", "^localhost", "Regex to allow CORS origin requests from, defaults to localhost only")
	serveCmd.Flags().String("engine", "docker", "Engine to use for deployments, defaults to docker")
	serveCmd.Flags().String("kubernetes-config-path", "", "Path to the kubeconfig file for kubernetes engine. If empty, will use InClusterConfig")
	serveCmd.Flags().String("kubernetes-namespace", "hosted-substreams-sinks", "Namespace to use for kubernetes engine")
}

var serveCmd = &cobra.Command{
	Use:   "sink-serve",
	Short: "Serve local sink deployments using docker-compose",
	Long: cli.Dedent(`
        Listens for "deploy" requests, allowing you to test your sink deployable units to a local docker-based dev environment.
	`),
	PersistentPreRun: func(cmd *cobra.Command, _ []string) {
		setup(cmd, zapcore.InfoLevel)
	},
	RunE:         serveE,
	Args:         cobra.ExactArgs(0),
	SilenceUsage: true,
}

func serveE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	listenAddr := sflags.MustGetString(cmd, "listen-addr")
	corsHostRegexAllow := sflags.MustGetString(cmd, "cors-host-regex-allow")
	dataDir := sflags.MustGetString(cmd, "data-dir")

	var cors *regexp.Regexp
	if corsHostRegexAllow != "" {
		hostRegex, err := regexp.Compile(corsHostRegexAllow)
		if err != nil {
			return fmt.Errorf("faild to compile cors host regex: %w", err)
		}
		cors = hostRegex
	}

	token := os.Getenv("SUBSTREAMS_API_TOKEN")
	if token == "" {
		return fmt.Errorf("error: please set SUBSTREAMS_API_TOKEN environment variable to a valid substreams API token")
	}

	engineType := sflags.MustGetString(cmd, "engine")
	var engine server.Engine
	var err error
	switch engineType {
	case "docker":
		engine, err = docker.NewEngine(dataDir, token)
		if err != nil {
			return err
		}
	case "kubernetes":
		engine, err = kubernetes.NewEngine(
			sflags.MustGetString(cmd, "kubernetes-config-path"),
			sflags.MustGetString(cmd, "kubernetes-namespace"),
			token,
		)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported engine: %q", engine)
	}

	srv, err := server.New(ctx, engine, dataDir, listenAddr, cors, zlog)
	if err != nil {
		return fmt.Errorf("initializing server: %w", err)
	}

	signal := derr.SetupSignalHandler(0)
	go func() {
		<-signal
		srv.Shutdown(nil)
		cancel()
	}()

	srv.Run(ctx)
	<-srv.Terminated()
	return srv.Err()
}
