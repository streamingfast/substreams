package postgres

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlbytes "github.com/streamingfast/substreams/sink/sql/bytes"
	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/sink/sql/db_proto/sql/postgres/pgcopy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresTextArray_Value(t *testing.T) {
	value, err := postgresTextArray(nil).Value()
	require.NoError(t, err)
	assert.Nil(t, value)

	value, err = postgresTextArray{}.Value()
	require.NoError(t, err)
	assert.Equal(t, "{}", value)

	value, err = postgresTextArray{`{"a":"b\"c\\d"}`, `{}`}.Value()
	require.NoError(t, err)
	assert.Equal(t, `{"{\"a\":\"b\\\"c\\\\d\"}","{}"}`, value)
}

// Repeated inline messages go to a jsonb[] column through binary COPY.
func TestPostgresTextArray_BinaryCopy(t *testing.T) {
	values := []postgresTextArray{{`{"a":"b\"c\\d"}`, `{"n":1}`}, {}, nil}

	var encoded bytes.Buffer
	writer, err := pgcopy.NewWriter(&encoded, []pgcopy.Column{{Name: "v", OID: pgtype.JSONBArrayOID}})
	require.NoError(t, err)
	for _, value := range values {
		require.NoError(t, writer.WriteRow([]any{value}))
	}
	require.NoError(t, writer.Close())

	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		t.Skip("PG_DSN not set: not checking the server reads it back")
	}
	dsn, err := db.ParseDSN(dsnString)
	require.NoError(t, err)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn.ConnString())
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()

	_, err = conn.Exec(ctx, `CREATE TEMP TABLE inline_messages (v jsonb[])`)
	require.NoError(t, err)
	_, err = conn.PgConn().CopyFrom(ctx, &encoded, `COPY inline_messages (v) FROM STDIN WITH (FORMAT binary)`)
	require.NoError(t, err)

	var quote, empty string
	var nulls int
	require.NoError(t, conn.QueryRow(ctx, `SELECT
		(SELECT v[1]->>'a' FROM inline_messages WHERE array_length(v, 1) = 2),
		(SELECT v::text FROM inline_messages WHERE v = '{}'),
		(SELECT count(*) FROM inline_messages WHERE v IS NULL)`).Scan(&quote, &empty, &nulls))
	assert.Equal(t, `b"c\d`, quote)
	assert.Equal(t, "{}", empty)
	assert.Equal(t, 1, nulls)
}

// Batch insert renders repeated inline messages into the statement, and stores what COPY does.
func TestPostgresTextArray_Rendered(t *testing.T) {
	values := []postgresTextArray{{`{"a":"b\"c\\d"}`, `{"n":1}`}, {}, nil}
	rendered := make([]string, len(values))
	for i, value := range values {
		require.NotPanics(t, func() { rendered[i] = ValueToString(value, sqlbytes.EncodingRaw) })
	}

	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		t.Skip("PG_DSN not set: not checking what the server stores")
	}
	dsn, err := db.ParseDSN(dsnString)
	require.NoError(t, err)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn.ConnString())
	require.NoError(t, err)
	defer func() { _ = conn.Close(ctx) }()

	_, err = conn.Exec(ctx, `CREATE TEMP TABLE inline_messages (v jsonb[])`)
	require.NoError(t, err)
	_, err = conn.Exec(ctx, `INSERT INTO inline_messages (v) VALUES (`+strings.Join(rendered, `), (`)+`)`, pgx.QueryExecModeSimpleProtocol)
	require.NoError(t, err)

	var quote, empty string
	var nulls int
	require.NoError(t, conn.QueryRow(ctx, `SELECT
		(SELECT v[1]->>'a' FROM inline_messages WHERE array_length(v, 1) = 2),
		(SELECT v::text FROM inline_messages WHERE v = '{}'),
		(SELECT count(*) FROM inline_messages WHERE v IS NULL)`).Scan(&quote, &empty, &nulls))
	assert.Equal(t, `b"c\d`, quote)
	assert.Equal(t, "{}", empty)
	assert.Equal(t, 1, nulls)
}
