package sinker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/logging/zapx"
	"github.com/streamingfast/shutter"
	sink "github.com/streamingfast/substreams/sink"
	pbdatabase "github.com/streamingfast/substreams-sink-database-changes/pb/sf/substreams/sink/database/v1"
	bundler2 "github.com/streamingfast/substreams/sink/sql/db_changes/bundler"
	writer2 "github.com/streamingfast/substreams/sink/sql/db_changes/bundler/writer"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	state2 "github.com/streamingfast/substreams/sink/sql/db_changes/state"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type GenerateCSVSinker struct {
	*shutter.Shutter
	*sink.Sinker

	bundlersByTable    map[string]*bundler2.Bundler
	cursorsTableStore  dstore.Store
	lastCursorFilename string
	stopBlock          uint64

	// cursor
	stateStore state2.Store
	bundleSize uint64

	loader *db2.Loader
	logger *zap.Logger
	tracer logging.Tracer

	stats           *Stats
	cursorTableName string
}

func NewGenerateCSVSinker(
	sink *sink.Sinker,
	destFolder string,
	workingDir string,
	cursorTableName string,
	bundleSize uint64,
	bufferSize uint64,
	loader *db2.Loader,
	lastCursorFilename string,
	logger *zap.Logger,
	tracer logging.Tracer,
) (*GenerateCSVSinker, error) {
	stopBlock := sink.StopBlock()
	if stopBlock == 0 {
		return nil, fmt.Errorf("sink must have a stop block defined")
	}

	stateStorePath := filepath.Join(workingDir, "state.yaml")
	stateFileDirectory := filepath.Dir(stateStorePath)
	if err := os.MkdirAll(stateFileDirectory, os.ModePerm); err != nil {
		return nil, fmt.Errorf("create state file directories: %w", err)
	}

	stateDStore, err := dstore.NewStore(destFolder, "", "", false)
	if err != nil {
		return nil, err
	}
	stateStore, err := state2.NewFileStateStore(stateStorePath, stateDStore, logger)
	if err != nil {
		return nil, fmt.Errorf("new file state store: %w", err)
	}

	csvOutputStore, err := dstore.NewStore(destFolder, "csv", "", false)
	if err != nil {
		return nil, err
	}

	cursorsStore, err := csvOutputStore.SubStore(cursorTableName)
	if err != nil {
		return nil, fmt.Errorf("cursors sub store: %w", err)
	}

	s := &GenerateCSVSinker{
		Shutter: shutter.New(),
		Sinker:  sink,

		bundlersByTable:    make(map[string]*bundler2.Bundler),
		cursorsTableStore:  cursorsStore,
		lastCursorFilename: lastCursorFilename,
		stopBlock:          stopBlock,

		loader: loader,
		logger: logger,
		tracer: tracer,

		stateStore: stateStore,
		bundleSize: bundleSize,

		cursorTableName: cursorTableName,

		stats: NewStats(logger),
	}

	tables := s.loader.GetAvailableTablesInSchema()
	for _, table := range tables {
		columns := s.loader.GetColumnsForTable(table)
		fb, err := getBundler(table, uint64(s.Sinker.StartBlock()), s.stopBlock, bundleSize, bufferSize, csvOutputStore, workingDir, logger, columns)
		if err != nil {
			return nil, err
		}
		s.bundlersByTable[table] = fb
	}

	return s, nil
}

func (s *GenerateCSVSinker) Run(ctx context.Context) {
	s.stateStore.Start(ctx)
	s.stateStore.OnTerminating(s.Shutdown)
	cursor, err := s.stateStore.ReadCursor(ctx)
	if err != nil && !errors.Is(err, db2.ErrCursorNotFound) {
		s.Shutdown(fmt.Errorf("unable to retrieve cursor: %w", err))
		return
	}

	s.Sinker.OnTerminating(s.Shutdown)
	s.OnTerminating(func(_ error) {
		s.Sinker.Shutdown(nil)
		s.stats.LogNow()
		s.logger.Info("csv sinker terminating", zap.Stringer("last_block_written", s.stats.lastBlock))
		s.stats.Close()
		s.stateStore.Shutdown(nil)
		s.closeAllFileBundlers()
	})

	s.stats.OnTerminated(func(err error) { s.Shutdown(err) })

	logEach := 15 * time.Second
	if s.logger.Core().Enabled(zap.DebugLevel) {
		logEach = 5 * time.Second
	}

	s.stats.Start(logEach, cursor)

	s.logger.Info("starting sql generate CSV sink",
		zapx.HumanDuration("stats_refresh_each", logEach),
		zap.Stringer("restarting_at", cursor.Block()),
		zap.String("loader", s.loader.GetIdentifier()),
	)

	for _, fb := range s.bundlersByTable {
		fb.Launch(ctx)
	}
	s.Sinker.Run(ctx, cursor, s)
}

func (s *GenerateCSVSinker) HandleBlockScopedData(ctx context.Context, data *pbsubstreamsrpc.BlockScopedData, isLive *bool, cursor *sink.Cursor) error {
	output := data.Output

	if output.Name != s.OutputModuleName() {
		return fmt.Errorf("received data from wrong output module, expected to received from %q but got module's output for %q", s.OutputModuleName(), output.Name)
	}

	dbChanges := &pbdatabase.DatabaseChanges{}
	mapOutput := output.GetMapOutput()
	if !mapOutput.MessageIs(dbChanges) && mapOutput.TypeUrl != "type.googleapis.com/sf.substreams.database.v1.DatabaseChanges" {
		return fmt.Errorf("mismatched message type: trying to unmarshal unknown type %q", mapOutput.MessageName())
	}

	if err := proto.Unmarshal(mapOutput.Value, dbChanges); err != nil {
		return fmt.Errorf("unmarshal database changes: %w", err)
	}

	if err := s.dumpDatabaseChangesIntoCSV(dbChanges); err != nil {
		return fmt.Errorf("apply database changes: %w", err)
	}

	s.stateStore.SetCursor(cursor)
	if cursor.Block().Num()%s.bundleSize == 0 {
		state, err := s.stateStore.GetState()
		if err != nil {
			s.Shutdown(fmt.Errorf("unable to get state: %w", err))
			return fmt.Errorf("unable to get state: %w", err)
		}

		if err := state.Save(); err != nil {
			s.Shutdown(fmt.Errorf("unable to save state: %w", err))
			return fmt.Errorf("unable to save state: %w", err)
		}
		s.stateStore.UploadCursor(state)
	}

	s.rollAllBundlers(ctx, data.Clock.Number, cursor)

	return nil
}

func (s *GenerateCSVSinker) dumpDatabaseChangesIntoCSV(dbChanges *pbdatabase.DatabaseChanges) error {
	for _, change := range dbChanges.TableChanges {
		if !s.loader.HasTable(change.Table) {
			return fmt.Errorf(
				"your Substreams sent us a change for a table named %s we don't know about on %s (available tables: %s)",
				change.Table,
				s.loader.GetIdentifier(),
				strings.Join(s.loader.GetAvailableTablesInSchema(), ", "),
			)
		}

		var fields map[string]string
		switch u := change.PrimaryKey.(type) {
		case *pbdatabase.TableChange_Pk:
			var err error
			fields, err = s.loader.GetPrimaryKey(change.Table, u.Pk)
			if err != nil {
				return err
			}

			delete(fields, "")

		case *pbdatabase.TableChange_CompositePk:
			fields = u.CompositePk.Keys
		default:
			return fmt.Errorf("unknown primary key type: %T", change.PrimaryKey)
		}

		table := change.Table
		tableBundler, ok := s.bundlersByTable[table]
		if !ok {
			return fmt.Errorf("cannot get bundler writer for table %s", table)
		}

		switch change.Operation {
		case pbdatabase.TableChange_OPERATION_CREATE:
			for _, field := range change.Fields {
				fields[field.Name] = field.Value
			}

			data, _ := bundler2.CSVEncode(fields)
			if !tableBundler.HeaderWritten {
				tableBundler.Writer().Write(tableBundler.Header)
				tableBundler.HeaderWritten = true
			}
			tableBundler.Writer().Write(data)
		case pbdatabase.TableChange_OPERATION_UPSERT:
			return fmt.Errorf("This substreams cannot be written to CSV because it performs 'UPSERT' operations on the %q store. Generate-CSV only supports inserts (CREATE)", change.Table)
		case pbdatabase.TableChange_OPERATION_UPDATE:
			return fmt.Errorf("This substreams cannot be written to CSV because it performs 'UPDATE' operations on the %q store. Generate-CSV only supports inserts (CREATE)", change.Table)
		case pbdatabase.TableChange_OPERATION_DELETE:
			return fmt.Errorf("This substreams cannot be written to CSV because it performs 'DELETE' operations on the %q store. Generate-CSV only supports inserts (CREATE)", change.Table)
		}
	}

	return nil
}

func (s *GenerateCSVSinker) rollAllBundlers(ctx context.Context, blockNum uint64, cursor *sink.Cursor) {
	block := cursor.Block()
	reachedEndBlock := &atomic.Bool{}

	var wg sync.WaitGroup
	for table, entityBundler := range s.bundlersByTable {
		wg.Add(1)

		go func(table string, eb *bundler2.Bundler) {
			rolled, err := eb.Roll(ctx, blockNum)
			if err != nil {
				if errors.Is(err, bundler2.ErrStopBlockReached) {
					reachedEndBlock.Store(true)
				} else {
					s.Shutdown(err)
				}
			}

			if rolled {
				s.logger.Debug("table data bundler rolled out some files", zap.String("table", table), zap.Stringer("block", block))
			}

			wg.Done()
		}(table, entityBundler)
	}

	wg.Wait()

	if reachedEndBlock.Load() {
		s.Shutdown(nil)
	}
}

func (s *GenerateCSVSinker) closeAllFileBundlers() {
	var wg sync.WaitGroup
	for _, fb := range s.bundlersByTable {
		wg.Add(1)
		f := fb
		go func() {
			f.Shutdown(nil)
			<-f.Terminated()
			wg.Done()
		}()
	}
	wg.Wait()
}

func (s *GenerateCSVSinker) HandleBlockUndoSignal(ctx context.Context, data *pbsubstreamsrpc.BlockUndoSignal, cursor *sink.Cursor) error {
	return fmt.Errorf("received undo signal but there is no handling of undo, this is because you used `--undo-buffer-size=0` which is invalid right now")
}

func getBundler(table string, startBlock, stopBlock, bundleSize, bufferSize uint64, baseOutputStore dstore.Store, workingDir string, logger *zap.Logger, columns []string) (*bundler2.Bundler, error) {
	boundaryWriter := writer2.NewBufferedIO(
		bufferSize,
		filepath.Join(workingDir, table),
		writer2.FileTypeCSV,
		logger.With(zap.String("table_name", table)),
	)
	subStore, err := baseOutputStore.SubStore(table)
	if err != nil {
		return nil, err
	}

	sort.Strings(columns)

	header := []byte(strings.Join(columns, ",") + "\n")
	fb, err := bundler2.New(bundleSize, stopBlock, boundaryWriter, subStore, logger, header)
	if err != nil {
		return nil, err
	}
	fb.Start(startBlock)
	return fb, nil
}

func (s *GenerateCSVSinker) HandleBlockRangeCompletion(ctx context.Context, cursor *sink.Cursor) error {
	s.closeAllFileBundlers()

	s.logger.Info("stream completed, writing cursors table", zap.Stringer("block", cursor.Block()))
	if err := s.writeCursorsTable(ctx, cursor); err != nil {
		return fmt.Errorf("write cursors table: %w", err)
	}

	return nil
}

func (s *GenerateCSVSinker) writeCursorsTable(ctx context.Context, lastCursor *sink.Cursor) error {
	buffer := bytes.NewBuffer(make([]byte, 0, 1024))

	columns := s.loader.GetColumnsForTable(s.cursorTableName)
	sort.Strings(columns)
	buffer.WriteString(strings.Join(columns, ","))
	buffer.WriteString("\n")

	block := lastCursor.Block()

	buffer.WriteString(fmt.Sprintf("%s,%d,%s,%s\n", block.ID(), block.Num(), lastCursor, s.OutputModuleHash()))

	if err := s.cursorsTableStore.WriteObject(ctx, s.lastCursorFilename, buffer); err != nil {
		return fmt.Errorf("write object: %w", err)
	}

	return nil
}
