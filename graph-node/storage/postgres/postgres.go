package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/abourget/llerrgroup"
	"github.com/jmoiron/sqlx"
	graphnode "github.com/streamingfast/substreams/graph-node"
	"github.com/streamingfast/substreams/graph-node/metrics"
	"github.com/streamingfast/substreams/graph-node/subgraph"
	"go.uber.org/zap"
)

const saveGracePeriodBeforeAbort = 10 * time.Second
const saveConcurrentUpdates = 20

type store struct {
	db                    *sqlx.DB
	schemaName            string
	loadStmts             map[string]*sqlx.Stmt
	saveStmts             map[string]string
	preparedSaveStmts     map[string]*sqlx.NamedStmt
	updateBlockRangeStmts map[string]*sqlx.Stmt
	persistentCache       *entityCache
	withNotifications     bool
	metrics               *metrics.BlockMetrics
	firstBlockWritten     bool
	neverReadFromDB       map[string]bool
	subgraph              *subgraph.Definition
	withTransaction       bool
	logger                *zap.Logger
	subgraphDeploymentID  string
	notifyTag             int64
}

type storeEventChangeData struct {
	EntityType string `json:"entity_type"`
	SubgraphID string `json:"subgraph_id"`
}
type storeEventChange struct {
	Data storeEventChangeData `json:"Data"`
}
type storeEventChanges struct {
	Changes []storeEventChange `json:"changes"`
	Tag     int64              `json:"tag"`
}

var SystemTables = []string{"poi2$", "cursor"}

func dbFromDSN(dsnString string) (*sqlx.DB, error) {
	connectionInfo, err := ParseDSN(dsnString)
	if err != nil {
		return nil, fmt.Errorf("parsing dsn: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sqlx.ConnectContext(ctx, "postgres", connectionInfo.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	db.SetMaxOpenConns(500)

	return db, nil
}

func New(
	logger *zap.Logger,
	metrics *metrics.BlockMetrics,
	dsn string,
	subgraphSchema string,
	subgraphDeploymentID string,
	subgraph *subgraph.Definition,
	entitiesNeverReadFromDB map[string]bool,
	withTransaction bool,
) (*store, error) {
	db, err := dbFromDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("creating database: %w", err)
	}

	return &store{
		db:                    db,
		metrics:               metrics,
		schemaName:            subgraphSchema,
		subgraphDeploymentID:  subgraphDeploymentID,
		subgraph:              subgraph,
		loadStmts:             map[string]*sqlx.Stmt{},
		saveStmts:             map[string]string{},
		preparedSaveStmts:     map[string]*sqlx.NamedStmt{},
		updateBlockRangeStmts: map[string]*sqlx.Stmt{},
		withNotifications:     os.Getenv("SQL_NOTIFY") == "true",

		persistentCache: newEntityCache(),

		neverReadFromDB: entitiesNeverReadFromDB,
		logger:          logger,
		withTransaction: withTransaction,
	}, nil
}

func (s *store) StartLogger(ctx context.Context) {
	go func() {
		cacheFrequency := 10 * time.Second

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(cacheFrequency):
			}

			s.logger.Info(fmt.Sprintf("cache stats each %s", cacheFrequency),
				zap.Int("cache_hits", cacheHits),
				zap.Int("cache_miss", cacheMiss),
				zap.Int("cache_delete", cacheRemove),
				zap.Int("cache_unsupported", cacheOther),
			)
		}
	}()
}

func (s *store) RegisterEntities() error {
	for _, entity := range s.subgraph.Entities.Entities() {
		if err := s.registerStatements(entity); err != nil {
			return err
		}
	}

	return nil
}

func (s *store) LoadAllDistinct(ctx context.Context, model graphnode.Entity, blockNum uint64) (out []graphnode.Entity, err error) {
	tableName := graphnode.GetTableName(model)
	query := fmt.Sprintf("SELECT * FROM %s.%s WHERE block_range @> %d", s.schemaName, tableName, blockNum)

	// FIXME: Could we somehow be able to correctly have a []graphnode.Entity directly? Here we create a new empty pointer
	// to a slice of the specific "models" type (for example `models.Pair`). This is used by `SelectContext` to know how
	// to properly unmarshal the data. Later we transform that into an `[]graphnode.Entity`.
	modelsPtr := reflect.New(reflect.SliceOf(reflect.TypeOf(model)))
	if err := s.db.SelectContext(ctx, modelsPtr.Interface(), query); err != nil {
		return nil, err
	}

	models := modelsPtr.Elem()
	modelCount := models.Len()
	if modelCount == 0 {
		return nil, nil
	}

	out = make([]graphnode.Entity, modelCount)
	for i := 0; i < modelCount; i++ {
		out[i] = models.Index(i).Interface().(graphnode.Entity)
	}
	return
}

func (s *store) registerStatements(ent graphnode.Entity) error {
	tableName := graphnode.GetTableName(ent)
	s.logger.Info("registering entity", zap.String("table_name", tableName))

	// load
	query := fmt.Sprintf("SELECT * FROM %q.%q WHERE id = $1 and block_range @> $2", s.schemaName, tableName)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	defer cancel()
	queryPreparedStmt, err := s.db.PreparexContext(ctx, query)
	if err != nil {
		return fmt.Errorf("preparing select statement for entity %q: %w", tableName, err)
	}
	s.loadStmts[tableName] = queryPreparedStmt
	s.logger.Info("query registered")

	save := buildSaveQuery(s.schemaName, tableName, ent)
	s.saveStmts[tableName] = save

	savePrepared, err := s.db.PrepareNamedContext(ctx, save)
	if err != nil {
		return fmt.Errorf("preparing save statement for entity %q: %w", tableName, err)
	}
	s.preparedSaveStmts[tableName] = savePrepared
	s.logger.Info("save statement registered")

	update := fmt.Sprintf(`UPDATE %q.%q
	                       SET
                             "_updated_block_number" = $1,
							 block_range = Q.block_range
	                       FROM (
	                         SELECT (value->>0)::bigint AS vid, (value->>1)::int4range AS block_range
	                         FROM json_array_elements($2)
	                       ) Q
	                       WHERE %q.vid = Q.vid`, s.schemaName, tableName, tableName)
	updateStmt, err := s.db.PreparexContext(ctx, update)
	if err != nil {
		return fmt.Errorf("preparing update statement for entity %q: %w", tableName, err)
	}
	s.updateBlockRangeStmts[tableName] = updateStmt
	s.logger.Info("update statement registered")

	return nil

}

func (s *store) BatchSave(ctx context.Context, blockNum uint64, blockHash string, blockTime time.Time, updates map[string]map[string]graphnode.Entity, cursor string) (err error) {
	// It seems we are called even if the context is done, we should find the upstream where abortion should be called instead of here, a bit hackish for now
	if ctx.Err() != nil {
		return nil
	}

	// The save operation must complete fully, so we use an independent context. We then start a go routine
	// which roles is to listen to the parent context (`ctx`) and when it's done, give <Grace Period> time
	// for the save to complete. If it does not complete
	saveCtx, cancelSave := context.WithCancel(context.Background())
	defer cancelSave()

	go func() {
		select {
		case <-ctx.Done():
			s.logger.Info("save parent context is done, waiting grace period before aborting on-going operation", zap.Duration("grace_period", saveGracePeriodBeforeAbort))
			time.Sleep(saveGracePeriodBeforeAbort)
			cancelSave()
		case <-saveCtx.Done():
			// The save actually completed already, we can stop right now.
			s.logger.Debug("save completed operation, no need to cancel anything")
		}
	}()

	trxs := []*sqlx.Tx{}
	eg := llerrgroup.New(saveConcurrentUpdates)
	for tableName, entities := range updates {
		if eg.Stop() {
			continue // short-circuit the loop if we got an error
		}

		var tx *sqlx.Tx
		if s.withTransaction {
			tx, err = s.db.BeginTxx(saveCtx, nil)
			if err != nil {
				return fmt.Errorf("begin transaction: %w", err)
			}
			trxs = append(trxs, tx)
		}

		theTableName := tableName
		theEntities := entities
		eg.Go(func() error {
			err = s.batchSave(saveCtx, tx, blockNum, theTableName, theEntities)
			if err != nil {
				return fmt.Errorf("batch saving: %w", err)
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		s.rollback(saveCtx, trxs)
		return fmt.Errorf("batch save: %w", err)
	}

	s.logger.Debug("all table flush", zap.Int("tx_count", len(trxs)), zap.Int("registered_entities", s.subgraph.Entities.Len()))

	depTx, err := s.db.BeginTxx(saveCtx, nil)
	if err != nil {
		s.rollback(saveCtx, trxs)
		return fmt.Errorf("begin transaction for deployment head tracking: %w", err)
	}
	trxs = append(trxs, depTx)

	if err = s.updateDeploymentHead(ctx, depTx, blockNum, blockHash); err != nil {
		s.rollback(saveCtx, trxs)
		return fmt.Errorf("unable to save subgraph deployemnt head: %w", err)
	}

	curTx, err := s.db.BeginTxx(saveCtx, nil)
	if err != nil {
		s.rollback(saveCtx, trxs)
		return fmt.Errorf("begin transaction for cursor: %w", err)
	}

	trxs = append(trxs, curTx)

	if err = s.saveCursor(saveCtx, curTx, cursor); err != nil {
		s.rollback(saveCtx, trxs)
		return fmt.Errorf("unable to save cursor: %w", err)
	}

	s.mustCommit(trxs)

	if s.withNotifications {
		if err := s.notify(saveCtx, updates); err != nil {
			s.logger.Warn("could not notify", zap.Error(err))
		}
	}

	if blockNum%100 == 0 {
		s.logger.Info("purging cache", zap.Duration("grace_period", saveGracePeriodBeforeAbort))
		s.persistentCache.purgeCache(blockNum, blockTime)
	}

	return nil
}

func (s *store) notify(ctx context.Context, updates map[string]map[string]graphnode.Entity) error {
	touchedTables := make(map[string]bool)
	for tableName := range updates {
		touchedTables[tableName] = true
	}
	changes := []storeEventChange{}
	for tableName := range touchedTables {
		t, _ := s.subgraph.Entities.GetType(tableName)
		changes = append(changes, storeEventChange{
			Data: storeEventChangeData{
				EntityType: t.Name(),
				SubgraphID: s.subgraphDeploymentID,
			},
		})
	}
	s.notifyTag++ // starts at 1
	notifChanges := &storeEventChanges{
		Tag:     s.notifyTag,
		Changes: changes,
	}
	notifChangesBytes, _ := json.Marshal(notifChanges)

	notifyQuery := fmt.Sprintf(`NOTIFY store_events, '%s'`, string(notifChangesBytes))
	fmt.Println(notifyQuery)
	if _, err := s.db.ExecContext(ctx, notifyQuery); err != nil {
		return fmt.Errorf("error calling notify '%s', %w", notifyQuery, err)
	}
	return nil
}

func (s *store) rollback(ctx context.Context, trxs []*sqlx.Tx) {
	// If the context was canceled, transaction are already rolled back because it's the behavior
	// of the sql driver to rollback active transaction on a canceled context, hence, we must skip it
	// here.
	// if ctx.Err() != nil {
	// 	return
	// }

	// If we roll back, it's because an error occurs, so we can afford an Info level heres
	s.logger.Info("about to rollback transaction", zap.Int("count", len(trxs)))
	for _, trx := range trxs {
		if err := trx.Rollback(); err != nil {
			s.logger.Panic("transaction rollback failed", zap.Error(err))
		}
	}
}

func (s *store) mustCommit(trxs []*sqlx.Tx) {
	s.logger.Debug("about to commit transaction", zap.Int("count", len(trxs)))
	for _, trx := range trxs {
		if err := trx.Commit(); err != nil {
			s.logger.Panic("transaction commit failed", zap.Error(err))
		}
	}
}

func (s *store) batchSave(ctx context.Context, dbTx *sqlx.Tx, blockNum uint64, tableName string, entities map[string]graphnode.Entity) (err error) {
	// This for loop is ONLY for updating the block ranges.
	var jsonSnips []string
	for id, ent := range entities {
		if ent == nil { // deleted
			ent = &graphnode.Base{}
			err := s.EntityForID(ctx, tableName, id, ent)
			if err != nil {
				s.logger.Warn("cannot delete entity", zap.Error(err), zap.String("id", id))
				continue
			}

			if !ent.Exists() {
				continue
			}
		}
		blockRange := ent.GetBlockRange()
		if blockRange == nil {
			continue
		}

		vid := ent.GetVID()
		r := &graphnode.BlockRange{
			StartBlock: blockRange.StartBlock,
			EndBlock:   blockNum,
		}

		rValue, err := r.Value()
		if err != nil {
			return fmt.Errorf("error with getting range value: %w", err)
		}
		snip := fmt.Sprintf(`[%d,"%s"]`, vid, rValue)
		// example: '[2168648,"[1407391,1407392)"]'
		jsonSnips = append(jsonSnips, snip)
	}

	if len(jsonSnips) > 0 {
		jsonArray := "[" + strings.Join(jsonSnips, ",") + "]"
		// example: '[[2168648,"[1407391,1407392)"],[...]]'
		startUpdate := time.Now()
		err := s.UpdateBlockRange(ctx, dbTx, tableName, blockNum, jsonArray)
		s.metrics.Exec.StoreUpdatesOnly += time.Since(startUpdate)
		if err != nil {
			return fmt.Errorf("error updating block range: %w", err)
		}
	}

	var processableEntities []graphnode.Entity
	for _, ent := range entities {
		if ent == nil {
			continue
		}
		ent.SetVID(0)
		ent.SetBlockRange(&graphnode.BlockRange{
			StartBlock: blockNum,
		})
		ent.SetUpdatedBlockNum(blockNum)

		processableEntities = append(processableEntities, ent)
		if e, ok := ent.(graphnode.Sanitizable); ok {
			e.Sanitize()
		}
	}

	if len(processableEntities) == 0 {
		return nil
	}
	s.metrics.Exec.StoreSave += int64(len(processableEntities))
	s.metrics.Exec.StoreCall += 1
	s.firstBlockWritten = true

	startInsert := time.Now()
	defer func() {
		s.metrics.Exec.StoreInsertsOnly += time.Since(startInsert)
	}()

	if len(processableEntities) == 1 {
		ent := processableEntities[0]
		// use prepared stmt
		row := struct {
			VID uint64 `db:"vid"`
		}{}

		stmt, found := s.preparedSaveStmts[tableName]
		if !found {
			return fmt.Errorf("could not find save smts for tableName %q", tableName)
		}

		if dbTx != nil {
			err = dbTx.NamedStmt(stmt).GetContext(ctx, &row, ent)
		} else {
			err = stmt.GetContext(ctx, &row, ent)
		}
		if err != nil {
			return fmt.Errorf("inserting into %q, id=%s, range=%v: %w", tableName, ent.GetID(), ent.GetBlockRange(), err)
		}

		ent.SetVID(row.VID)
		s.persistentCache.SetEntity(tableName, ent)
		return nil
	}

	stmt, found := s.saveStmts[tableName]
	if !found {
		return fmt.Errorf("could not find save smts for tableName %q", tableName)
	}

	var vids []uint64

	//stmt = INSERT INTO sgd1.uniswap_factory ("id", "block_range", "pair_count", "total_volume_usd", "total_volume_eth", "untracked_volume_usd", "total_liquidity_usd", "total_liquidity_eth", "tx_count") VALUES (:id, :block_range, :pair_count, :total_volume_usd, :total_volume_eth, :untracked_volume_usd, :total_liquidity_usd, :total_liquidity_eth, :tx_count) RETURNING vid

	var rows *sqlx.Rows
	if dbTx != nil {
		rows, err = dbTx.NamedQuery(stmt, processableEntities)
	} else {
		rows, err = s.db.NamedQuery(stmt, processableEntities)
	}

	if err != nil {
		s.logger.Warn("error inserting into db",
			zap.String("table_name", tableName),
			zap.Int("entities_count", len(processableEntities)),
			zap.Uint64("blk_number", blockNum),
			zap.String("sql_stmt", stmt),
			zap.Error(err),
		)
		return fmt.Errorf("inserting into %q, %d entities: at block_num: %d: %w", tableName, len(processableEntities), blockNum, err)
	}

	for rows.Next() {
		var vid uint64
		if err := rows.Scan(&vid); err != nil {
			return fmt.Errorf("cannot read rows: %w", err)
		}
		vids = append(vids, vid)
	}

	for i, ent := range processableEntities {
		ent.SetVID(vids[i])
		s.persistentCache.SetEntity(tableName, ent)
	}

	return nil
}

func (s *store) CleanDataAtBlock(ctx context.Context, blockNum uint64) error {
	return s.cleanDBAboveBlockNum(blockNum)
}

func (s *store) cleanDBAboveBlockNum(blockNum uint64) error {
	badBlockNum := blockNum + 1
	for table := range s.subgraph.Entities.Data() {
		deleteStmt := fmt.Sprintf("DELETE FROM %s.%s where _updated_block_number = %d and upper(block_range) is null", s.schemaName, table, badBlockNum)
		updateStmt := fmt.Sprintf("UPDATE %s.%s set block_range = int4range(lower(block_range), NULL) where block_range @> %d and upper(block_range) = %d and _updated_block_number = %d", s.schemaName, table, blockNum, badBlockNum, badBlockNum)

		startDel := time.Now()
		res, err := s.db.Exec(deleteStmt)
		if err != nil {
			return fmt.Errorf("delete rows of table: %s where range is [%d, ]: %w", table, badBlockNum, err)
		}

		affectedRowCount, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("deleted effect row: %w", err)
		}
		s.logger.Info("deleted rows with bad block_range", zap.String("table", table), zap.Uint64("bad_block_num", badBlockNum), zap.Int64("effected_rows", affectedRowCount), zap.Duration("duration", time.Since(startDel)))

		startUpd := time.Now()
		res, err = s.db.Exec(updateStmt)
		if err != nil {
			return fmt.Errorf("update rows of table: %s where upper(block_range) is [%d, ]: %w", table, badBlockNum, err)
		}

		affectedRowCount, err = res.RowsAffected()
		if err != nil {
			return fmt.Errorf("updated effect row: %w", err)
		}
		s.logger.Info("updated rows with bad upper block_range", zap.String("table", table), zap.Uint64("bad_block_num", badBlockNum), zap.Int64("effected_rows", affectedRowCount), zap.Duration("duration", time.Since(startUpd)))
	}
	return nil
}

func buildSaveQuery(schemaName, tableName string, ent graphnode.Entity) string {
	fields := []string{`"id"`, `"block_range"`, `_updated_block_number`}
	colonFields := []string{":id", ":block_range", `:_updated_block_number`}

	for _, el := range graphnode.DBFields(reflect.TypeOf(ent)) {
		if el.Base {
			continue
		}
		fields = append(fields, `"`+el.ColumnName+`"`)
		colonFields = append(colonFields, ":"+el.ColumnName)
	}
	// BaseEntity is excluded above ^^
	return "INSERT INTO " + schemaName + "." + tableName + " (" + strings.Join(fields, ", ") + ") VALUES (" + strings.Join(colonFields, ", ") + ") RETURNING vid"
}

func (s *store) Load(ctx context.Context, id string, ent graphnode.Entity, blockNum uint64) error {
	startOne := time.Now()
	defer func() {
		s.metrics.Exec.FullLoadTime += time.Since(startOne)
	}()
	tableName := graphnode.GetTableName(ent)

	found := s.persistentCache.GetEntity(tableName, id, ent)
	if found {
		return nil
	}

	if cacheable, ok := ent.(graphnode.Cacheable); ok {
		if cacheable.SkipDBLookup() {
			return nil
		}
	}

	if s.firstBlockWritten && s.neverReadFromDB[tableName] {
		return nil
	}

	// FIXME syntax error with @> using a prepared named statement
	// LIMIT 1 is an optimization because GIST index is cannot be UNIQUE, but we assume it is.
	// The index can have an additional uniqueness constraint, but it won't help performance, on the contrary.
	query := fmt.Sprintf("SELECT * FROM "+s.schemaName+"."+tableName+" WHERE id = '%s' and block_range @> %d LIMIT 1", id, blockNum)

	//stmt := s.loadStmts[tableName]
	start := time.Now()
	defer func() {
		s.metrics.Exec.SelectQueries += time.Since(start)
		s.metrics.Exec.SelectQueriesCounts[tableName]++
		duration := time.Since(start)
		s.metrics.Exec.SelectQueriesDurations[tableName] += duration
		if duration > time.Millisecond*100 {
			s.logger.Info("slow query from DB", zap.Bool("found", ent.Exists()), zap.Duration("duration", duration), zap.String("query", query), zap.Uint64("block_num", blockNum), zap.String("id", id))
		}

		s.persistentCache.SetEntity(tableName, ent) // existing OR non-existing will be cached
	}()

	// We are creating a temporary Entity as to not override the caller's
	// entity until we are guaranteed that we loaded a row. The sqlx reflex lib
	// optimistically changes the callers attributes to some default values even when nullable
	// i.e. a *bool which is set to null would be changed to (false)
	entType, _ := s.subgraph.Entities.GetType(tableName)
	tempEnt := reflect.New(entType).Interface()
	err := s.db.GetContext(ctx, tempEnt, query)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("load %q from %q: %w", id, tableName, err)
	}
	ve := reflect.ValueOf(ent).Elem()
	ve.Set(reflect.ValueOf(tempEnt).Elem())
	ent.SetExists(true)

	return nil
}

func (s *store) EntityForID(ctx context.Context, tableName string, id string, entity graphnode.Entity) (err error) {
	start := time.Now()
	loadQuery := "SELECT id, block_range, vid FROM " + s.schemaName + "." + tableName + " WHERE id = $1 ORDER BY block_range DESC limit 1"
	err = s.db.GetContext(ctx, entity, loadQuery, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("get with context %q: %w", id, err)
	}
	s.metrics.Exec.SelectQueriesCounts[tableName]++
	s.metrics.Exec.SelectQueriesDurations[tableName] += time.Since(start)
	entity.SetExists(true)
	return
}

func (s *store) UpdateBlockRange(ctx context.Context, dbTx *sqlx.Tx, tableName string, blockNum uint64, jsonArray string) (err error) {
	stmt := s.updateBlockRangeStmts[tableName]

	var res sql.Result
	if dbTx != nil {
		res, err = dbTx.Stmtx(stmt).ExecContext(ctx, blockNum, jsonArray)
	} else {
		res, err = stmt.ExecContext(ctx, blockNum, jsonArray)
	}
	if err != nil {
		return fmt.Errorf("update %q table block range with json %q: %w", tableName, jsonArray, err)
	}

	rowsCnt, err := res.RowsAffected()
	s.logger.Debug("updated block ranges", zap.String("table_name", tableName), zap.Int64("rows_affected", rowsCnt))
	return nil
}

func (s *store) updateDeploymentHead(ctx context.Context, tx *sqlx.Tx, blockNumer uint64, blockHash string) error {
	return nil //todo: fix this please
	updateDeploymentQuery := "update subgraphs.subgraph_deployment set latest_ethereum_block_number=$1, latest_ethereum_block_hash=$2 where deployment = $3"
	result, err := tx.ExecContext(ctx, updateDeploymentQuery, blockNumer, blockHash, s.subgraphDeploymentID)
	if err != nil {
		return fmt.Errorf("failed updating subgraph %q at block %d: %w", s.subgraphDeploymentID, blockNumer, err)
	}
	rowAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting affected row count: %w", err)
	}
	if rowAffected == 0 {
		return fmt.Errorf("no row affected by deployment head update query for depployment %q: %w", s.subgraphDeploymentID, err)
	}
	return nil
}

// FIXME: the `subgraphID` was the DEPLOYMENT ID with Qmhellowold.. not the `exchange` string.. FIX IT ABOURGET!
func (s *store) saveCursor(ctx context.Context, tx *sqlx.Tx, cursor string) error {
	query := fmt.Sprintf("INSERT INTO %s.cursor (id, cursor) VALUES (1, $1) ON CONFLICT (id) DO UPDATE SET cursor = $2", s.schemaName)
	_, err := tx.ExecContext(ctx, query, cursor, cursor)
	if err != nil {
		return fmt.Errorf("failed saving cursor's query %q: %w", query, err)
	}

	return nil
}

func (s *store) LoadCursor(ctx context.Context) (string, error) {
	row := struct {
		ID     uint64 `db:"id"`
		Cursor string `db:"cursor"`
	}{}

	// create the table if not exists:
	_, _ = s.db.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.cursor (id integer PRIMARY KEY, cursor text);`, s.schemaName))
	if err := s.db.GetContext(ctx, &row, fmt.Sprintf("SELECT * FROM %s.cursor WHERE id = 1", s.schemaName)); err != nil {
		if err == sql.ErrNoRows {
			return "", nil // nothing exists yet
		}

		return "", fmt.Errorf("coulnd't get cursor: %w", err)
	}

	return row.Cursor, nil
}

func (s *store) CleanUpFork(ctx context.Context, longestChainStartBlock uint64) error {
	//longestChainStartBlock:  first new block on the longest chain (after the common ancestor)

	for table := range s.subgraph.Entities.Data() {
		// SCENARIOS
		//a,1,1,"[1,10)",10,k
		//a,2,2,"[10,20)",20,k
		//a,3,3,"[20,30)",30,u
		//a,4,4,"[30,40)",40,d
		//a,5,5,"[40,)",40,d
		//b,1,6,"[1,)",1,k
		//c,1,7,"[1,10)",10,k
		//c,2,8,"[10,20)",20,k
		//c,3,9,"[20,25)",25,u
		//c,4,10,"[25,30)",30,d
		//c,5,11,"[30,)",30,d

		//Test table
		//create table if not exists sgd4.toto
		//(
		//	id text not null,
		//	value numeric not null,
		//	vid bigserial not null
		//		constraint toto_pkey
		//			primary key,
		//	block_range int4range not null,
		//	_updated_block_number numeric not null,
		//	result text not null
		//);
		//
		//alter table sgd4.toto owner to graph;
		//
		//create index if not exists toto_id
		//	on sgd4.toto (id);
		//
		//create index if not exists toto_value
		//	on sgd4.toto (value);
		//
		//create index if not exists toto_block_range_closed
		//	on sgd4.toto (COALESCE(upper(block_range), 2147483647))
		//	where (COALESCE(upper(block_range), 2147483647) < 2147483647);

		deleteStmt := fmt.Sprintf("delete from %s.%s  where (_updated_block_number > %d and not %d <@ block_range) or (_updated_block_number >= %d and block_range @> %d and lower(block_range) = %d) returning id", s.schemaName, table, longestChainStartBlock, longestChainStartBlock, longestChainStartBlock, longestChainStartBlock, longestChainStartBlock)
		updateStmt := fmt.Sprintf("update %s.%s set block_range = int4range(lower(block_range), null), _updated_block_number = lower(block_range) where _updated_block_number >=  %d and (block_range @> %d or upper(block_range) = %d) returning id", s.schemaName, table, longestChainStartBlock, longestChainStartBlock, longestChainStartBlock)

		s.logger.Info("cleaning fork", zap.String("delete_statement", deleteStmt), zap.String("update_statement", updateStmt))

		startDel := time.Now()
		rows, err := s.db.QueryContext(ctx, deleteStmt)
		if err != nil {
			return fmt.Errorf("delete rows of table: %s where _updated_block_number > %d: %w", table, longestChainStartBlock, err)
		}

		var count int
		for rows.Next() {
			count++
			var id string
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("cannot read rows: %w", err)
			}
			s.persistentCache.Invalidate(table, id)
		}

		s.logger.Info("deleted rows because of a fork", zap.String("table", table), zap.Uint64("new_head", longestChainStartBlock), zap.Int("affected_rows", count), zap.Duration("duration", time.Since(startDel)))

		startUpd := time.Now()
		rows, err = s.db.QueryContext(ctx, updateStmt)
		if err != nil {
			return fmt.Errorf("update rows of table: %s where _updated_block_number = %d: %w", table, longestChainStartBlock, err)
		}

		count = 0
		for rows.Next() {
			count++
			var id string
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("cannot read rows: %w", err)
			}
			s.persistentCache.Invalidate(table, id)
		}
		s.logger.Info("updated rows because of a fork", zap.String("table", table), zap.Uint64("new_head", longestChainStartBlock), zap.Int("affected_rows", count), zap.Duration("duration", time.Since(startUpd)))

	}
	return nil
}

func (s *store) TruncateAll(ctx context.Context, confirmFunc func(tables []string) (bool, error)) (bool, error) {
	sqlStmts := []string{}
	labels := []string{}
	for _, tbl := range SystemTables {
		label := fmt.Sprintf("%s.%s", s.schemaName, tbl)
		sqlStmt := s.truncateStmt(tbl)
		sqlStmts = append(sqlStmts, sqlStmt)
		labels = append(labels, label)
	}
	for table := range s.subgraph.Entities.Data() {
		label := fmt.Sprintf("%s.%s", s.schemaName, table)
		sqlStmt := s.truncateStmt(table)
		sqlStmts = append(sqlStmts, sqlStmt)
		labels = append(labels, label)
	}
	ok, err := confirmFunc(labels)
	if err != nil {
		return false, fmt.Errorf("confirmation failed: %w", err)
	}
	if !ok {
		return false, nil
	}

	for _, stmt := range sqlStmts {
		_, err := s.db.ExecContext(ctx, stmt)
		if err != nil {
			return false, fmt.Errorf("unable to truncate table %s: %w", stmt, err)
		}
	}
	return true, nil
}

func (s *store) truncateStmt(tableName string) string {
	return fmt.Sprintf("TRUNCATE %s.%s;", s.schemaName, tableName)
}

func (s *store) Close() error { return nil }
