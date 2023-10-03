package main

import (
	"fmt"
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
}

var serveCmd = &cobra.Command{
	Use:   "sink-serve <package>",
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

	engine := "docker"
	srv, err := server.New(cmd.Context(), engine, dataDir, listenAddr, cors, zlog)
	if err != nil {
		return fmt.Errorf("initializing server: %w", err)
	}

	signal := derr.SetupSignalHandler(0)
	go func() {
		<-signal
		srv.Shutdown(nil)
	}()

	srv.Run()
	<-srv.Terminated()
	return srv.Err()
}
