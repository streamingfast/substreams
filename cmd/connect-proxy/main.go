package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/bufbuild/connect-go"
	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	ssconnect "github.com/streamingfast/substreams/proto/sf/substreams/v1/substreamsv1connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type ConnectServer struct {
	ssconnect.UnimplementedStreamHandler
	Manifest               string
	StartBlock             uint64
	SubstreamsClientConfig *client.SubstreamsClientConfig
}

func (cs *ConnectServer) Blocks(
	ctx context.Context,
	req *connect.Request[pbsubstreams.Request],
	stream *connect.ServerStream[pbsubstreams.Response],
) error {

	newReq := &pbsubstreams.Request{
		StartBlockNum:                  req.Msg.StartBlockNum,
		StopBlockNum:                   req.Msg.StopBlockNum,
		StartCursor:                    req.Msg.StartCursor,
		ForkSteps:                      req.Msg.ForkSteps,
		IrreversibilityCondition:       req.Msg.IrreversibilityCondition,
		OutputModules:                  req.Msg.OutputModules,
		Modules:                        req.Msg.Modules,
		InitialStoreSnapshotForModules: req.Msg.InitialStoreSnapshotForModules,
	}

	if cs.Manifest != "" {
		manifestReader := manifest.NewReader(cs.Manifest)
		pkg, err := manifestReader.Read()
		if err != nil {
			return fmt.Errorf("read manifest %q: %w", cs.Manifest, err)
		}
		newReq.Modules = pkg.Modules
	}

	if cs.StartBlock != 0 {
		newReq.StartBlockNum = int64(cs.StartBlock)
	}

	ssClient, connClose, callOpts, err := client.NewSubstreamsClient(cs.SubstreamsClientConfig)
	if err != nil {
		return fmt.Errorf("substreams client setup: %w", err)
	}
	defer connClose()

	if err := pbsubstreams.ValidateRequest(newReq); err != nil {
		return fmt.Errorf("validate request: %w", err)
	}

	cli, err := ssClient.Blocks(ctx, newReq, callOpts...)
	if err != nil {
		return fmt.Errorf("call sf.substreams.v1.Stream/Blocks: %w", err)
	}

	for {
		resp, err := cli.Recv()
		if resp != nil {
			stream.Send(resp)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("failed:", err)
	}
}

var rootCmd = &cobra.Command{
	Use:          "connect-proxy",
	Short:        "A tool to proxy substreams requests from a web browser (connect-web protocol)",
	Long:         "A tool to proxy substreams requests from a web browser (connect-web protocol)",
	SilenceUsage: true,
	RunE:         run,
}

func init() {
	rootCmd.Flags().StringP("substreams-endpoint", "e", "api.streamingfast.io:443", "Substreams gRPC endpoint")
	rootCmd.Flags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "name of variable containing Substreams Authentication token")
	rootCmd.Flags().String("listen-addr", "localhost:8080", "listen on this address (unencrypted)")
	rootCmd.Flags().BoolP("insecure", "k", false, "Skip certificate validation on GRPC connection")
	rootCmd.Flags().BoolP("plaintext", "p", false, "Establish GRPC connection in plaintext")
	rootCmd.Flags().String("force-manifest", "", "if non-empty, the requests' modules will be replaced by the modules loaded from this location. Can be a local spkg or yaml file, or a remote (HTTP) spkg file.")
	rootCmd.Flags().Uint64("force-start-block", 0, "if non-zero, the requests' start-block will be replaced by this value")
}

func run(cmd *cobra.Command, args []string) error {
	addr := mustGetString(cmd, "listen-addr")
	fmt.Println("listening on", addr)

	substreamsClientConfig := client.NewSubstreamsClientConfig(
		mustGetString(cmd, "substreams-endpoint"),
		readAPIToken(cmd, "substreams-api-token-envvar"),
		mustGetBool(cmd, "insecure"),
		mustGetBool(cmd, "plaintext"),
	)

	cs := &ConnectServer{
		Manifest:               mustGetString(cmd, "force-manifest"),
		SubstreamsClientConfig: substreamsClientConfig,
		StartBlock:             mustGetUint64(cmd, "force-start-block"),
	}

	mux := http.NewServeMux()
	// The generated constructors return a path and a plain net/http
	// handler.
	mux.Handle(ssconnect.NewStreamHandler(cs))
	return http.ListenAndServe(
		addr,
		// For gRPC clients, it's convenient to support HTTP/2 without TLS. You can
		// avoid x/net/http2 by using http.ListenAndServeTLS.
		h2c.NewHandler(mux, &http2.Server{}),
	)
}

func mustGetString(cmd *cobra.Command, flagName string) string {
	val, err := cmd.Flags().GetString(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}
func readAPIToken(cmd *cobra.Command, envFlagName string) string {
	envVar := mustGetString(cmd, envFlagName)
	value := os.Getenv(envVar)
	if value != "" {
		return value
	}

	return os.Getenv("SF_API_TOKEN")
}

func mustGetInt64(cmd *cobra.Command, flagName string) int64 {
	val, err := cmd.Flags().GetInt64(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}
func mustGetUint64(cmd *cobra.Command, flagName string) uint64 {
	val, err := cmd.Flags().GetUint64(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}
func mustGetBool(cmd *cobra.Command, flagName string) bool {
	val, err := cmd.Flags().GetBool(flagName)
	if err != nil {
		panic(fmt.Sprintf("flags: couldn't find flag %q", flagName))
	}
	return val
}
