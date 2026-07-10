package main

import (
	"fmt"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink"
	sinksql "github.com/streamingfast/substreams/sink/sql"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	"github.com/streamingfast/substreams/sink/sql/db_proto/proto"
	"github.com/streamingfast/substreams/sink/sql/services"
	"google.golang.org/protobuf/types/descriptorpb"
)

var sinkFromProtoCmd = &cobra.Command{
	Use:   "from-proto <manifest> [<output_module>]",
	Short: "Sink Substreams data into an SQL database using relational mappings derived from the protobuf output",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  sinkFromProtoE,
}

func init() {
	flags := sinkFromProtoCmd.Flags()
	sink.AddFlagsToSet(flags, sink.FlagExcludeDefault(sink.FlagUndoBufferSize))
	addOperatorFlags(flags)
	addDSNFlag(flags)

	flags.Bool("no-constraints", false, "Do not add any constraints to the database. This is useful to speed up the initial import of a large dataset.")
	flags.Int("block-batch-size", 25, "number of blocks to process at a time")
	flags.String("clickhouse-sink-info-folder", "", "folder where to store the clickhouse sink info")
	flags.String("clickhouse-cursor-file-path", "cursor.txt", "file name where to store the clickhouse cursor")
	flags.String("bytes-encoding", "raw", "Encoding for protobuf bytes fields (raw, hex, 0xhex, base64, base58)")
	flags.String("proto-file-override", "", "Override protobuf file to use instead of extracting from substreams package")
	flags.Int("clickhouse-query-retry-count", 3, "Number of retries for ClickHouse queries when an error occurs")
	flags.Duration("clickhouse-query-retry-sleep", time.Second, "Sleep duration between ClickHouse query retries (e.g. 1s, 500ms)")

	SinkCmd.AddCommand(sinkFromProtoCmd)
}

func sinkFromProtoE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}

	manifestPath := args[0]

	outputModuleName := sink.InferOutputModuleFromPackage
	if len(args) == 2 {
		outputModuleName = args[1]
	}

	useConstraints := !sflags.MustGetBool(cmd, "no-constraints")
	blockBatchSize := sflags.MustGetInt(cmd, "block-batch-size")

	encodingStr := sflags.MustGetString(cmd, "bytes-encoding")
	encoding, err := sqlbytes.ParseEncoding(encodingStr)
	if err != nil {
		return fmt.Errorf("invalid bytes encoding %q: %w", encodingStr, err)
	}

	retryCount := sflags.MustGetInt(cmd, "clickhouse-query-retry-count")
	retrySleep := sflags.MustGetDuration(cmd, "clickhouse-query-retry-sleep")

	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parsing dsn: %w", err)
	}

	sinkDBPreStart(cmd)

	app := cli.NewApplication(cmd.Context())

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	spkg, module, _, err := sink.ReadManifestAndModule(manifestPath, "", nil, outputModuleName, "", false, nil, zlog)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	outputModuleName = module.Name
	outputType := proto.ModuleOutputType(spkg, outputModuleName)
	if outputType == "" {
		return fmt.Errorf("could not find output type for module %s", outputModuleName)
	}

	if service, err := sinksql.ExtractSinkService(spkg); err == nil {
		if err := services.Run(service, zlog); err != nil {
			return fmt.Errorf("running service: %w", err)
		}
	}

	var fileDescriptor *desc.FileDescriptor

	protoFiles := map[string]*descriptorpb.FileDescriptorProto{}
	for _, file := range spkg.ProtoFiles {
		protoFiles[file.GetName()] = file
	}

	deps, err := proto.ResolveDependencies(protoFiles)
	if err != nil {
		return fmt.Errorf("resolving dependencies: %w", err)
	}

	protoFileOverride := sflags.MustGetString(cmd, "proto-file-override")
	if protoFileOverride != "" {
		parser := protoparse.Parser{
			ImportPaths:           []string{},
			IncludeSourceCodeInfo: true,
			LookupImport: func(filename string) (*desc.FileDescriptor, error) {
				if fd, exists := deps[filename]; exists {
					return fd, nil
				}
				return nil, fmt.Errorf("import %q not found", filename)
			},
		}

		fds, err := parser.ParseFiles(protoFileOverride)
		if err != nil {
			return fmt.Errorf("parsing proto file override %q: %w", protoFileOverride, err)
		}
		if len(fds) == 0 {
			return fmt.Errorf("no file descriptors found in proto file override %q", protoFileOverride)
		}
		fileDescriptor = fds[0]
	} else {
		fileDescriptor, err = proto.FileDescriptorForOutputType(spkg, deps, outputType)
		if err != nil {
			return fmt.Errorf("finding file descriptor for output type %q: %w", outputType, err)
		}
	}

	useProtoOption := false
	for _, descriptor := range fileDescriptor.GetDependencies() {
		if descriptor.GetName() == "sf/substreams/sink/sql/schema/v1/schema.proto" {
			useProtoOption = true
		}
	}
	if !useProtoOption {
		useConstraints = false
	}

	var rootMessageDescriptor *desc.MessageDescriptor
	for _, messageDescriptor := range fileDescriptor.GetMessageTypes() {
		if messageDescriptor.GetFullyQualifiedName() == outputType {
			rootMessageDescriptor = messageDescriptor
			break
		}
	}
	if rootMessageDescriptor == nil {
		return fmt.Errorf("message descriptor not found for output type %q. Your substreams need to bundle its protobuf definitions", outputType)
	}

	baseSink, err := sink.NewFromViper(
		cmd,
		outputType,
		manifestPath,
		outputModuleName,
		"sink_from_proto",
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new base sinker: %w", err)
	}

	factory := db_proto.SinkerFactory(baseSink, outputModuleName, rootMessageDescriptor.UnwrapMessage(), db_proto.SinkerFactoryOptions{
		UseProtoOption:  useProtoOption,
		UseConstraints:  useConstraints,
		UseTransactions: true,
		BlockBatchSize:  blockBatchSize,
		Parallel:        false,
		Encoding:        encoding,
		Clickhouse: db_proto.SinkerFactoryClickhouse{
			SinkInfoFolder:  sflags.MustGetString(cmd, "clickhouse-sink-info-folder"),
			CursorFilePath:  sflags.MustGetString(cmd, "clickhouse-cursor-file-path"),
			QueryRetryCount: retryCount,
			QueryRetrySleep: retrySleep,
		},
	})

	sinker, err := factory(cmd.Context(), dsnString, dsn.Schema(), zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating sinker: %w", err)
	}

	app.SuperviseAndStartUsing(sinker, sinker.Run)

	if err := app.WaitForTermination(zlog, 0, 0); err != nil {
		cli.Quit("application terminated with error: %s", err)
	}

	return nil
}
