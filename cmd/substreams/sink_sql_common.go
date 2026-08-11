package main

import (
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/logging/zapx"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink"
	sinksql "github.com/streamingfast/substreams/sink/sql"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
	"github.com/streamingfast/substreams/sink/sql/db_proto"
	"github.com/streamingfast/substreams/sink/sql/db_proto/proto"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/buffer"
	"github.com/streamingfast/substreams/sink/sql/services"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/descriptorpb"
)

var supportedOutputTypes = "sf.substreams.sink.database.v1.DatabaseChanges,sf.substreams.database.v1.DatabaseChanges"

var onModuleHashMismatchFlag = "on-module-hash-mismatch"

func addDSNFlag(flags *pflag.FlagSet) {
	flags.String("dsn", "", "Database connection string (falls back to the SUBSTREAMS_SINK_DSN environment variable), e.g. \"psql://user:pass@localhost:5432/db?sslmode=disable\" or \"clickhouse://user:pass@localhost:9000/db\"")
}

func addOperatorFlags(flags *pflag.FlagSet) {
	flags.Duration("delay-before-start", 0, "[Operator] Amount of time to wait before starting any internal processes, can be used to perform maintenance on the pod before actually letting it start")
	flags.String("pprof-listen-addr", "", "[Operator] If non-empty, the process will listen on this address for pprof analysis (see https://golang.org/pkg/net/http/pprof/)")
}

func resolveSinkDSN(cmd *cobra.Command) (string, error) {
	dsn := sflags.MustGetString(cmd, "dsn")
	if dsn == "" {
		dsn = os.Getenv("SUBSTREAMS_SINK_DSN")
	}
	if dsn == "" {
		return "", fmt.Errorf("no DSN provided, use --dsn or set SUBSTREAMS_SINK_DSN (e.g. \"psql://user:pass@localhost:5432/db?sslmode=disable\" or \"clickhouse://user:pass@localhost:9000/db\")")
	}
	return dsn, nil
}

// validateDSNEngine parses the DSN and ensures its driver matches the engine of the
// command being run, so that e.g. a "clickhouse://" DSN cannot be used with the
// postgres commands.
func validateDSNEngine(dsnString, expectedDriver string) (*db.DSN, error) {
	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return nil, fmt.Errorf("parsing dsn: %w", err)
	}
	if dsn.Driver() != expectedDriver {
		return nil, fmt.Errorf("DSN scheme %q does not match command %q", dsn.Driver(), expectedDriver)
	}
	return dsn, nil
}

func sinkDBPreStart(cmd *cobra.Command) {
	if delay := sflags.MustGetDuration(cmd, "delay-before-start"); delay > 0 {
		zlog.Info("sleeping to respect delay before start setting", zapx.HumanDuration("delay", delay))
		time.Sleep(delay)
	}

	if addr := sflags.MustGetString(cmd, "pprof-listen-addr"); addr != "" {
		go func() {
			zlog.Debug("starting pprof server", zap.String("listen_addr", addr))
			if err := http.ListenAndServe(addr, nil); err != nil {
				zlog.Warn("unable to start profiling server", zap.Error(err), zap.String("listen_addr", addr))
			}
		}()
	}
}

func addOnModuleHashMismatchFlag(flags *pflag.FlagSet) {
	flags.String(onModuleHashMismatchFlag, "error", cli.FlagDescription(`
		What to do when the module hash in the manifest does not match the one in the database, can be 'error', 'warn' or 'ignore'

		- If 'error' is used (default), it will exit with an error explaining the problem and how to fix it.
		- If 'warn' is used, it does the same as 'ignore' but it will log a warning message when it happens.
		- If 'ignore' is set, we pick the cursor at the highest block number and use it as the starting point. Subsequent
		updates to the cursor will overwrite the module hash in the database.
	`))
}

func addClusterFlag(flags *pflag.FlagSet) {
	flags.String("cluster", "", "[Operator] If non-empty, a 'ON CLUSTER <cluster>' clause will be applied when setting up tables in Clickhouse. It will also replace the table engine with it's replicated counterpart (MergeTree will be replaced with ReplicatedMergeTree for example).")
}

func addCursorTableFlags(flags *pflag.FlagSet) {
	flags.String("cursors-table", "cursors", "[Operator] Name of the table to use for storing cursors")
	flags.String("history-table", "substreams_history", "[Operator] Name of the table to use for storing block history, used to handle reorgs")
}

func addBytesEncodingFlag(flags *pflag.FlagSet) {
	flags.String("bytes-encoding", "raw", "Encoding for protobuf bytes fields: raw, hex, 0xhex, base64, base58. Non-raw encodings store data as string type in database.")
}

// clusterFlag returns the value of the --cluster flag, or an empty string when the
// flag is not registered on the command (i.e. postgres commands).
func clusterFlag(cmd *cobra.Command) string {
	if sflags.FlagDefined(cmd, "cluster") {
		return sflags.MustGetString(cmd, "cluster")
	}
	return ""
}

// boolFlag returns the value of a boolean flag, or false when the flag is not
// registered on the command.
func boolFlag(cmd *cobra.Command, name string) bool {
	if sflags.FlagDefined(cmd, name) {
		return sflags.MustGetBool(cmd, name)
	}
	return false
}

func isDatabaseChangesType(outputType string) bool {
	unprefixed := strings.TrimPrefix(outputType, "proto:")
	for _, t := range strings.Split(supportedOutputTypes, ",") {
		if unprefixed == t {
			return true
		}
	}
	return false
}

// addDatabaseChangesModeRunFlags registers the run flags that only apply when the
// selected module outputs DatabaseChanges.
func addDatabaseChangesModeRunFlags(flags *pflag.FlagSet) {
	flags.Int("undo-buffer-size", 0, "[DatabaseChanges mode] If non-zero, handling of reorgs in the database is disabled. Instead, a buffer is introduced to only process blocks once they have been confirmed by that many blocks, introducing a latency but slightly reducing the load on the database when close to head. Set to 0 to enable reorg handling in the database (required for some databases like Postgres).")
	flags.Int("batch-block-flush-interval", 1_000, "[DatabaseChanges mode] When in catch up mode, flush every N blocks or after batch-row-flush-interval, whichever comes first. Set to 0 to disable and only use batch-row-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
	flags.Int("batch-row-flush-interval", 100_000, "[DatabaseChanges mode] When in catch up mode, flush every N rows or after batch-block-flush-interval, whichever comes first. Set to 0 to disable and only use batch-block-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
	flags.Int("live-block-flush-interval", 1, "[DatabaseChanges mode] When processing in live mode, flush every N blocks.")
	flags.Int("flush-retry-count", 3, "[DatabaseChanges mode] Number of retry attempts for flush operations")
	flags.Duration("flush-retry-delay", 1*time.Second, "[DatabaseChanges mode] Base delay for incremental retry backoff on flush failures")
	flags.String("cursors-table", "cursors", "[DatabaseChanges mode] Name of the table to use for storing cursors")
	flags.String("history-table", "substreams_history", "[DatabaseChanges mode] Name of the table to use for storing block history, used to handle reorgs")
	flags.String(onModuleHashMismatchFlag, "error", "[DatabaseChanges mode] What to do when the module hash in the manifest does not match the one in the database, can be 'error', 'warn' or 'ignore'")
}

// defaultLocalBufferDir is where the from-proto sink buffers rows when nothing else is
// asked for: a data folder under the working directory, as the other sink commands use
// for their own local state.
const defaultLocalBufferDir = "./localdata/local-buffer"

// addFromProtoModeRunFlags registers the run flags that only apply when the selected
// module outputs an arbitrary protobuf message (relational mappings). The ClickHouse
// specific flags are only registered for the clickhouse engine.
func addFromProtoModeRunFlags(flags *pflag.FlagSet, driver string) {
	flags.Bool("with-constraints", false, "[from-proto mode] Add the schema's primary keys, unique and foreign key constraints, creating the ones an earlier run left out. This prevents the performance enhancements the sink relies on: multi-row inserts and the local buffer are both disabled while constraints are on, which slows a large initial sync down considerably, so we recommend leaving it off until the sink is close to chain HEAD. Enabling the SQL constraints can also take a long time on a populated database, locking the tables while it runs.")
	flags.Int("block-batch-size", 25, "[from-proto mode] number of blocks to process at a time")
	flags.String("proto-file-override", "", "[from-proto mode] Override protobuf file to use instead of extracting from substreams package")

	if driver == "postgres" {
		flags.String("local-buffer", defaultLocalBufferDir, "[from-proto mode] Buffer rows in this directory and load them with binary COPY from a background goroutine, so the stream never waits on the database. An empty value disables it, and so does --with-constraints")
		flags.String("local-buffer-max-size", "8GiB", "[from-proto mode] Disk budget for --local-buffer; the stream is held once the buffer reaches it")
	}

	if driver == "clickhouse" {
		flags.String("sink-info-folder", "", "[from-proto mode] folder where to store the clickhouse sink info")
		flags.String("cursor-file-path", "cursor.txt", "[from-proto mode] file name where to store the clickhouse cursor")
		flags.Int("query-retry-count", 3, "[from-proto mode] Number of retries for ClickHouse queries when an error occurs")
		flags.Duration("query-retry-sleep", time.Second, "[from-proto mode] Sleep duration between ClickHouse query retries (e.g. 1s, 500ms)")
	}
}

// addSinkRunFlags registers the full set of flags used by the sink process of an
// engine command.
func addSinkRunFlags(flags *pflag.FlagSet, driver string) {
	sink.AddFlagsToSet(flags, sink.FlagExcludeDefault(sink.FlagUndoBufferSize))
	addBytesEncodingFlag(flags)
	if driver == "clickhouse" {
		addClusterFlag(flags)
	}
	addDatabaseChangesModeRunFlags(flags)
	addFromProtoModeRunFlags(flags, driver)
}

// sinkBytesEncoding resolves the --bytes-encoding flag into a concrete encoding.
func sinkBytesEncoding(cmd *cobra.Command) (sqlbytes.Encoding, error) {
	encodingStr := sflags.MustGetString(cmd, "bytes-encoding")
	encoding, err := sqlbytes.ParseEncoding(encodingStr)
	if err != nil {
		return 0, fmt.Errorf("invalid bytes encoding %q: %w", encodingStr, err)
	}
	return encoding, nil
}

// sinkEngineParentArgs validates the engine command's positional args. The command
// runs the sink directly, so a removed subcommand name would otherwise be taken as
// a manifest path and fail with a confusing registry error.
func sinkEngineParentArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.RangeArgs(0, 2)(cmd, args); err != nil {
		return err
	}
	if len(args) > 0 {
		switch args[0] {
		case "create-user":
			return fmt.Errorf("the create-user command was removed, create the user directly in your database")
		case "run":
			return fmt.Errorf("there is no run subcommand, the command runs the sink directly: substreams sink %s <manifest>", cmd.Name())
		}
	}
	return nil
}

// newSinkRunE builds the RunE for the given engine command. It resolves
// the manifest/module positional params, reads the package once to detect the output
// module type and routes to the DatabaseChanges or the from-proto sink accordingly.
func newSinkRunE(driver string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		dsnString, err := resolveSinkDSN(cmd)
		if err != nil {
			return err
		}
		if _, err := validateDSNEngine(dsnString, driver); err != nil {
			return err
		}

		manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
		if err != nil {
			return err
		}
		if outputModule == "" {
			outputModule = sink.InferOutputModuleFromPackage
		}

		sink.LoadSubstreamsAuthEnvFile(manifestPath)

		// TODO: this initial read is only used to detect the output module type so we can
		// route to the right sink; the DatabaseChanges path then reads the manifest again
		// inside sink.NewFromViper. Threading the package through would require a sink
		// library change, so the extra read is accepted for now.
		spkg, module, _, err := sink.ReadManifestAndModule(manifestPath, "", nil, outputModule, sink.IgnoreOutputModuleType, false, nil, zlog)
		if err != nil {
			return fmt.Errorf("reading manifest: %w", err)
		}

		if isDatabaseChangesType(module.Output.Type) {
			return runDatabaseChangesSink(cmd, manifestPath, outputModule, dsnString)
		}

		return runFromProtoSink(cmd, driver, manifestPath, dsnString, spkg, module.Name)
	}
}

func runDatabaseChangesSink(cmd *cobra.Command, manifestPath, outputModule, dsnString string) error {
	sinkDBPreStart(cmd)

	app := cli.NewApplication(cmd.Context())

	sinker.RegisterMetrics()

	baseSink, err := sink.NewFromViper(
		cmd,
		supportedOutputTypes,
		manifestPath,
		outputModule,
		"sink_database_changes",
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new base sinker: %w", err)
	}

	sinkerFactory := sinker.SinkerFactory(baseSink, sinker.SinkerFactoryOptions{
		CursorTableName:         sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:        sflags.MustGetString(cmd, "history-table"),
		ClickhouseCluster:       clusterFlag(cmd),
		BatchBlockFlushInterval: sflags.MustGetInt(cmd, "batch-block-flush-interval"),
		BatchRowFlushInterval:   sflags.MustGetInt(cmd, "batch-row-flush-interval"),
		LiveBlockFlushInterval:  sflags.MustGetInt(cmd, "live-block-flush-interval"),
		OnModuleHashMismatch:    sflags.MustGetString(cmd, onModuleHashMismatchFlag),
		HandleReorgs:            sflags.MustGetInt(cmd, "undo-buffer-size") == 0,
		FlushRetryCount:         sflags.MustGetInt(cmd, "flush-retry-count"),
		FlushRetryDelay:         sflags.MustGetDuration(cmd, "flush-retry-delay"),
	})

	sqlSinker, err := sinkerFactory(app.Context(), dsnString, zlog, tracer)
	if err != nil {
		return fmt.Errorf("unable to setup sql sinker: %w", err)
	}

	app.SuperviseAndStart(sqlSinker)

	return app.WaitForTermination(zlog, 0*time.Second, 30*time.Second)
}

func runFromProtoSink(cmd *cobra.Command, driver, manifestPath, dsnString string, spkg *pbsubstreams.Package, outputModuleName string) error {
	useConstraints := sflags.MustGetBool(cmd, "with-constraints")
	blockBatchSize := sflags.MustGetInt(cmd, "block-batch-size")

	encoding, err := sinkBytesEncoding(cmd)
	if err != nil {
		return err
	}

	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parsing dsn: %w", err)
	}

	sinkDBPreStart(cmd)

	app := cli.NewApplication(cmd.Context())

	if service, err := sinksql.ExtractSinkService(spkg); err == nil {
		if err := services.Run(service, zlog); err != nil {
			return fmt.Errorf("running service: %w", err)
		}
	}

	protoFileOverride := sflags.MustGetString(cmd, "proto-file-override")
	rootMessageDescriptor, outputType, useProtoOption, err := resolveFromProtoRootMessage(spkg, outputModuleName, protoFileOverride)
	if err != nil {
		return err
	}
	if !useProtoOption {
		useConstraints = false
	}
	warnAboutConstraints(useConstraints)

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

	localBuffer, err := fromProtoLocalBufferOptions(cmd, driver, useConstraints)
	if err != nil {
		return err
	}

	factory := db_proto.SinkerFactory(baseSink, outputModuleName, rootMessageDescriptor.UnwrapMessage(), db_proto.SinkerFactoryOptions{
		LocalBuffer:     localBuffer,
		UseProtoOption:  useProtoOption,
		UseConstraints:  useConstraints,
		UseTransactions: true,
		BlockBatchSize:  blockBatchSize,
		Encoding:        encoding,
		Clickhouse:      fromProtoClickhouseOptions(cmd, driver),
	})

	sqlSinker, err := factory(cmd.Context(), dsnString, dsn.Schema(), zlog, tracer)
	if err != nil {
		return fmt.Errorf("creating sinker: %w", err)
	}

	app.SuperviseAndStartUsing(sqlSinker, sqlSinker.Run)

	if err := app.WaitForTermination(zlog, 0, 0); err != nil {
		cli.Quit("application terminated with error: %s", err)
	}

	return nil
}

// resolveFromProtoRootMessage extracts, from the substreams package, the root protobuf
// message descriptor for the module's output type together with the output type name and
// whether the schema.proto annotation is bundled (which enables database constraints). It
// mirrors the descriptor resolution performed by the from-proto run path so the run and
// setup commands share a single resolution.
func resolveFromProtoRootMessage(spkg *pbsubstreams.Package, outputModuleName, protoFileOverride string) (rootMessageDescriptor *desc.MessageDescriptor, outputType string, useProtoOption bool, err error) {
	outputType = proto.ModuleOutputType(spkg, outputModuleName)
	if outputType == "" {
		return nil, "", false, fmt.Errorf("could not find output type for module %s", outputModuleName)
	}

	protoFiles := map[string]*descriptorpb.FileDescriptorProto{}
	for _, file := range spkg.ProtoFiles {
		protoFiles[file.GetName()] = file
	}

	deps, err := proto.ResolveDependencies(protoFiles)
	if err != nil {
		return nil, "", false, fmt.Errorf("resolving dependencies: %w", err)
	}

	var fileDescriptor *desc.FileDescriptor
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
			return nil, "", false, fmt.Errorf("parsing proto file override %q: %w", protoFileOverride, err)
		}
		if len(fds) == 0 {
			return nil, "", false, fmt.Errorf("no file descriptors found in proto file override %q", protoFileOverride)
		}
		fileDescriptor = fds[0]
	} else {
		fileDescriptor, err = proto.FileDescriptorForOutputType(spkg, deps, outputType)
		if err != nil {
			return nil, "", false, fmt.Errorf("finding file descriptor for output type %q: %w", outputType, err)
		}
	}

	for _, descriptor := range fileDescriptor.GetDependencies() {
		if descriptor.GetName() == "sf/substreams/sink/sql/schema/v1/schema.proto" {
			useProtoOption = true
		}
	}

	for _, messageDescriptor := range fileDescriptor.GetMessageTypes() {
		if messageDescriptor.GetFullyQualifiedName() == outputType {
			rootMessageDescriptor = messageDescriptor
			break
		}
	}
	if rootMessageDescriptor == nil {
		return nil, "", false, fmt.Errorf("message descriptor not found for output type %q. Your substreams need to bundle its protobuf definitions", outputType)
	}

	return rootMessageDescriptor, outputType, useProtoOption, nil
}

// fromProtoClickhouseOptions collects the ClickHouse-specific options needed to construct
// the from-proto database. It returns the zero value for non-clickhouse engines.
func fromProtoClickhouseOptions(cmd *cobra.Command, driver string) db_proto.SinkerFactoryClickhouse {
	if driver != "clickhouse" {
		return db_proto.SinkerFactoryClickhouse{}
	}
	return db_proto.SinkerFactoryClickhouse{
		SinkInfoFolder:  sflags.MustGetString(cmd, "sink-info-folder"),
		CursorFilePath:  sflags.MustGetString(cmd, "cursor-file-path"),
		QueryRetryCount: sflags.MustGetInt(cmd, "query-retry-count"),
		QueryRetrySleep: sflags.MustGetDuration(cmd, "query-retry-sleep"),
	}
}

func newSinkSetupE(driver string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		dsnString, err := resolveSinkDSN(cmd)
		if err != nil {
			return err
		}
		if _, err := validateDSNEngine(dsnString, driver); err != nil {
			return err
		}

		manifestPath := args[0]

		sinkDBPreStart(cmd)

		sink.LoadSubstreamsAuthEnvFile(manifestPath)

		spkg, module, _, err := sink.ReadManifestAndModule(manifestPath, "", nil, sink.InferOutputModuleFromPackage, sink.IgnoreOutputModuleType, false, nil, zlog)
		if err != nil {
			return fmt.Errorf("reading manifest: %w", err)
		}

		if isDatabaseChangesType(module.Output.Type) {
			return runDatabaseChangesSetup(cmd, dsnString, spkg)
		}

		return runFromProtoSetup(cmd, driver, dsnString, spkg, module.Name)
	}
}

func runDatabaseChangesSetup(cmd *cobra.Command, dsnString string, spkg *pbsubstreams.Package) error {
	options := sinker.SinkerSetupOptions{
		CursorTableName:            sflags.MustGetString(cmd, "cursors-table"),
		HistoryTableName:           sflags.MustGetString(cmd, "history-table"),
		ClickhouseCluster:          clusterFlag(cmd),
		OnModuleHashMismatch:       sflags.MustGetString(cmd, onModuleHashMismatchFlag),
		SystemTablesOnly:           sflags.MustGetBool(cmd, "system-tables-only"),
		IgnoreDuplicateTableErrors: sflags.MustGetBool(cmd, "ignore-duplicate-table-errors"),
		Postgraphile:               boolFlag(cmd, "postgraphile"),
	}

	return sinker.SinkerSetup(cmd.Context(), dsnString, spkg, options, zlog, tracer)
}

// runFromProtoSetup performs the from-proto "update database schema" step: it resolves the
// schema from the module's output proto, connects using the DSN and ensures the database
// schema exists via db_proto.SetupDatabaseSchema, then exits without starting a sinker. It
// is idempotent thanks to the sink-info guard inside SetupDatabaseSchema.
func runFromProtoSetup(cmd *cobra.Command, driver, dsnString string, spkg *pbsubstreams.Package, outputModuleName string) error {
	warnIgnoredDatabaseChangesSetupFlags(cmd)

	useConstraints := boolFlag(cmd, "with-constraints")

	encoding, err := sinkBytesEncoding(cmd)
	if err != nil {
		return err
	}

	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parsing dsn: %w", err)
	}

	protoFileOverride := sflags.MustGetString(cmd, "proto-file-override")
	rootMessageDescriptor, _, useProtoOption, err := resolveFromProtoRootMessage(spkg, outputModuleName, protoFileOverride)
	if err != nil {
		return err
	}
	if !useProtoOption {
		useConstraints = false
	}
	warnAboutConstraints(useConstraints)

	options := db_proto.SinkerFactoryOptions{
		UseProtoOption: useProtoOption,
		UseConstraints: useConstraints,
		Encoding:       encoding,
		Clickhouse:     fromProtoClickhouseOptions(cmd, driver),
	}

	if _, err := db_proto.SetupDatabaseSchema(cmd.Context(), dsnString, dsn.Schema(), outputModuleName, rootMessageDescriptor.UnwrapMessage(), options, zlog, tracer); err != nil {
		return fmt.Errorf("setting up database schema: %w", err)
	}

	zlog.Info("database schema setup completed", zap.String("schema", dsn.Schema()), zap.Bool("constraints", useConstraints))
	return nil
}

// warnIgnoredDatabaseChangesSetupFlags logs a warning for each DatabaseChanges-only setup
// flag that the user explicitly set, since those flags have no effect in from-proto mode.
func warnIgnoredDatabaseChangesSetupFlags(cmd *cobra.Command) {
	for _, name := range []string{"postgraphile", "system-tables-only", "ignore-duplicate-table-errors"} {
		if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
			zlog.Warn("flag has no effect in from-proto setup mode and is ignored", zap.String("flag", name))
		}
	}
}

// newSinkToolsCmd builds the `tools` command subtree for the given engine. The
// ClickHouse cluster flag is only registered for the clickhouse engine.
func newSinkToolsCmd(driver string) *cobra.Command {
	toolsCmd := &cobra.Command{
		Use:   "tools",
		Short: "Tools for developers and operators",
	}

	cursorCmd := &cobra.Command{
		Use:   "cursor",
		Short: "Tools related to cursor handling (read/write)",
	}

	readCmd := &cobra.Command{
		Use:   "read",
		Short: "[Operator] Read active cursor(s) from database, if present",
		Long: cli.Dedent(`
			This command is going to fetch all known cursors from the database. In the database,
			a cursor is saved per module's hash which mean if you update your '.spkg', you might
			end up with multiple cursors for different module.

			This command will list all of them.
		`),
		RunE: toolsReadCursorE,
	}

	writeCmd := &cobra.Command{
		Use:   "write <module_hash> <cursor>",
		Short: "[Operator] Write a new active cursor for a given module's hash in the database",
		Long: cli.Dedent(`
			**Warning** This can screw up the sink's state, use only if you know what you are doing.

			This command is going to write a new cursor in the database for the given module's
			hash. The command update the current cursor if it exists or insert a new one if
			none already exist.
		`),
		Args: cobra.ExactArgs(2),
		RunE: toolsWriteCursorE,
	}

	deleteCmd := &cobra.Command{
		Use:   "delete [<module_hash>]",
		Short: "[Operator] Delete the active cursor for a given module's hash in the database",
		Long: cli.Dedent(`
			**Warning** This can screw up the sink's state, use only if you know what you are doing.

			This command is going to delete the cursor in the database for the given module's
			hash. If the cursor does not exist, the command assume it is correctly deleted.
		`),
		Args: cobra.RangeArgs(0, 1),
		RunE: toolsDeleteCursorE,
	}

	for _, c := range []*cobra.Command{readCmd, writeCmd, deleteCmd} {
		addCursorTableFlags(c.Flags())
		if driver == "clickhouse" {
			addClusterFlag(c.Flags())
		}
	}
	deleteCmd.Flags().BoolP("all", "a", false, "Delete all active cursors")

	cursorCmd.AddCommand(readCmd, writeCmd, deleteCmd)
	toolsCmd.AddCommand(cursorCmd)

	return toolsCmd
}

func toolsReadCursorE(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}

	sinkDBPreStart(cmd)

	loader, err := toolsCreateLoader(cmd, dsnString)
	if err != nil {
		return err
	}

	out, err := loader.GetAllCursors(cmd.Context())
	cli.NoError(err, "Unable to get all cursors")

	if len(out) == 0 {
		fmt.Println("No cursor(s) present in the database")
		return nil
	}

	for id, cursor := range out {
		fmt.Printf("Module %s: Block %s [%s]\n", id, cursor.Block(), cursorToShortString(cursor))
	}

	return nil
}

func toolsWriteCursorE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}

	moduleHash := args[0]
	opaqueCursor := args[1]

	cli.Ensure(moduleHash != "", "The <module_hash> cannot be empty")
	cli.Ensure(len(moduleHash) == 40, "The <module_hash> must be exactly 40 characters long")

	cursor, err := sink.NewCursor(opaqueCursor)
	cli.NoError(err, "The <cursor> is invalid")

	sinkDBPreStart(cmd)

	loader, err := toolsCreateLoader(cmd, dsnString)
	if err != nil {
		return err
	}

	err = loader.UpdateCursor(cmd.Context(), nil, moduleHash, cursor)
	if err != nil {
		if errors.Is(err, db.ErrCursorNotFound) {
			err = loader.InsertCursor(cmd.Context(), moduleHash, cursor)
			cli.NoError(err, "Unable to insert cursor")
		}

		cli.NoError(err, "Unable to update cursor")
	}

	fmt.Println("Cursor written successfully")
	fmt.Printf("- Block %s\n", cursor.Block())
	fmt.Printf("- Head Block %s\n", cursor.HeadBlock)
	fmt.Printf("- LIB Block %s\n", cursor.LIB)
	fmt.Printf("- Step %q\n", cursor.Step)
	fmt.Printf("- Cursor %q\n", cursorToShortString(cursor))
	return nil
}

func toolsDeleteCursorE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}

	moduleHash := ""
	if !sflags.MustGetBool(cmd, "all") {
		cli.Ensure(len(args) == 1, "Module hash is required, if you want to delete all cursors, use --all to avoid specifying a module's hash")

		moduleHash = args[0]

		cli.Ensure(moduleHash != "", "The <module_hash> cannot be empty")
		cli.Ensure(len(moduleHash) == 40, "The <module_hash> must be exactly 40 characters long")
	}

	sinkDBPreStart(cmd)

	loader, err := toolsCreateLoader(cmd, dsnString)
	if err != nil {
		return err
	}

	if moduleHash == "" {
		deletedCount, err := loader.DeleteAllCursors(cmd.Context())
		cli.NoError(err, "Unable to delete cursor")

		fmt.Printf("Deleted %d cursor(s) successfully\n", deletedCount)
	} else {
		err := loader.DeleteCursor(cmd.Context(), moduleHash)
		if err != nil && !errors.Is(err, db.ErrCursorNotFound) {
			cli.NoError(err, "Unable to delete cursor")
		}

		fmt.Println("Cursor delete successfully")
	}

	return nil
}

func toolsCreateLoader(cmd *cobra.Command, dsnString string) (*db.Loader, error) {
	cursorTableName := sflags.MustGetString(cmd, "cursors-table")
	historyTableName := sflags.MustGetString(cmd, "history-table")

	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	handleReorgs := false
	loader, err := db.NewLoader(
		dsn,
		cursorTableName,
		historyTableName,
		clusterFlag(cmd),
		0, 0, 0,
		db.OnModuleHashMismatchError.String(),
		&handleReorgs,
		zlog, tracer,
	)
	if err != nil {
		return nil, fmt.Errorf("creating loader: %w", err)
	}

	if err := loader.LoadTables(dsn.Schema(), cursorTableName, historyTableName); err != nil {
		var systemTableError *db.SystemTableError
		if errors.As(err, &systemTableError) {
			fmt.Printf("Error validating the system table: %s\n", systemTableError)
			fmt.Println("Did you run setup ?")
			os.Exit(1)
		}

		cli.NoError(err, "Unable to load table information from database")
	}

	return loader, nil
}

func cursorToShortString(in *sink.Cursor) string {
	cursor := in.String()
	if len(cursor) > 12 {
		cursor = cursor[0:6] + "..." + cursor[len(cursor)-6:]
	}

	return cursor
}

// warnAboutConstraints says out loud what --with-constraints costs, since the flag turns
// off the two things that make a large sync bearable and its effect on an already
// populated database is not instantaneous.
func warnAboutConstraints(useConstraints bool) {
	if !useConstraints {
		return
	}

	zlog.Warn("constraints are enabled, which disables the sink's performance enhancements: rows are inserted one at a time and the local buffer is off. " +
		"A large initial sync is considerably slower this way, so keep --with-constraints off until the sink is close to chain HEAD. " +
		"Enabling the constraints themselves can also take a long time on a populated database, and locks the tables while it runs")
}

// fromProtoLocalBufferOptions resolves the --local-buffer flags into buffer options, or nil
// when the buffer is off.
func fromProtoLocalBufferOptions(cmd *cobra.Command, driver string, useConstraints bool) (*buffer.Options, error) {
	if driver != "postgres" {
		return nil, nil
	}

	if useConstraints {
		// Binary COPY feeds one table at a time, so a child row can reach the server
		// before its parent within a segment, which an immediate foreign key rejects.
		// The buffer is therefore off with constraints — silently when it is only on by
		// default, but never behind the back of someone who asked for it.
		for _, name := range []string{"local-buffer", "local-buffer-max-size"} {
			if cmd.Flags().Changed(name) {
				return nil, fmt.Errorf("--%s cannot be combined with --with-constraints: the buffer loads a table at a time with binary COPY, which a foreign key rejects when a child row reaches the server before its parent", name)
			}
		}

		return nil, nil
	}

	dir := sflags.MustGetString(cmd, "local-buffer")
	if dir == "" {
		return nil, nil
	}

	rawSize := sflags.MustGetString(cmd, "local-buffer-max-size")
	maxBytes, err := humanize.ParseBytes(rawSize)
	if err != nil {
		return nil, fmt.Errorf("invalid --local-buffer-max-size %q: %w", rawSize, err)
	}

	return &buffer.Options{Dir: dir, MaxBytes: int64(maxBytes)}, nil
}
