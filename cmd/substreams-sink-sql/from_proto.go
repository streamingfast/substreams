package main

import (
	"fmt"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/cli"
	. "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	sinksql "github.com/streamingfast/substreams/sink/sql"
	"github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	"github.com/streamingfast/substreams/sink/sql/db_proto/proto"
	pbsql "github.com/streamingfast/substreams/sink/sql/pb/sf/substreams/sink/sql/services/v1"
	"github.com/streamingfast/substreams/sink/sql/services"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	"google.golang.org/protobuf/types/descriptorpb"
)

var fromProtoCmd = Command(fromProtoE,
	"from-proto <dsn> <manifest> [output-module]",
	"",
	RangeArgs(2, 3),
	Flags(func(flags *pflag.FlagSet) {
		sink.AddFlagsToSet(flags, sink.FlagExcludeDefault("undo-buffer-size"))
		// Deprecated: use --endpoint instead
		flags.String("substreams-endpoint", "", "")
		flags.MarkHidden("substreams-endpoint")
		flags.MarkDeprecated("substreams-endpoint", "use --endpoint instead")

		flags.Bool("no-constraints", false, "Do not add any constraints to the database. This is useful to speed up the initial import of a large dataset.")
		//flags.Bool("no-proto-option", false, "this tell the schema manager to not rely on proto option to generate the schema.")
		//flags.Bool("no-transactions", false, "Do not use transactions when inserting data. This is useful to speed up the initial import of a large dataset.")
		//flags.Bool("parallel", false, "Run the sinker in parallel mode. This is useful to speed up the initial import of a large dataset. This is will process blocks of a batch in parallel")
		flags.Int("block-batch-size", 25, "number of blocks to process at a time")
		flags.String("clickhouse-sink-info-folder", "", "folder where to store the clickhouse sink info")
		flags.String("clickhouse-cursor-file-path", "cursor.txt", "file name where to store the clickhouse cursor")
		flags.String("bytes-encoding", "raw", "Encoding for protobuf bytes fields (raw, hex, 0xhex, base64, base58)")
		flags.String("proto-file-override", "", "Override protobuf file to use instead of extracting from substreams package")
		flags.Int("clickhouse-query-retry-count", 3, "Number of retries for ClickHouse queries when an error occurs")
		flags.Duration("clickhouse-query-retry-sleep", time.Second, "Sleep duration between ClickHouse query retries (e.g. 1s, 500ms)")
	}),
)

//now
//todo: add a validator on top of schema to validate all the relations

// Later
//todo: migration tool
//todo: add index support
//todo: post generate index
//todo: external process
//todo: handle network

func fromProtoE(cmd *cobra.Command, args []string) error {
	app := cli.NewApplication(cmd.Context())

	dsnString := args[0]
	manifestPath := args[1]

	outputModuleName := sink.InferOutputModuleFromPackage
	if len(args) == 3 {
		outputModuleName = args[2]
	}

	useConstraints := !sflags.MustGetBool(cmd, "no-constraints")
	blockBatchSize := sflags.MustGetInt(cmd, "block-batch-size")

	encodingStr := sflags.MustGetString(cmd, "bytes-encoding")
	encoding, err := bytes.ParseEncoding(encodingStr)
	if err != nil {
		return fmt.Errorf("invalid bytes encoding %q: %w", encodingStr, err)
	}

	useTransactions := true
	parallel := false

	retryCount := sflags.MustGetInt(cmd, "clickhouse-query-retry-count")
	retrySleep := sflags.MustGetDuration(cmd, "clickhouse-query-retry-sleep")

	// Support deprecated --substreams-endpoint flag for backward compatibility
	if value, valueProvided := sflags.MustGetStringProvided(cmd, "substreams-endpoint"); valueProvided {
		if err := cmd.Flags().Set("endpoint", value); err != nil {
			return fmt.Errorf("setting endpoint flag from substreams-endpoint: %w", err)
		}
	}

	endpoint := sflags.MustGetString(cmd, "endpoint")

	if endpoint == "" {
		network := sflags.MustGetString(cmd, "network")
		if network == "" {
			reader, err := manifest.NewReader(manifestPath)
			if err != nil {
				return fmt.Errorf("setup manifest reader: %w", err)
			}
			pkgBundle, err := reader.Read()
			if err != nil {
				return fmt.Errorf("read manifest: %w", err)
			}
			network = pkgBundle.Package.Network
		}
		var err error
		endpoint, err = manifest.ExtractNetworkEndpoint(network, "", zlog)
		if err != nil {
			return err
		}
	}

	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parsing dsn: %w", err)
	}

	//todo: handle params
	spkg, module, _, err := sink.ReadManifestAndModule(manifestPath, "", nil, outputModuleName, "", false, nil, zlog)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	outputModuleName = module.Name
	outputType := proto.ModuleOutputType(spkg, outputModuleName)
	if outputType == "" {
		return fmt.Errorf("could not find output type for module %s", outputModuleName)
	}

	service, err := sinksql.ExtractSinkService(spkg)
	if err != nil {
		service = &pbsql.Service{}
	}

	err = services.Run(service, zlog)
	if err != nil {
		return fmt.Errorf("running service: %w", err)
	}

	var fileDescriptor *desc.FileDescriptor

	// Use original logic to extract from substreams package
	protoFiles := map[string]*descriptorpb.FileDescriptorProto{}
	for _, file := range spkg.ProtoFiles {
		protoFiles[file.GetName()] = file
	}

	deps, err := proto.ResolveDependencies(protoFiles)
	if err != nil {
		return fmt.Errorf("resolving dependencies: %w", err)
	}

	// Check if proto-file-override flag is provided
	protoFileOverride := sflags.MustGetString(cmd, "proto-file-override")
	if protoFileOverride != "" {
		// Load file descriptor from the override proto file
		// Include dependencies from the substreams package to resolve imports
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
		fileDescriptor, err = proto.FileDescriptorForOutputType(spkg, err, deps, outputType)
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
		name := messageDescriptor.GetFullyQualifiedName()
		if name == outputType {
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
		fmt.Sprintf("substreams-sink-sql/%s", version),
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new base sinker: %w", err)
	}

	factory := db_proto.SinkerFactory(baseSink, outputModuleName, rootMessageDescriptor.UnwrapMessage(), db_proto.SinkerFactoryOptions{
		UseProtoOption:  useProtoOption,
		UseConstraints:  useConstraints,
		UseTransactions: useTransactions,
		BlockBatchSize:  blockBatchSize,
		Parallel:        parallel,
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
