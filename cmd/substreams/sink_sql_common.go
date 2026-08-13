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
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/spool"
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

// stringSliceFlag mirrors boolFlag for the per-table constraint switches, which are only
// registered for the from-proto commands.
func stringFlag(cmd *cobra.Command, name string) string {
	if sflags.FlagDefined(cmd, name) {
		return sflags.MustGetString(cmd, name)
	}

	return ""
}

// intFlag returns the value of an int flag, or zero when the flag is not registered on
// the command.
func intFlag(cmd *cobra.Command, name string) int {
	if sflags.FlagDefined(cmd, name) {
		return sflags.MustGetInt(cmd, name)
	}

	return 0
}

func stringSliceFlag(cmd *cobra.Command, name string) []string {
	if sflags.FlagDefined(cmd, name) {
		return sflags.MustGetStringSlice(cmd, name)
	}

	return nil
}

// sinkManifestAndModule reads the positional arguments the schema-side commands take.
//
// The module is optional and inferred from the package when it is left out, exactly as the
// run command does — but a package with more than one candidate has to be told which, or
// `setup` and `constraints` would derive a schema from a different module than the run
// they are meant to accompany.
func sinkManifestAndModule(args []string) (manifestPath string, outputModule string) {
	if len(args) > 1 && args[1] != "" {
		return args[0], args[1]
	}

	return args[0], sink.InferOutputModuleFromPackage
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
	flags.Int("undo-buffer-size", 0, "If non-zero, handling of reorgs in the database is disabled. Instead, a buffer is introduced to only process blocks once they have been confirmed by that many blocks, introducing a latency but slightly reducing the load on the database when close to head. Set to 0 to enable reorg handling in the database (required for some databases like Postgres).")
	flags.Int("batch-block-flush-interval", 1_000, "When in catch up mode, flush every N blocks or after batch-row-flush-interval, whichever comes first. Set to 0 to disable and only use batch-row-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
	flags.Int("batch-row-flush-interval", 100_000, "When in catch up mode, flush every N rows or after batch-block-flush-interval, whichever comes first. Set to 0 to disable and only use batch-block-flush-interval. Ineffective if the sink is now in the live portion of the chain where only 'live-block-flush-interval' applies.")
	flags.Int("live-block-flush-interval", 1, "When processing in live mode, flush every N blocks.")
	flags.Int("flush-retry-count", 3, "Number of retry attempts for flush operations")
	flags.Duration("flush-retry-delay", 1*time.Second, "Base delay for incremental retry backoff on flush failures")
	flags.String("cursors-table", "cursors", "Name of the table to use for storing cursors")
	flags.String("history-table", "substreams_history", "Name of the table to use for storing block history, used to handle reorgs")
	flags.String(onModuleHashMismatchFlag, "error", "What to do when the module hash in the manifest does not match the one in the database, can be 'error', 'warn' or 'ignore'")
}

// defaultSpoolDir is where the from-proto sink spools rows when nothing else is asked
// for: a data folder under the working directory, as the other sink commands use for
// their own local state.
const defaultSpoolDir = "./localdata/spool"

// fromProtoSchemaFlagNames are the from-proto flags that describe the schema itself, as
// opposed to how the sink writes to it. They are the ones `setup` and `constraints` need
// too: all three commands have to agree on which constraints the schema is meant to have.
var fromProtoSchemaFlagNames = []string{
	"disable-foreign-keys",
	"disable-primary-keys",
	"disable-unique-constraints",
	"disable-block-number-index",
	"no-constraints",
	"proto-file-override",
}

// fromProtoRunFlagNames are the from-proto flags that only mean something while the sink
// is running. Registering them anywhere else puts knobs on commands that never decode a
// block or write a row.
var fromProtoRunFlagNames = []string{
	"apply-constraints",
	"write-mode",
	"decode-workers",
	"decode-batch-size",
	"db-write-target-duration",
	"db-write-max-size",
	"spool-dir",
	"spool-max-size",
	"spool-max-idle",
	"block-batch-size",
	"sink-info-folder",
	"cursor-file-path",
	"query-retry-count",
	"query-retry-sleep",
}

// databaseChangesFlagNames are the run flags that only mean something when the module
// outputs DatabaseChanges.
var databaseChangesFlagNames = []string{
	"undo-buffer-size",
	"batch-block-flush-interval",
	"batch-row-flush-interval",
	"live-block-flush-interval",
	"flush-retry-count",
	"flush-retry-delay",
	"cursors-table",
	"history-table",
}

// addFromProtoSchemaFlags registers the from-proto flags that describe the schema. They
// go on the run command, on `setup` and on `constraints`, which all have to agree on
// which constraints the schema is meant to have.
// addConstraintPassFlags registers what governs how the constraint pass runs, as opposed
// to which constraints it creates. Only the `constraints` commands carry it: the run
// creates them one at a time, which is the safe shape, and anything more deliberate is
// what the command is for.
func addConstraintPassFlags(flags *pflag.FlagSet) {
	flags.Int("constraints-per-transaction", 1, "How many constraints are created or dropped per transaction. Building an index and validating a foreign key are the two most memory-hungry things the sink asks of the server, so they go one at a time by default: a run killed part-way keeps what it finished, and the next one carries on. Raise it to trade that back for fewer round trips on a schema whose constraints are small.")
}

func addFromProtoSchemaFlags(flags *pflag.FlagSet) {
	flags.Bool("disable-foreign-keys", false, "Never create foreign keys, including the one every table has to the block table. They are what a load pays most for, and without them a reorg is undone by deleting from each table rather than by a cascade.")
	flags.StringSlice("disable-primary-keys", nil, "Tables that go without a primary key, or 'all'. No primary key also means no index on the entity id.")
	flags.StringSlice("disable-unique-constraints", nil, "Tables whose unique constraints are left out, or 'all'.")
	flags.Bool("disable-block-number-index", false, "Leave out the index on _block_number_. Every table carries that column and every reorg deletes from every table by it, so without the index each undo is a sequential scan per table — a foreign key indexes its referenced side only. It is created when the sink starts, concurrently and outside the constraint pass: --apply-constraints describes the schema and is yours to schedule, where this one the sink depends on to undo a reorg. Measured over 10GiB it costs 2.6s to build and 2% of the table. Only dead weight on a run that can never reorg, such as --final-blocks-only.")
	flags.String("proto-file-override", "", "Override protobuf file to use instead of extracting from substreams package")

	flags.Bool("no-constraints", false, "Deprecated, use --disable-foreign-keys --disable-primary-keys=all --disable-unique-constraints=all.")
	_ = flags.MarkDeprecated("no-constraints", "use --disable-foreign-keys --disable-primary-keys=all --disable-unique-constraints=all")
}

// addFromProtoRunFlags registers the from-proto flags that only apply while the sink is
// running. The ClickHouse specific ones are only registered for the clickhouse engine.
func addFromProtoRunFlags(flags *pflag.FlagSet, driver string) {
	flags.String("apply-constraints", string(protosql.ConstraintsAuto), "When the schema's constraints are created: 'auto' has the sink create them once the backfill reaches chain HEAD or the end of a bounded run, 'manual' leaves it to the 'sink postgres constraints apply' command, 'always' creates them before the load. Creating them is a stop-the-world operation — indexes to build, foreign keys to validate, tables locked throughout — so on a large database 'manual' is how that pass goes into a maintenance window instead. Loading with them already in place is the expensive option: measured through binary COPY, 27x slower than loading without, where building the same constraints afterwards costs 3.3x. The index on _block_number_ is not one of these: the sink creates it when it starts, see --disable-block-number-index.")

	flags.String("write-mode", string(protosql.WriteModeAuto), "How a sealed spool segment reaches the database: 'copy' loads each table file with binary COPY, 'batch-insert' builds one multi-row INSERT per table, 'row-insert' issues one prepared INSERT per row. 'auto' picks copy on PostgreSQL, batch-insert on ClickHouse, and row-insert for a schema whose foreign keys cannot be ordered. An explicit mode the driver or the schema cannot support is an error rather than a downgrade. Ignored once the stream reaches chain HEAD, where the sink always inserts directly.")

	flags.Int("decode-workers", 0, "How many blocks are unmarshalled and walked concurrently. Zero takes one per core less one for the goroutine draining the stream, capped at 8: measured at 4.13x on eight workers and only 4.24x on fifteen, the work being allocator-bound well before it runs out of cores. This is CPU work only and does not change what the database sees.")
	flags.Int("decode-batch-size", 0, "How many blocks are held in memory and decoded together. Zero takes four per decode worker. Larger batches keep the workers fed; smaller ones cost less memory, since every held block keeps its payload and its decoded rows. This sizes the CPU stage, not the database write — see --db-write-target-duration for that.")

	flags.Duration("db-write-target-duration", 3*time.Second, "How long one commit to the database should take. A commit is one spooled segment, whichever write mode applies it; each is measured and the next segment sized toward this. Raise it for fewer, larger commits; lower it to keep the sink from occupying a database that is shared with something else. Sizing by measured duration rather than by a block count is what keeps this stable across chains, where block payloads differ by orders of magnitude. Backfill only.")
	flags.String("db-write-max-size", "512MiB", "Ceiling for the segment size the sizer may choose, whatever the target duration would allow. Backfill only.")

	flags.Int("block-batch-size", 25, "Deprecated, use --decode-batch-size.")
	_ = flags.MarkDeprecated("block-batch-size", "use --decode-batch-size")

	flags.String("spool-dir", defaultSpoolDir, "Directory the pending segments are written to. Rows land here first and a background goroutine loads them, so the stream never waits on the database and blocks already downloaded survive a restart instead of being streamed, and paid for, twice. Backfill only.")
	flags.String("spool-max-size", "8GiB", "Disk budget for --spool-dir, and the only bound on how far ahead of the database the stream may run. The stream is held once pending segments reach it, which is what turns a slow database into backpressure rather than a full disk. Backfill only.")
	flags.Duration("spool-max-idle", 10*time.Second, "Write the open segment to the database once no new row has reached it for this long, short of its size target. A stream that stalls would otherwise sit on those rows indefinitely, leaving the cursor where it was and the blocks to be streamed, and paid for, again on restart. Zero disables it. Backfill only.")

	if driver == "clickhouse" {
		flags.String("sink-info-folder", "", "Folder where to store the clickhouse sink info")
		flags.String("cursor-file-path", "cursor.txt", "File name where to store the clickhouse cursor")
		flags.Int("query-retry-count", 3, "Number of retries for ClickHouse queries when an error occurs")
		flags.Duration("query-retry-sleep", time.Second, "Sleep duration between ClickHouse query retries (e.g. 1s, 500ms)")
	}
}

// addSinkRunFlags registers the full set of flags used by the sink process of an
// engine command.
//
// Both mode vocabularies land on the same command, and that is not fixable: the mode is
// read from the module's output type at run time, long after init() has registered
// everything. What the run command can do is say which half applies — see
// setModeGroupedUsage — and fail on a flag typed for the other one.
func addSinkRunFlags(flags *pflag.FlagSet, driver string) {
	sink.AddFlagsToSet(flags, sink.FlagExcludeDefault(sink.FlagUndoBufferSize))
	addBytesEncodingFlag(flags)
	if driver == "clickhouse" {
		addClusterFlag(flags)
	}
	addDatabaseChangesModeRunFlags(flags)
	addFromProtoSchemaFlags(flags)
	addFromProtoRunFlags(flags, driver)
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
			if err := rejectFromProtoFlags(cmd); err != nil {
				return err
			}

			return runDatabaseChangesSink(cmd, manifestPath, outputModule, dsnString)
		}

		if err := rejectDatabaseChangesFlags(cmd); err != nil {
			return err
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
	constraints, err := fromProtoConstraintPolicy(cmd)
	if err != nil {
		return err
	}

	writeMode, err := protosql.ParseWriteMode(sflags.MustGetString(cmd, "write-mode"))
	if err != nil {
		return err
	}

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
		// Without the schema annotations there are no relations to constrain. The index on
		// _block_number_ is not declared by them either, so it stays.
		constraints = protosql.DisableAllConstraints().WithBlockNumberIndex(constraints)

		zlog.Warn("the module's output carries no schema.proto annotations, so its tables get no primary keys, no unique constraints and no foreign keys. " +
			"Queries against them are sequential scans and duplicate ids are not rejected. " +
			"`substreams tools extract-proto --sql` writes the annotated proto to start from, and --proto-file-override points the sink at it")
	}
	warnAboutConstraints(constraints)

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

	// Nothing told the sinker when the stream reached the chain head, so nothing could
	// act on it — the local buffer kept holding blocks that a live sink wants in the
	// database as they arrive. The cursor-based checker turns live as soon as an
	// undo-able block shows up, which is exactly that moment.
	sink.WithLivenessChecker(sink.NewCursorBasedLivenessChecker())(baseSink)

	spool, err := fromProtoSpoolOptions(cmd)
	if err != nil {
		return err
	}

	options := db_proto.SinkerFactoryOptions{
		Spool:           spool,
		UseProtoOption:  useProtoOption,
		Constraints:     constraints,
		UseTransactions: true,
		WriteMode:       writeMode,
		DecodeWorkers:   sflags.MustGetInt(cmd, "decode-workers"),
		DecodeBatchSize: fromProtoDecodeBatchSize(cmd),
		Encoding:        encoding,
		Clickhouse:      fromProtoClickhouseOptions(cmd, driver),
	}.Defaults()

	logFromProtoSettings(cmd, driver, options, constraints)

	factory := db_proto.SinkerFactory(baseSink, outputModuleName, rootMessageDescriptor.UnwrapMessage(), options)

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

		manifestPath, outputModule := sinkManifestAndModule(args)

		sinkDBPreStart(cmd)

		sink.LoadSubstreamsAuthEnvFile(manifestPath)

		spkg, module, _, err := sink.ReadManifestAndModule(manifestPath, "", nil, outputModule, sink.IgnoreOutputModuleType, false, nil, zlog)
		if err != nil {
			return fmt.Errorf("reading manifest: %w", err)
		}

		if isDatabaseChangesType(module.Output.Type) {
			if err := rejectFlags(cmd, fromProtoSchemaFlagNames, "from-proto",
				"This one outputs DatabaseChanges, where the SQL schema is yours: it is created from the "+
					"'schema.sql' bundled in the manifest, and the sink neither derives it nor manages its constraints"); err != nil {
				return err
			}

			return runDatabaseChangesSetup(cmd, dsnString, spkg)
		}

		if err := rejectDatabaseChangesSetupFlags(cmd); err != nil {
			return err
		}

		return runFromProtoSetup(cmd, driver, dsnString, spkg, module.Name)
	}
}

// newSinkApplyConstraintsE creates the schema's constraints on a database the sink has
// already loaded, which is deliberately a separate command.
//
// It is a stop-the-world operation: every index is built and every foreign key validated,
// with the tables locked while it runs. On a large database that is a maintenance window,
// so it is the operator who picks the moment, not the sink.
// constraintsAction says which way `sink postgres constraints` runs.
type constraintsAction string

const (
	constraintsApply constraintsAction = "apply"
	constraintsDrop  constraintsAction = "drop"
)

func newSinkConstraintsE(driver string, action constraintsAction) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		dsnString, err := resolveSinkDSN(cmd)
		if err != nil {
			return err
		}
		if _, err := validateDSNEngine(dsnString, driver); err != nil {
			return err
		}

		manifestPath, outputModule := sinkManifestAndModule(args)

		sinkDBPreStart(cmd)
		sink.LoadSubstreamsAuthEnvFile(manifestPath)

		spkg, module, _, err := sink.ReadManifestAndModule(manifestPath, "", nil, outputModule, sink.IgnoreOutputModuleType, false, nil, zlog)
		if err != nil {
			return fmt.Errorf("reading manifest: %w", err)
		}

		if isDatabaseChangesType(module.Output.Type) {
			return errDatabaseChangesOwnsSchema("sink " + driver + " constraints")
		}

		constraints, err := fromProtoConstraintPolicy(cmd)
		if err != nil {
			return err
		}
		if constraints.SkipsEverything() {
			return fmt.Errorf("every constraint is disabled by the flags, so there is nothing to %s", action)
		}

		dsn, err := db.ParseDSN(dsnString)
		if err != nil {
			return fmt.Errorf("parsing dsn: %w", err)
		}

		protoFileOverride := sflags.MustGetString(cmd, "proto-file-override")
		rootMessageDescriptor, _, useProtoOption, err := resolveFromProtoRootMessage(spkg, module.Name, protoFileOverride)
		if err != nil {
			return err
		}
		if !useProtoOption {
			// Without annotations the sink has no declared keys or relations, but the
			// index it creates for its own reorg path is not declared either — so there is
			// still something to do, and refusing would leave every table unindexed.
			constraints = protosql.DisableAllConstraints().WithBlockNumberIndex(constraints)

			zlog.Warn("the module's output carries no schema.proto annotations, so there are no primary keys, unique constraints or foreign keys to " + string(action) + ". " +
				"The index on _block_number_ is not affected: the sink creates that one when it starts. " +
				"`substreams tools extract-proto --sql` writes the annotated proto to start from")
		}

		encoding, err := sinkBytesEncoding(cmd)
		if err != nil {
			return err
		}

		options := db_proto.SinkerFactoryOptions{
			UseProtoOption: useProtoOption,
			Constraints:    constraints,
			Encoding:       encoding,
			Clickhouse:     fromProtoClickhouseOptions(cmd, driver),
		}

		if action == constraintsApply {
			zlog.Info("creating the schema's constraints, this locks every table while indexes are built and foreign keys validated",
				zap.String("schema", dsn.Schema()),
				zap.String("constraints", constraints.Describe()))
		} else {
			zlog.Info("dropping the schema's constraints",
				zap.String("schema", dsn.Schema()),
				zap.String("constraints", constraints.Describe()))
		}

		startAt := time.Now()
		database, err := db_proto.SetupDatabaseSchema(cmd.Context(), dsnString, dsn.Schema(), module.Name, rootMessageDescriptor.UnwrapMessage(), options, zlog, tracer)
		if err != nil {
			return fmt.Errorf("setting up database schema: %w", err)
		}
		defer database.Close(cmd.Context())

		if action == constraintsApply {
			if err := applyDatabaseConstraints(database); err != nil {
				return err
			}
			zlog.Info("constraints created", zap.Duration("duration", time.Since(startAt)))

			return nil
		}

		if err := dropDatabaseConstraints(database); err != nil {
			return err
		}
		zlog.Info("constraints dropped", zap.Duration("duration", time.Since(startAt)))

		return nil
	}
}

// rejectFromProtoFlags fails on a from-proto flag typed for a Substreams that outputs
// DatabaseChanges.
//
// The mode is read from the module, not chosen by a flag, so both vocabularies are
// registered on the same command and half of them are inert for any given run. They used
// to be inert silently; a flag typed here means the operator expects the sink to be doing
// something it will not do, and a warning in a log that scrolls past is not how they
// should find that out.
func rejectFromProtoFlags(cmd *cobra.Command) error {
	names := append(append([]string{}, fromProtoSchemaFlagNames...), fromProtoRunFlagNames...)

	return rejectFlags(cmd, names, "from-proto",
		"This one outputs DatabaseChanges, where rows are written from the module's own database changes "+
			"and the schema is the 'schema.sql' bundled in the manifest")
}

// rejectDatabaseChangesFlags is the same guard in the other direction.
func rejectDatabaseChangesFlags(cmd *cobra.Command) error {
	return rejectFlags(cmd, databaseChangesFlagNames, "DatabaseChanges",
		"This one outputs an arbitrary protobuf message, which the sink maps to relational tables of its own")
}

func rejectFlags(cmd *cobra.Command, names []string, appliesTo string, because string) error {
	for _, name := range names {
		if flagChanged(cmd, name) {
			return fmt.Errorf("--%s only applies to %s Substreams. %s", name, appliesTo, because)
		}
	}

	return nil
}

// errDatabaseChangesOwnsSchema is what every from-proto-only entry point says when the
// module turns out to output DatabaseChanges.
func errDatabaseChangesOwnsSchema(what string) error {
	return fmt.Errorf("%s only applies to from-proto Substreams. This one outputs DatabaseChanges, where the SQL schema is yours: "+
		"it is created from the 'schema.sql' bundled in the manifest, and the sink neither derives it nor manages its constraints", what)
}

// applyDatabaseConstraints and dropDatabaseConstraints do not wrap the pass in a
// transaction: it owns its own, committing every --constraints-per-transaction statements
// so a killed run keeps what it finished.
func applyDatabaseConstraints(database protosql.Database) error {
	if err := database.ApplyConstraints(); err != nil {
		return fmt.Errorf("applying constraints: %w", err)
	}

	return nil
}

func dropDatabaseConstraints(database protosql.Database) error {
	if err := database.DropConstraints(); err != nil {
		return fmt.Errorf("dropping constraints: %w", err)
	}

	return nil
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
	constraints, err := fromProtoConstraintPolicy(cmd)
	if err != nil {
		return err
	}

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
		// Without the schema annotations there are no relations to constrain. The index on
		// _block_number_ is not declared by them either, so it stays.
		constraints = protosql.DisableAllConstraints().WithBlockNumberIndex(constraints)

		zlog.Warn("the module's output carries no schema.proto annotations, so its tables get no primary keys, no unique constraints and no foreign keys. " +
			"Queries against them are sequential scans and duplicate ids are not rejected. " +
			"`substreams tools extract-proto --sql` writes the annotated proto to start from, and --proto-file-override points the sink at it")
	}
	warnAboutConstraints(constraints)

	options := db_proto.SinkerFactoryOptions{
		UseProtoOption: useProtoOption,
		Constraints:    constraints,
		Encoding:       encoding,
		Clickhouse:     fromProtoClickhouseOptions(cmd, driver),
	}

	database, err := db_proto.SetupDatabaseSchema(cmd.Context(), dsnString, dsn.Schema(), outputModuleName, rootMessageDescriptor.UnwrapMessage(), options, zlog, tracer)
	if err != nil {
		return fmt.Errorf("setting up database schema: %w", err)
	}
	defer database.Close(cmd.Context())

	zlog.Info("database schema setup completed", zap.String("schema", dsn.Schema()), zap.String("constraints", constraints.Describe()))
	return nil
}

// warnIgnoredDatabaseChangesSetupFlags logs a warning for each DatabaseChanges-only setup
// flag that the user explicitly set, since those flags have no effect in from-proto mode.
// rejectDatabaseChangesSetupFlags fails on a DatabaseChanges-only setup flag typed for a
// from-proto setup. It used to warn; the two directions now agree, because a flag typed
// for the wrong mode means the operator expects something that will not happen.
func rejectDatabaseChangesSetupFlags(cmd *cobra.Command) error {
	names := append([]string{"postgraphile", "system-tables-only", "ignore-duplicate-table-errors"}, databaseChangesFlagNames...)

	return rejectFlags(cmd, names, "DatabaseChanges",
		"This one outputs an arbitrary protobuf message, whose schema the sink derives from the module's own descriptors")
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

// fromProtoConstraintPolicy resolves the constraint flags. Everything is created by
// default, once the backfill is done rather than before it.
func fromProtoConstraintPolicy(cmd *cobra.Command) (protosql.ConstraintPolicy, error) {
	timing, err := protosql.ParseConstraintTiming(stringFlag(cmd, "apply-constraints"))
	if err != nil {
		return protosql.ConstraintPolicy{}, err
	}

	policy := protosql.ConstraintPolicy{
		Timing:             timing,
		DisableForeignKeys: boolFlag(cmd, "disable-foreign-keys"),
		DisablePrimaryKeys: stringSliceFlag(cmd, "disable-primary-keys"),
		DisableUniques:     stringSliceFlag(cmd, "disable-unique-constraints"),

		DisableBlockNumberIndex: boolFlag(cmd, "disable-block-number-index"),
		PerTransaction:          intFlag(cmd, "constraints-per-transaction"),
	}

	// --no-constraints shipped in v1.21.0 and said "none of them at all", which is exactly
	// what the three switches say together. It stays honoured for a release.
	if flagChanged(cmd, "no-constraints") && boolFlag(cmd, "no-constraints") {
		disabled := protosql.DisableAllConstraints()
		policy.DisableForeignKeys = true
		policy.DisablePrimaryKeys = disabled.DisablePrimaryKeys
		policy.DisableUniques = disabled.DisableUniques
	}

	return policy, nil
}

// fromProtoDecodeBatchSize resolves how many blocks are decoded together, honouring the
// flag --block-batch-size shipped under before it named the database write it no longer
// sizes.
func fromProtoDecodeBatchSize(cmd *cobra.Command) int {
	if flagChanged(cmd, "decode-batch-size") {
		return sflags.MustGetInt(cmd, "decode-batch-size")
	}
	if flagChanged(cmd, "block-batch-size") {
		return sflags.MustGetInt(cmd, "block-batch-size")
	}

	return 0
}

// fromProtoSpoolOptions resolves the --spool-* and --db-write-* flags.
func fromProtoSpoolOptions(cmd *cobra.Command) (*spool.Options, error) {
	dir := sflags.MustGetString(cmd, "spool-dir")
	if dir == "" {
		return nil, fmt.Errorf("--spool-dir cannot be empty; the spool is what makes a restart resume rather than re-stream")
	}

	maxBytes, err := parseByteSize(cmd, "spool-max-size")
	if err != nil {
		return nil, err
	}

	segmentMaxBytes, err := parseByteSize(cmd, "db-write-max-size")
	if err != nil {
		return nil, err
	}

	maxIdle := sflags.MustGetDuration(cmd, "spool-max-idle")
	if maxIdle == 0 {
		// Zero disables it, which the Options zero value cannot say on its own.
		maxIdle = -1
	}

	return &spool.Options{
		Dir:                 dir,
		MaxBytes:            maxBytes,
		WriteTargetDuration: sflags.MustGetDuration(cmd, "db-write-target-duration"),
		SegmentMaxBytes:     segmentMaxBytes,
		MaxIdle:             maxIdle,
	}, nil
}

func parseByteSize(cmd *cobra.Command, name string) (int64, error) {
	raw := sflags.MustGetString(cmd, name)
	parsed, err := humanize.ParseBytes(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --%s %q: %w", name, raw, err)
	}

	return int64(parsed), nil
}

// flagChanged reports whether the operator actually typed the flag, as opposed to it
// carrying its default. Every mode and deprecation check keys off this: a default value
// must never be read as an instruction.
func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)

	return flag != nil && flag.Changed
}

// logFromProtoSettings states, in one line, every knob that applies to this run.
//
// The alternative is an operator reading a help screen that lists both mode vocabularies
// and guessing which half took effect. The write mode here is what was asked for; the
// database logs what it resolved to, and why, once it has seen the schema.
func logFromProtoSettings(cmd *cobra.Command, driver string, options db_proto.SinkerFactoryOptions, constraints protosql.ConstraintPolicy) {
	fields := []zap.Field{
		zap.String("driver", driver),
		zap.String("write_mode_requested", string(options.WriteMode)),
		zap.Int("decode_workers", options.DecodeWorkers),
		zap.Int("decode_batch_size", options.DecodeBatchSize),
		zap.String("constraints", constraints.Describe()),
	}

	if options.Spool != nil {
		fields = append(fields,
			zap.String("spool_dir", options.Spool.Dir),
			zap.String("spool_max_size", humanize.IBytes(uint64(options.Spool.MaxBytes))),
			zap.Duration("spool_max_idle", options.Spool.MaxIdle),
			zap.Duration("db_write_target_duration", options.Spool.WriteTargetDuration),
			zap.String("db_write_max_size", humanize.IBytes(uint64(options.Spool.SegmentMaxBytes))),
		)
	}

	zlog.Info("from-proto sink settings", fields...)
}

// warnAboutConstraints says out loud what applying constraints before the backfill costs,
// since the flag makes the load an order of magnitude slower and its effect on an already
// populated database is not instantaneous.
func warnAboutConstraints(constraints protosql.ConstraintPolicy) {
	if !constraints.ApplyUpfront() {
		return
	}

	zlog.Warn("constraints are being created before the backfill rather than after it. Loading with foreign keys in place measured 27x slower than loading without them, " +
		"where building the same constraints afterwards cost 3.3x, so a large initial sync is considerably slower this way. " +
		"Creating the constraints on an already populated database can also take a long time, and locks the tables while it runs")
}
