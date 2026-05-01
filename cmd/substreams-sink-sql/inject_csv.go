package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	. "github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging/zapx"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"go.uber.org/zap"
)

var injectCSVCmd = Command(injectCSVE,
	"inject-csv <psql_dsn> <input_path> <table> <start>:<stop>",
	"Injects generated CSV rows for <table> into the database pointed by <psql_dsn> argument. (postgresql-only)",
	Description(`
		Can be run in parallel for multiple rows up to the same <stop>, the <start> and <stop> block must be provided explicitely.

		Watch out, the <start> must be aligned with the range size of the CSV files or the module initial block.
	`),
	ExactArgs(4),
	Flags(func(flags *pflag.FlagSet) {
		AddCommonDatabaseChangesFlags(flags)
	}),
)

func injectCSVE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	psqlDSN := args[0]
	inputPath := args[1]
	tableName := args[2]
	cursorTableName := sflags.MustGetString(cmd, "cursors-table")

	blockRange, err := readBlockRangeArgument(args[3])
	if err != nil {
		return fmt.Errorf("invalid block range %q: %w", args[3], err)
	}

	sqlDSN, err := db2.ParseDSN(psqlDSN)
	if err != nil {
		return fmt.Errorf("invalid sql DSN %q: %w", psqlDSN, err)
	}

	if sqlDSN.Driver() != "postgres" {
		return fmt.Errorf("inject-csv only supports postgresql DSNs, got %q", sqlDSN.Driver())
	}

	zlog.Info("connecting to input store")
	inputStore, err := dstore.NewStore(inputPath, "csv", "", false)
	if err != nil {
		return fmt.Errorf("unable to create input store: %w", err)
	}

	pool, err := pgxpool.Connect(ctx, sqlDSN.ConnString())
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}

	t0 := time.Now()

	zlog.Debug("table filler", zap.String("pg_schema", sqlDSN.Schema()), zap.String("table_name", tableName), zap.Stringer("range", blockRange))
	filler := NewTableFiller(pool, sqlDSN.Schema(), tableName, blockRange.StartBlock(), *blockRange.EndBlock(), inputStore, cursorTableName)

	if err := filler.Run(ctx); err != nil {
		return fmt.Errorf("table filler %q: %w", tableName, err)
	}

	zlog.Info("table done", zapx.HumanDuration("total", time.Since(t0)))
	return nil
}

type TableFiller struct {
	pqSchema string
	tblName  string

	in              dstore.Store
	startBlockNum   uint64
	stopBlockNum    uint64
	pool            *pgxpool.Pool
	cursorTableName string
}

func NewTableFiller(pool *pgxpool.Pool, pqSchema, tblName string, startBlockNum, stopBlockNum uint64, inStore dstore.Store, cursorTableName string) *TableFiller {
	return &TableFiller{
		tblName:         tblName,
		pqSchema:        pqSchema,
		pool:            pool,
		startBlockNum:   startBlockNum,
		stopBlockNum:    stopBlockNum,
		in:              inStore,
		cursorTableName: cursorTableName,
	}
}

func extractFieldsFromFirstLine(ctx context.Context, filename string, store dstore.Store) ([]string, error) {
	fl, err := store.OpenObject(ctx, filename)
	if err != nil {
		return nil, fmt.Errorf("opening csv: %w", err)
	}
	defer fl.Close()

	return extractFieldsFromReader(fl)
}

func extractColumnsFromBytes(data []byte) ([]string, error) {
	return extractFieldsFromReader(bytes.NewBuffer(data))
}

func extractFieldsFromReader(reader io.Reader) ([]string, error) {
	r := csv.NewReader(reader)
	out, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv first line: %w", err)
	}

	return out, nil
}

func (t *TableFiller) Run(ctx context.Context) error {
	zlog.Info("table filler", zap.String("table", t.tblName))

	if t.tblName == t.cursorTableName {
		return t.injectCursorsTable(ctx)
	}

	loadFiles, err := findFilesToLoad(ctx, t.in, t.tblName, t.stopBlockNum, t.startBlockNum)
	if err != nil {
		return fmt.Errorf("listing files: %w", err)
	}

	if len(loadFiles) == 0 {
		return fmt.Errorf("no file to process")
	}

	dbFields, err := extractFieldsFromFirstLine(ctx, loadFiles[0], t.in)
	if err != nil {
		return fmt.Errorf("extracting fields from first csv line: %w", err)
	}

	zlog.Info("files to load",
		zap.String("table", t.tblName),
		zap.Int("file_count", len(loadFiles)),
	)

	for _, filename := range loadFiles {
		zlog.Info("opening file", zap.String("file", filename))

		if err := t.injectCSVFromFile(ctx, filename, dbFields); err != nil {
			return fmt.Errorf("failed to inject file %q: %w", filename, err)
		}
	}

	return nil
}

func (t *TableFiller) injectCursorsTable(ctx context.Context) error {
	path := t.cursorTableName + "/" + lastCursorFilename
	reader, err := t.in.OpenObject(ctx, path)
	if err != nil {
		if errors.Is(err, dstore.ErrNotFound) {
			return fmt.Errorf("trying to inject %q table but the last cursor filename %q was not found in %s", t.cursorTableName, path, t.in.BaseURL())
		}

		return fmt.Errorf("open object %q: %w", lastCursorFilename, err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read object %q: %w", lastCursorFilename, err)
	}

	dbColumns, err := extractColumnsFromBytes(content)
	if err != nil {
		return fmt.Errorf("extracting fields from first csv line: %w", err)
	}

	zlog.Info("injecting cursors table")
	if err := t.injectCSVFromReader(ctx, bytes.NewBuffer(content), "<generated>", dbColumns); err != nil {
		return fmt.Errorf("failed to inject %q table content: %w", t.cursorTableName, err)
	}

	return nil
}

func (t *TableFiller) injectCSVFromFile(ctx context.Context, filename string, dbColumns []string) error {
	fl, err := t.in.OpenObject(ctx, filename)
	if err != nil {
		return fmt.Errorf("opening csv: %w", err)
	}
	defer fl.Close()

	return t.injectCSVFromReader(ctx, fl, filename, dbColumns)
}

func (t *TableFiller) injectCSVFromReader(ctx context.Context, fl io.Reader, source string, dbColumns []string) error {
	query := fmt.Sprintf(`COPY %s.%s ("%s") FROM STDIN WITH (FORMAT CSV, HEADER)`,
		db2.EscapeIdentifier(t.pqSchema),
		db2.EscapeIdentifier(t.tblName),
		strings.Join(dbColumns, `","`))
	zlog.Info("loading file into sql from reader", zap.String("source", source), zap.String("table_name", t.tblName), zap.Strings("db_columns", dbColumns))

	t0 := time.Now()

	conn, err := t.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("pool acquire: %w", err)
	}
	defer conn.Release()

	tag, err := conn.Conn().PgConn().CopyFrom(ctx, fl, query)
	if err != nil {
		return fmt.Errorf("failed COPY FROM for %q: %w", t.tblName, err)
	}
	count := tag.RowsAffected()
	elapsed := time.Since(t0)
	zlog.Info("loaded file into sql",
		zap.String("source", source),
		zap.String("table_name", t.tblName),
		zap.Int64("rows_affected", count),
		zapx.HumanDuration("elapsed", elapsed),
	)

	return nil
}

func findFilesToLoad(ctx context.Context, inputStore dstore.Store, tableName string, stopBlockNum, desiredStartBlockNum uint64) (out []string, err error) {
	err = inputStore.Walk(ctx, tableName+"/", func(filename string) (err error) {
		startBlockNum, endBlockNum, err := getBlockRange(filename)
		if err != nil {
			return fmt.Errorf("fail reading block range in %q: %w", filename, err)
		}

		if stopBlockNum != 0 && startBlockNum >= stopBlockNum {
			return dstore.StopIteration
		}

		if endBlockNum < desiredStartBlockNum {
			return nil
		}

		out = append(out, filename)
		return nil
	})
	return
}

var blockRangeRegex = regexp.MustCompile(`(\d{10})-(\d{10})`)

func getBlockRange(filename string) (uint64, uint64, error) {
	match := blockRangeRegex.FindStringSubmatch(filename)
	if match == nil {
		return 0, 0, fmt.Errorf("no block range in filename: %s", filename)
	}

	startBlock, _ := strconv.ParseUint(match[1], 10, 64)
	stopBlock, _ := strconv.ParseUint(match[2], 10, 64)
	return startBlock, stopBlock, nil
}
