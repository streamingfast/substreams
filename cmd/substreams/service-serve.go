package main

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/streamingfast/dauth"
	dauthnull "github.com/streamingfast/dauth/null"
	"github.com/streamingfast/substreams/sink-server/docker"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	server "github.com/streamingfast/substreams/sink-server"
	"go.uber.org/zap/zapcore"
)

func init() {
	serviceCmd.AddCommand(serveCmd)

	serveCmd.Flags().StringP("endpoint", "e", "", "Substreams endpoint to connect to")
	serveCmd.Flags().String("data-dir", "./sink-data", "Store data to this folder")
	serveCmd.Flags().String("listen-addr", "localhost:8000", "Listen for GRPC connections on this address")
	serveCmd.Flags().String("cors-host-regex-allow", "^localhost", "Regex to allow CORS origin requests from, defaults to localhost only")
	serveCmd.Flags().String("engine", "docker", "Engine to use for deployments, defaults to docker")
	serveCmd.Flags().String("kubernetes-config-path", "", "Path to the kubeconfig file for kubernetes engine. If empty, will use InClusterConfig")
	serveCmd.Flags().String("kubernetes-namespace", "hosted-substreams-sinks", "Namespace to use for kubernetes engine")
	dauthnull.Register()
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve local service deployments using docker-compose",
	Long: cli.Dedent(`
        Listens for "deploy" requests, allowing you to test your deployable units to a local docker-based dev environment.
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
	endpoint := sflags.MustGetString(cmd, "endpoint")

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
		engine, err = docker.NewEngine(dataDir, token, endpoint)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported engine: %q", engine)
	}

	// local service serve does not support auth, so we disable by using null://
	auth, err := dauth.New("null://", zlog)
	if err != nil {
		return err
	}

	srv, err := server.New(ctx, engine, dataDir, listenAddr, cors, auth, zlog)
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
