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
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/anypb"
	"os"
)

func RemoteRun(ctx context.Context, config *Config) error {
	manif, err := manifest.New(config.ManifestPath)
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", config.ManifestPath, err)
	}

	if config.PrintMermaid {
		manif.PrintMermaid()
	}

	manifProto, err := manif.ToProto()
	if err != nil {
		return fmt.Errorf("parse manifest to proto%q: %w", config.ManifestPath, err)
	}

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

	if config.StartBlock == 0 {
		sb, err := graph.ModuleStartBlock(config.OutputStreamName)
		if err != nil {
			return fmt.Errorf("getting module start block: %w", err)
		}
		config.StartBlock = sb
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

	req := &pbfirehose.Request{
		StartBlockNum: int64(config.StartBlock),
		StopBlockNum:  config.StopBlock,
		ForkSteps:     []pbfirehose.ForkStep{pbfirehose.ForkStep_STEP_IRREVERSIBLE},
		Transforms: []*anypb.Any{
			trans,
		},
	}

	cli, err := fhClient.Blocks(ctx, req, callOpts...)
	if err != nil {
		return fmt.Errorf("call Blocks: %w", err)
	}

	for {
		resp, err := cli.Recv()
		if err != nil {
			return err
		}
		cursor, _ := bstream.CursorFromOpaque(resp.Cursor)
		output := &pbsubstreams.Output{}
		err = proto.Unmarshal(resp.Block.GetValue(), output)
		if err != nil {
			return fmt.Errorf("unmarshalling substream output: %w", err)
		}
		retErr := config.ReturnHandler(output, stepFromProto(resp.Step), cursor)
		if retErr != nil {
			fmt.Println(retErr)
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
