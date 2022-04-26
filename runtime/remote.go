package runtime

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/firehose/client"
	pbfirehose "github.com/streamingfast/pbgo/sf/firehose/v1"
	"github.com/streamingfast/substreams/manifest"
	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/anypb"
	"io"
	"os"
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

	sub := &pbtransform.Transform{
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

	fhClient, callOpts, err := client.NewFirehoseClient(
		config.FirehoseEndpoint,
		os.Getenv(config.FirehoseApiKeyEnvVar),
		config.InsecureMode,
		config.Plaintext,
	)

	if err != nil {
		return fmt.Errorf("firehose client: %w", err)
	}
	zlog.Info("firehose client configured")

	req := &pbfirehose.Request{
		StartBlockNum: int64(config.StartBlock),
		StopBlockNum:  config.StopBlock,
		ForkSteps:     []pbfirehose.ForkStep{pbfirehose.ForkStep_STEP_IRREVERSIBLE},
		Transforms: []*anypb.Any{
			trans,
		},
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
		zlog.Debug("object received from stream")
		cursor, _ := bstream.CursorFromOpaque(resp.Cursor)
		output := &pbsubstreams.Output{}
		err = proto.Unmarshal(resp.Block.GetValue(), output)
		if err != nil {
			return fmt.Errorf("unmarshalling substream output: %w", err)
		}
		retErr := config.ReturnHandler(output, stepFromProto(resp.Step), cursor)
		if retErr != nil {
			return fmt.Errorf("return handler: %w", retErr)
		}
	}
}

func stepFromProto(step pbfirehose.ForkStep) bstream.StepType {
	switch step {
	case pbfirehose.ForkStep_STEP_NEW:
		return bstream.StepNew
	case pbfirehose.ForkStep_STEP_UNDO:
		return bstream.StepUndo
	case pbfirehose.ForkStep_STEP_IRREVERSIBLE:
		return bstream.StepIrreversible
	}
	return bstream.StepType(0)
}
