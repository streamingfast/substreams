package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
)

const lastCursorFilename = "last_cursor"

var sinkPostgresGenerateCSVCmd = &cobra.Command{
	Use:   "generate-csv [<manifest> [<module>]]",
	Short: "Generates CSVs for each table so it can be bulk inserted with `inject-csv` (for postgresql only)",
	Long: cli.Dedent(`
		This command is the first of a multi-step process to bulk insert data into a PostgreSQL database.
		It creates a folder for each table and generates CSVs for block ranges. This files can be used with
		the 'inject-csv' command to bulk insert data into the database.

		It needs that the database already exists and that the schema is already created.

		The stop block is required and provided through the -t/--stop-block flag.

		The process is as follows:

		- Generate CSVs for each table with this command
		- Inject the CSVs into the database with the 'inject-csv' command (contains 'cursors' table, double check you injected it correctly!)
		- Start streaming with the 'run' command
	`),
	Args: cobra.RangeArgs(0, 2),
	RunE: sinkPostgresGenerateCSVE,
}

func init() {
	flags := sinkPostgresGenerateCSVCmd.Flags()
	sink.AddFlagsToSet(flags, sink.FlagExcludeDefault(sink.FlagFinalBlocksOnly))
	addOnModuleHashMismatchFlag(flags)
	addCursorTableFlags(flags)

	flags.Uint64("bundle-size", 10000, "Size of output bundle, in blocks")
	flags.String("working-dir", "./workdir", "Path to local folder used as working directory")
	flags.String("output-dir", "./csv-output", "Path to local folder used as destination for CSV")
	flags.Uint64("buffer-max-size", 4*1024*1024, cli.FlagDescription(`
		Amount of memory bytes to allocate to the buffered writer. If your data set is small enough that every is hold in memory, we are going to avoid
		the local I/O operation(s) and upload accumulated content in memory directly to final storage location.

		Ideally, you should set this as to about 80%% of RAM the process has access to. This will maximize amount of element in memory,
		and reduce 'syscall' and I/O operations to write to the temporary file as we are buffering a lot of data.

		This setting has probably the greatest impact on writing throughput.

		Default value for the buffer is 4 MiB.
	`))

	sinkPostgresCmd.AddCommand(sinkPostgresGenerateCSVCmd)
}

func sinkPostgresGenerateCSVE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	dsnString, err := resolveSinkDSN(cmd)
	if err != nil {
		return err
	}
	if _, err := validateDSNEngine(dsnString, sinkPostgresDriver); err != nil {
		return err
	}

	manifestPath, outputModule, err := ruiOrGuiManifestModulePositionalParams(args)
	if err != nil {
		return err
	}
	if outputModule == "" {
		outputModule = sink.InferOutputModuleFromPackage
	}

	if stop := sflags.MustGetString(cmd, sink.FlagStopBlock); stop == "" || stop == "0" {
		return fmt.Errorf("generate-csv requires a stop block, provide it with -t/--stop-block")
	}

	sinkDBPreStart(cmd)

	app := cli.NewApplication(cmd.Context())

	sinker.RegisterMetrics()

	outputDir := sflags.MustGetString(cmd, "output-dir")
	bundleSize := sflags.MustGetUint64(cmd, "bundle-size")
	bufferMaxSize := sflags.MustGetUint64(cmd, "buffer-max-size")
	workingDir := sflags.MustGetString(cmd, "working-dir")
	cursorTableName := sflags.MustGetString(cmd, "cursors-table")
	historyTableName := sflags.MustGetString(cmd, "history-table")

	sink.LoadSubstreamsAuthEnvFile(manifestPath)

	sinkConfig, err := sink.ConfigFromViper(
		cmd,
		supportedOutputTypes,
		manifestPath,
		outputModule,
		"sink_database_changes",
		zlog,
		tracer,
	)
	if err != nil {
		return fmt.Errorf("new base sinker config: %w", err)
	}
	sinkConfig.FinalBlocksOnly = true

	baseSink, err := sink.NewFromConfig(sinkConfig)
	if err != nil {
		return fmt.Errorf("new base sinker: %w", err)
	}

	dsn, err := db.ParseDSN(dsnString)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}

	handleReorgs := false
	dbLoader, err := db.NewLoader(
		dsn,
		cursorTableName,
		historyTableName,
		"",
		0, 0, 0,
		sflags.MustGetString(cmd, onModuleHashMismatchFlag),
		&handleReorgs,
		zlog, tracer,
	)
	if err != nil {
		return fmt.Errorf("creating loader: %w", err)
	}

	if err := dbLoader.LoadTables(dsn.Schema(), cursorTableName, historyTableName); err != nil {
		var e *db.SystemTableError
		if errors.As(err, &e) {
			fmt.Printf("Error validating the system table: %s\n", e)
			fmt.Println("Did you run setup ?")
			return e
		}

		return fmt.Errorf("load tables: %w", err)
	}

	generateCSVSinker, err := sinker.NewGenerateCSVSinker(
		baseSink,
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
