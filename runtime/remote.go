package runtime

import (
	"fmt"
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/anypb"
	"io"
)

func RemoteRun(ctx context.Context, config *RemoteConfig) error {
	zlog.Info("about to run remote",
		zap.String("output_stream_name", config.OutputStreamName),
		zap.String("firehose_endpoint", config.FirehoseEndpoint),
		zap.String("firehose_api_key", config.FirehoseApiKeyEnvVar),
		zap.Uint64("start_block", config.StartBlock),
		zap.Uint64("stop_block", config.StopBlock))

	manif, err := manifest.New(config.ManifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", config.ManifestPath, err)
	}
	zlog.Info("manifest loaded")

	if config.PrintMermaid {
		manif.PrintMermaid()
	}

	manifProto, err := manif.ToProto()
	if err != nil {
		return fmt.Errorf("parse manifest to proto%q: %w", config.ManifestPath, err)
	}
	zlog.Info("parsed manifest to proto")

	// TURN THAT INTO A REQUEST NOW
	sub := &pbsubstreams.Request{
		OutputModule: config.OutputStreamName,
		Manifest:     manifProto,
	}

	trans, err := anypb.New(sub)
	if err != nil {
		return fmt.Errorf("convert transform to any: %w", err)
	}

	graph, err := manifest.NewModuleGraph(manifProto.Modules)
	if err != nil {
		return fmt.Errorf("create module graph %w", err)
	}
	zlog.Info("graph created")

	if config.StartBlock == 0 {
		sb, err := graph.ModuleStartBlock(config.OutputStreamName)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		config.StartBlock = sb
		zlog.Info("start block updated", zap.Uint64("new_start_block", config.StartBlock))
	}

	fhClient, callOpts, err := client.NewSubstreamsClient(
		config.FirehoseEndpoint,
		os.Getenv(config.FirehoseApiKeyEnvVar),
		config.InsecureMode,
		config.Plaintext,
	)

	if err != nil {
		return fmt.Errorf("substreams client: %w", err)
	}
	zlog.Info("firehose client configured")

	req := &pbsubstreams.Request{
		StartBlockNum: int64(config.StartBlock),
		StopBlockNum:  config.StopBlock,
		ForkSteps:     []pbsubstreams.ForkStep{pbsubstreams.ForkStep_STEP_IRREVERSIBLE},
		Manifest:      manifProto,
		OutputModules: []string{config.OutputStreamName},
	}
	zlog.Info("firehose request created")

	stream, err := fhClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		return fmt.Errorf("call Blocks: %w", err)
	}
	zlog.Info("stream created")

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				zlog.Info("received end of file on stream")
				return nil
			}
			return fmt.Errorf("receiving block: %w", err)
		}

		switch r := resp.Message.(type) {
		case *pbsubstreams.Response_Progress:
			_ = r.Progress
		case *pbsubstreams.Response_SnapshotData:
			_ = r.SnapshotData
		case *pbsubstreams.Response_SnapshotComplete:
			_ = r.SnapshotComplete
		case *pbsubstreams.Response_Data:
			// block-scoped data
			resp := r.Data
			cursor, _ := bstream.CursorFromOpaque(resp.Cursor)

			if err := config.ReturnHandler(resp); err != nil {
				fmt.Printf("RETURN HANDLER ERROR: %s\n", err)
			}

			fmt.Println("---------- %d (%s) %s", resp.Clock.Number, resp.Clock.Id, resp.Clock.Timestamp)
			for _, output := range resp.Outputs {
				for _, line := range output.Logs {
					fmt.Printf("LOG (%s): %s\n", output.Name, line)
				}
				switch data := output.Data.(type) {
				case *pbsubstreams.ModuleOutput_MapOutput:
					retErr := config.ReturnHandler(output, stepFromProto(resp.Step), cursor)
					if retErr != nil {
			return fmt.Errorf("return handler: %w", retErr)
					}
					_ = data
				case *pbsubstreams.ModuleOutput_StoreDeltas:
					retErr := config.ReturnHandler(output, stepFromProto(resp.Step), cursor)
					if retErr != nil {
						fmt.Println(retErr)
					}
					_ = data
				}
				err = proto.Unmarshal(output.Block.GetValue(), output)
				if err != nil {
					return fmt.Errorf("unmarshalling substream output: %w", err)
				}
			}
		}
	}
}

func stepFromProto(step pbsubstreams.ForkStep) bstream.StepType {
	switch step {
	case pbsubstreams.ForkStep_STEP_NEW:
		return bstream.StepNew
	case pbsubstreams.ForkStep_STEP_UNDO:
		return bstream.StepUndo
	case pbsubstreams.ForkStep_STEP_IRREVERSIBLE:
		return bstream.StepIrreversible
	}
	return bstream.StepType(0)
}
