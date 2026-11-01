package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block's changes are undone last to first: a child row inserted after its parent in one block
// goes first, or the parent's delete violates the foreign key.
func TestPostgresRevert_UndoesABlockInReverseOrder(t *testing.T) {
	dsnString := pgTestSchema(t)
	dsn, err := ParseDSN(dsnString)
	require.NoError(t, err)
	schema := dsn.Schema()

	raw, err := sql.Open("postgres", dsn.ConnString())
	require.NoError(t, err)
	defer raw.Close()
	_, err = raw.Exec(fmt.Sprintf(`
		CREATE TABLE %[1]s.parent (id text PRIMARY KEY);
		CREATE TABLE %[1]s.child (id text PRIMARY KEY, parent_id text NOT NULL REFERENCES %[1]s.parent (id));`, schema))
	require.NoError(t, err)

	ctx := context.Background()
	l := pgTestLoader(t, dsnString, true)
	block := uint64(11)
	for i := range 20 {
		parent, child := fmt.Sprintf("p%d", i), fmt.Sprintf("c%d", i)
		require.NoError(t, l.Insert("parent", map[string]string{"id": parent}, map[string]FieldData{}, &block))
		require.NoError(t, l.Insert("child", map[string]string{"id": child}, map[string]FieldData{"parent_id": {Value: parent}}, &block))
	}
	require.NoError(t, l.InsertCursor(ctx, testModuleHash, pgCursor(10, 10)))
	_, err = l.Flush(ctx, testModuleHash, pgCursor(11, 10), 10)
	require.NoError(t, err)

	require.NoError(t, l.Revert(ctx, testModuleHash, pgCursor(10, 10), 10))

	var parents, children int
	require.NoError(t, raw.QueryRow(fmt.Sprintf(`SELECT (SELECT count(*) FROM %[1]s.parent), (SELECT count(*) FROM %[1]s.child)`, schema)).Scan(&parents, &children))
	assert.Zero(t, parents)
	assert.Zero(t, children)
}
