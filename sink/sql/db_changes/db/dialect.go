package db

import (
	"context"
	"database/sql"
	"fmt"

	sink "github.com/streamingfast/substreams/sink"
)

type UnknownDriverError struct {
	Driver string
}

// Error returns a formatted string description.
func (e UnknownDriverError) Error() string {
	return fmt.Sprintf("unknown database driver: %s", e.Driver)
}

type Dialect interface {
	GetCreateCursorQuery(schema string, withPostgraphile bool) string
	GetCreateHistoryQuery(schema string, withPostgraphile bool) string
	ExecuteSetupScript(ctx context.Context, l *Loader, schemaSql string) error
	DriverSupportRowsAffected() bool
	GetUpdateCursorQuery(table, moduleHash string, cursor *sink.Cursor, block_num uint64, block_id string) string
	GetAllCursorsQuery(table string) string
	ParseDatetimeNormalization(value string) string
	Flush(tx Tx, ctx context.Context, l *Loader, outputModuleHash string, lastFinalBlock uint64) (int, error)
	Revert(tx Tx, ctx context.Context, l *Loader, lastValidFinalBlock uint64) error
	Capabilities() DialectCapabilities
	CreateUser(tx Tx, ctx context.Context, l *Loader, username string, password string, database string, readOnly bool) error
	GetTableColumns(db *sql.DB, schemaName, tableName string) ([]*sql.ColumnType, error)
	GetPrimaryKey(db *sql.DB, schemaName, tableName string) ([]string, error)
	GetTablesInSchema(db *sql.DB, schemaName string) ([][2]string, error)
}

// DialectCapabilities lists the operations a dialect supports, which the loader checks before
// accepting a change or turning on reorg handling.
type DialectCapabilities struct {
	// Mutations is whether upsert, update and delete operations are supported. Without it, only
	// inserts are.
	Mutations bool
	// Reorgs is whether forked blocks can be reverted, through the history table.
	Reorgs bool
	// DuplicateInserts is whether a primary key can be inserted more than once in a batch, the
	// last insert winning, instead of failing.
	DuplicateInserts bool
}
