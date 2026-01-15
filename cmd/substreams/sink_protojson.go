package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lithammer/dedent"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/dstore"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/protojson"
	"go.uber.org/zap"
)

var sinkProtoJSONCmd = &cobra.Command{
	Use:   "protojson <manifest> [<module>] [--start-block=<num>] [--stop-block=<num>]",
	Short: "Downloads substreams data to ProtoJSONL files",
	Long: `
Downloads Substreams data to JSONL files (one entry per line) using 'ProtoJSON' encoding (https://protobuf.dev/programming-guides/json/)",
`,
	RunE: JSONRunE,
	Args: cobra.RangeArgs(2, 3),

	Example: `
	# Extract USDT events to ProtoJSONL of a 200 blocks range, in 100-blocks-chunks
	substreams sink protojson substreams_ethereum_usdt@v0.1.0 map_events -o ./output --filter=.transfers[] -n 100 -s 20000000 -t +200
	`,
}

func init() {
	configureProtoJSONFlags(sinkProtoJSONCmd.Flags())
	SinkCmd.AddCommand(sinkProtoJSONCmd)
}

func configureProtoJSONFlags(flags *pflag.FlagSet) {
	// default sinker flags - ignore specific flags for noop sink
	sink.AddFlagsToSet(flags,
		sink.FlagExcludeDefault(
			sink.FlagUndoBufferSize,
			sink.FlagDevelopmentMode,
			sink.FlagNoopMode,
		),
	)

	flags.String("state-file", "./state.yaml", "File where the sink will store its cursor. If empty, no cursor will be saved or used, only the start-block.")
	flags.StringP("output-dir", "o", "./output", FlagMultiLineDescription(`
				Output store where to write files, supports gs://, s3://, file:// and local paths,
				see https://github.com/streamingfast/dstore?tab=readme-ov-file#features for supported syntax.
			`))

	flags.String("filter", ".", "Filter to apply to the data when choosing what to extract. Use '.' to extract the full message from that block.")
	flags.String("working-dir", "./localdata/working", "Working store where we accumulate data")
	flags.Uint64P("blocks-per-file", "n", 1000, "Number of blocks per file")

	flags.Uint64("buffer-max-size", 64*1024*1024, FlagMultiLineDescription(`
				Amount of memory bytes to allocate to the buffered writer. If your data set is small enough that every is hold in memory, we are going to avoid
				the local I/O operation(s) and upload accumulated content in memory directly to final storage location.

				Ideally, you should set this as to about 80%% of RAM the process has access to. This will maximize amout of element in memory,
				and reduce 'syscall' and I/O operations to write to the temporary file as we are buffering a lot of data.

				This setting has probably the greatest impact on writing throughput.

				Default value for the buffer is 64 MiB.
			`))

}

func JSONRunE(cmd *cobra.Command, args []string) error {
	app := cli.NewApplication(cmd.Context())

	manifestPath := args[0]
	outputModuleName := sink.InferOutputModuleFromPackage

	if len(args) > 1 {
		outputModuleName = args[1]
	}

	filter := sflags.MustGetString(cmd, "filter")
	fileOutputPath := sflags.MustGetString(cmd, "output-dir")
	fileWorkingDir := sflags.MustGetString(cmd, "working-dir")
	stateStorePath := sflags.MustGetString(cmd, "state-file")
	blocksPerFile := sflags.MustGetUint64(cmd, "blocks-per-file")
	bufferMaxSize := sflags.MustGetUint64(cmd, "buffer-max-size")

	zlog.Info("sink to JSON",
		zap.String("filter", filter),
		zap.String("file_output_path", fileOutputPath),
		zap.String("file_working_dir", fileWorkingDir),
		zap.String("state_store", stateStorePath),
		zap.Uint64("blocks_per_file", blocksPerFile),
		zap.Uint64("buffer_max_size", bufferMaxSize),
	)

	sinkerConfig, err := sink.ConfigFromViper(cmd,
		sink.IgnoreOutputModuleType,
		manifestPath, outputModuleName,
		"substreams-sink-protojson",
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new sinker config: %w", err)
	}

	if sinkerConfig.StopBlock != 0 {
		sinkerConfig.StopBlock += 1 // common case: we need this block to be INCLUDED so that the filewriter will commit to disk
	}
	sinker, err := sink.NewFromConfig(sinkerConfig)
	if err != nil {
		return fmt.Errorf("new sinker: %w", err)
	}

	if blockRange := sinker.BlockRange(); blockRange.EndBlock() != nil {
		size, err := sinker.BlockRange().Size()
		if err != nil {
			panic(fmt.Errorf("size should error only on open ended range, which we should have caught earlier: %w", err))
		}

		cli.Ensure(size >= blocksPerFile, "You requested %d blocks per file but your block range spans only %d blocks, this would produce 0 file, refusing to start", blocksPerFile, size)
	}

	fileOutputStore, err := dstore.NewStore(fileOutputPath, "", "", false)
	if err != nil {
		return fmt.Errorf("new store %q: %w", fileOutputPath, err)
	}

	stateFileDirectory := filepath.Dir(stateStorePath)
	if err := os.MkdirAll(stateFileDirectory, os.ModePerm); err != nil {
		return fmt.Errorf("create state file directories: %w", err)
	}

	stateStore, err := protojson.NewFileStateStore(stateStorePath)
	if err != nil {
		return fmt.Errorf("new file state store: %w", err)
	}

	boundaryWriter := protojson.NewBufferedIO(bufferMaxSize, fileWorkingDir, protojson.FileTypeJSONL, zlog)

	msgDesc, err := protojson.OutputMessageDescriptor(sinker)
	if err != nil {
		return fmt.Errorf("output message descriptor: %w", err)
	}

	sinkEncoder, err := protojson.NewProtoToJson(filter, msgDesc)
	if err != nil {
		return fmt.Errorf("new proto to protojson encoder: %w", err)
	}

	bundler, err := protojson.NewBundler(
		blocksPerFile,
		boundaryWriter,
		stateStore,
		fileOutputStore,
		zlog,
	)
	if err != nil {
		return fmt.Errorf("new bundler: %w", err)
	}

	app.SuperviseAndStart(protojson.NewFileSinker(sinker, bundler, sinkEncoder, zlog, tracer))

	if err := app.WaitForTermination(zlog, 0*time.Second, 30*time.Second); err != nil {
		zlog.Info("app termination error", zap.Error(err))
		return err
	}

	zlog.Info("app terminated")
	return nil
}

func FlagMultiLineDescription(in string) string {
	return string(Description(in))
}

func Description(value string) description {
	return description(strings.TrimSpace(dedent.Dedent(value)))
}

type description string

func (d description) Apply(cmd *cobra.Command) {
	cmd.Long = string(d)
}
