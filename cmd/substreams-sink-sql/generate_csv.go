package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	. "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	sink "github.com/streamingfast/substreams/sink"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	sinker2 "github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
)

// lastCursorFilename is the name of the file where the last cursor is stored, no extension as it's added by the store
const lastCursorFilename = "last_cursor"

var generateCsvCmd = Command(generateCsvE,
	"generate-csv <dsn> <manifest> [start]:<stop>",
	"Generates CSVs for each table so it can be bulk inserted with `inject-csv` (for postgresql only)",
	Description(`
		This command command is the first of a multi-step process to bulk insert data into a PostgreSQL database.
		It creates a folder for each table and generates CSVs for block ranges. This files can be used with
		the 'inject-csv' command to bulk insert data into the database.

		It needs that the database already exists and that the schema is already created.

		The process is as follows:

		- Generate CSVs for each table with this command
		- Inject the CSVs into the database with the 'inject-csv' command (contains 'cursors' table, double check you injected it correctly!)
		- Start streaming with the 'run' command
	`),
	ExactArgs(3),
	Flags(func(flags *pflag.FlagSet) {
		sink.AddFlagsToSet(flags)
		AddCommonSinkerFlags(flags)
		AddCommonDatabaseChangesFlags(flags)

		flags.Uint64("bundle-size", 10000, "Size of output bundle, in blocks")
		flags.String("working-dir", "./workdir", "Path to local folder used as working directory")
		flags.String("output-dir", "./csv-output", "Path to local folder used as destination for CSV")
		flags.Uint64("buffer-max-size", 4*1024*1024, FlagDescription(`
			Amount of memory bytes to allocate to the buffered writer. If your data set is small enough that every is hold in memory, we are going to avoid
			the local I/O operation(s) and upload accumulated content in memory directly to final storage location.

			Ideally, you should set this as to about 80%% of RAM the process has access to. This will maximize amount of element in memory,
			and reduce 'syscall' and I/O operations to write to the temporary file as we are buffering a lot of data.

			This setting has probably the greatest impact on writing throughput.

			Default value for the buffer is 4 MiB.
		`))
	}),
	OnCommandErrorLogAndExit(zlog),
)

func generateCsvE(cmd *cobra.Command, args []string) error {
	app := NewApplication(cmd.Context())

	sinker2.RegisterMetrics()

	dsnString := args[0]
	manifestPath := args[1]
	blockRange := args[2]

	// Parse block range and set flags to bridge with substreams/sink library
	br, err := readBlockRangeArgument(blockRange)
	if err != nil {
		return fmt.Errorf("invalid block range %q: %w", blockRange, err)
	}

	// Bridge start-block flag with substreams/sink library
	if br.StartBlock() > 0 {
		if err := cmd.Flags().Set("start-block", fmt.Sprintf("%d", br.StartBlock())); err != nil {
			return fmt.Errorf("setting start-block flag: %w", err)
		}
	}
	// Bridge stop-block flag with substreams/sink library
	if br.EndBlock() != nil {
		if err := cmd.Flags().Set("stop-block", fmt.Sprintf("%d", *br.EndBlock())); err != nil {
			return fmt.Errorf("setting stop-block flag: %w", err)
		}
	}

	outputDir := sflags.MustGetString(cmd, "output-dir")
	bundleSize := sflags.MustGetUint64(cmd, "bundle-size")
	bufferMaxSize := sflags.MustGetUint64(cmd, "buffer-max-size")
	workingDir := sflags.MustGetString(cmd, "working-dir")
	cursorTableName := sflags.MustGetString(cmd, "cursors-table")
	historyTableName := sflags.MustGetString(cmd, "history-table")

	// Bridge final-blocks-only flag with substreams/sink library (required for CSV generation)
	if err := cmd.Flags().Set("final-blocks-only", "true"); err != nil {
		return fmt.Errorf("setting final-blocks-only flag: %w", err)
	}

	sk, err := sink.NewFromViper(
		cmd,
		supportedOutputTypes,
		manifestPath,
		sink.InferOutputModuleFromPackage,
		fmt.Sprintf("substreams-sink-sql/%s", version),
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new base sinker: %w", err)
	}

	dsn, err := db2.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	handleReorgs := false
	dbLoader, err := db2.NewLoader(
		dsn,
		cursorTableName,
		historyTableName,
		sflags.MustGetString(cmd, "clickhouse-cluster"),
		0, 0, 0,
		resolveOnModuleHashMismatchFlag(cmd),
		&handleReorgs,
		zlog, tracer,
	)

	if err != nil {
		return fmt.Errorf("creating loader: %w", err)
	}

	if err := dbLoader.LoadTables(dsn.Schema(), cursorTableName, historyTableName); err != nil {
		var e *db2.SystemTableError
		if errors.As(err, &e) {
			fmt.Printf("Error validating the system table: %s\n", e)
			fmt.Println("Did you run setup ?")
			return e
		}

		return fmt.Errorf("load tables: %w", err)
	}

	generateCSVSinker, err := sinker2.NewGenerateCSVSinker(
		sk,
		outputDir,
		workingDir,
		cursorTableName,
		bundleSize,
		bufferMaxSize,
		dbLoader,
		lastCursorFilename,
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("unable to setup generate csv sinker: %w", err)
	}

	app.Supervise(generateCSVSinker.Shutter)

	go func() {
		generateCSVSinker.Run(app.Context())
	}()

	return app.WaitForTermination(zlog, 0*time.Second, 30*time.Second)
}
