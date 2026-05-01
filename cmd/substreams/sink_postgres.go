package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/derr"
	sinksqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	dbchangesdb "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	dbchangessinker "github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
	dbproto "github.com/streamingfast/substreams/sink/sql/db_proto"
	dbprotoproto "github.com/streamingfast/substreams/sink/sql/db_proto/proto"
	"github.com/streamingfast/substreams/sink"
	"google.golang.org/protobuf/types/descriptorpb"
)

func init() {
	sink.AddFlagsToSet(sinkPostgresCmd.Flags(),
		sink.FlagExcludeDefault(sink.FlagUndoBufferSize))

	sinkPostgresCmd.Flags().String("on-module-hash-mismatch", "error", "What to do when the module hash in the manifest does not match the one in the database, can be 'error', 'warn' or 'ignore'")
	sinkPostgresCmd.Flags().String("cursors-table", "cursors", "Name of the table to use for storing cursors")
	sinkPostgresCmd.Flags().String("history-table", "substreams_history", "Name of the table to use for storing block history, used to handle reorgs")
	sinkPostgresCmd.Flags().String("bytes-encoding", "raw", "Encoding for protobuf bytes fields: raw, hex, 0xhex, base64, base58")
	sinkPostgresCmd.Flags().Int("batch-block-flush-interval", 1000, "When in catch up mode, flush every N blocks")
	sinkPostgresCmd.Flags().Int("batch-row-flush-interval", 100000, "When in catch up mode, flush every N rows")
	sinkPostgresCmd.Flags().Int("live-block-flush-interval", 1, "When processing in live mode, flush every N blocks")
	sinkPostgresCmd.Flags().Int("flush-retry-count", 3, "Number of retry attempts for flush operations")
	sinkPostgresCmd.Flags().Duration("flush-retry-delay", 1*time.Second, "Base delay for retry backoff on flush failures")
	sinkPostgresCmd.Flags().Bool("no-constraints", false, "Do not add constraints to the database (proto-based mode only)")
	sinkPostgresCmd.Flags().Int("block-batch-size", 25, "Number of blocks to process at a time (proto-based mode only)")

	SinkCmd.AddCommand(sinkPostgresCmd)
}

var sinkPostgresCmd = &cobra.Command{
	Use:   "postgres <dsn> [<manifest> [<module_name>]]",
	Short: "Run a PostgreSQL sink for Substreams",
	RunE:  sinkPostgresE,
	Args:  cobra.RangeArgs(1, 3),
}

func sinkPostgresE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	cmd.SilenceUsage = true

	dsnString := args[0]
	var manifestPath, outputModule string
	if len(args) > 1 {
		manifestPath = args[1]
	}
	if len(args) > 2 {
		outputModule = args[2]
	}

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	sinkerConfig, err := sink.ConfigFromViper(cmd, sink.IgnoreOutputModuleType, manifestPath, outputModule, "sink_postgres", zlog, tracer)
	if err != nil {
		return err
	}

	outputType := strings.TrimPrefix(sinkerConfig.OutputModule.Output.Type, "proto:")

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		<-derr.SetupSignalHandler(0)
		cancel()
	}()

	if strings.Contains(outputType, "DatabaseChanges") {
		return sinkPostgresDatabaseChanges(ctx, cmd, dsnString, sinkerConfig)
	}
	return sinkPostgresProto(ctx, cmd, dsnString, sinkerConfig, outputType)
}

func sinkPostgresDatabaseChanges(ctx context.Context, cmd *cobra.Command, dsnString string, sinkerConfig *sink.SinkerConfig) error {
	dbchangessinker.RegisterMetrics()

	baseSink, err := sink.NewFromConfig(sinkerConfig)
	if err != nil {
		return fmt.Errorf("creating base sinker: %w", err)
	}

	sinkerFactory := dbchangessinker.SinkerFactory(baseSink, dbchangessinker.SinkerFactoryOptions{
		CursorTableName:         sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:        sflags.MustGetString(cmd, "history-table"),
		BatchBlockFlushInterval: sflags.MustGetInt(cmd, "batch-block-flush-interval"),
		BatchRowFlushInterval:   sflags.MustGetInt(cmd, "batch-row-flush-interval"),
		LiveBlockFlushInterval:  sflags.MustGetInt(cmd, "live-block-flush-interval"),
		OnModuleHashMismatch:    sflags.MustGetString(cmd, "on-module-hash-mismatch"),
		HandleReorgs:            true,
		FlushRetryCount:         sflags.MustGetInt(cmd, "flush-retry-count"),
		FlushRetryDelay:         sflags.MustGetDuration(cmd, "flush-retry-delay"),
	})

	sqlSinker, err := sinkerFactory(ctx, dsnString, zlog, tracer)
	if err != nil {
		return fmt.Errorf("unable to setup sql sinker: %w", err)
	}

	sqlSinker.Run(ctx)
	return sqlSinker.Err()
}

func sinkPostgresProto(ctx context.Context, cmd *cobra.Command, dsnString string, sinkerConfig *sink.SinkerConfig, outputType string) error {
	dsn, err := dbchangesdb.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parsing dsn: %w", err)
	}

	spkg := sinkerConfig.Pkg
	protoFiles := make(map[string]*descriptorpb.FileDescriptorProto, len(spkg.ProtoFiles))
	for _, file := range spkg.ProtoFiles {
		protoFiles[file.GetName()] = file
	}

	deps, err := dbprotoproto.ResolveDependencies(protoFiles)
	if err != nil {
		return fmt.Errorf("resolving dependencies: %w", err)
	}

	fileDescriptor, err := dbprotoproto.FileDescriptorForOutputType(spkg, nil, deps, outputType)
	if err != nil {
		return fmt.Errorf("finding file descriptor for output type %q: %w", outputType, err)
	}

	var rootMessageDescriptor *desc.MessageDescriptor
	for _, md := range fileDescriptor.GetMessageTypes() {
		if md.GetFullyQualifiedName() == outputType {
			rootMessageDescriptor = md
			break
		}
	}
	if rootMessageDescriptor == nil {
		return fmt.Errorf("message descriptor not found for output type %q, ensure your substreams bundles its protobuf definitions", outputType)
	}

	useConstraints := !sflags.MustGetBool(cmd, "no-constraints")
	useProtoOption := false
	for _, dep := range fileDescriptor.GetDependencies() {
		if dep.GetName() == "sf/substreams/sink/sql/schema/v1/schema.proto" {
			useProtoOption = true
		}
	}
	if !useProtoOption {
		useConstraints = false
	}

	encodingStr := sflags.MustGetString(cmd, "bytes-encoding")
	encoding, err := sinksqlbytes.ParseEncoding(encodingStr)
	if err != nil {
		return fmt.Errorf("invalid bytes encoding %q: %w", encodingStr, err)
	}

	baseSink, err := sink.NewFromConfig(sinkerConfig)
	if err != nil {
		return fmt.Errorf("creating base sinker: %w", err)
	}

	outputModuleName := sinkerConfig.OutputModule.Name
	factory := dbproto.SinkerFactory(baseSink, outputModuleName, rootMessageDescriptor.UnwrapMessage(), dbproto.SinkerFactoryOptions{
		UseProtoOption:  useProtoOption,
		UseConstraints:  useConstraints,
		UseTransactions: true,
		BlockBatchSize:  sflags.MustGetInt(cmd, "block-batch-size"),
		Parallel:        false,
		Encoding:        encoding,
	})

	dbProtoSinker, err := factory(ctx, dsnString, dsn.Schema(), zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating sinker: %w", err)
	}

	return dbProtoSinker.Run(ctx)
}
