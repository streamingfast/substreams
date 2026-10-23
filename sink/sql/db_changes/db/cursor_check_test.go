package db

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/streamingfast/bstream"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pgTestSchema creates a fresh schema with the cursors and history tables plus an `acc` table, and
// returns the DSN pointing at it. The test is skipped unless PG_DSN is set.
func pgTestSchema(t *testing.T) string {
	t.Helper()
	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		t.Skip(`PG_DSN not set, please specify PG_DSN to run this test, example: PG_DSN="psql://dev-node:insecure-change-me-in-prod@localhost:5432/dev-node?sslmode=disable"`)
	}

	dsn, err := ParseDSN(dsnString)
	require.NoError(t, err)
	raw, err := sql.Open("postgres", dsn.ConnString())
	require.NoError(t, err)
	defer raw.Close()

	schema := fmt.Sprintf("writer_test_%d", time.Now().UnixNano())
	_, err = raw.Exec(fmt.Sprintf(`CREATE SCHEMA %s; CREATE TABLE %s.acc (id text PRIMARY KEY, n numeric NOT NULL DEFAULT 0)`, schema, schema))
	require.NoError(t, err)
	t.Cleanup(func() {
		raw, err := sql.Open("postgres", dsn.ConnString())
		if err == nil {
			raw.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", schema))
			raw.Close()
		}
	})

	scoped := dsnString + "&schemaName=" + schema
	l := pgTestLoader(t, scoped, false)
	require.NoError(t, l.Setup(context.Background(), schema, "", false))
	return scoped
}

func pgTestLoader(t *testing.T, dsnString string, loadTables bool) *Loader {
	t.Helper()
	dsn, err := ParseDSN(dsnString)
	require.NoError(t, err)
	l, err := NewLoader(dsn, testCursorTableName, testHistoryTableName, "", 0, 0, 0, OnModuleHashMismatchError.String(), nil, zlog, tracer)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })
	if loadTables {
		require.NoError(t, l.LoadTables(dsn.Schema(), testCursorTableName, testHistoryTableName))
	}
	return l
}

func pgCursor(num, lib uint64) *sink.Cursor {
	blk := bstream.NewBlockRef(fmt.Sprintf("%d", num), num)
	return sink.MustNewCursor((&bstream.Cursor{
		Step:      bstream.StepNewIrreversible,
		Block:     blk,
		LIB:       bstream.NewBlockRef(fmt.Sprintf("%d", lib), lib),
		HeadBlock: blk,
	}).ToOpaque())
}

func pgAccValue(t *testing.T, l *Loader, id string) string {
	t.Helper()
	var n string
	require.NoError(t, l.DB.QueryRow(fmt.Sprintf(`SELECT n::text FROM %s.acc WHERE id = $1`, l.dsn.Schema()), id).Scan(&n))
	return n
}

const testModuleHash = "abcd"

// readCursor reads the stored cursor and has l check it from then on, as SQLSinker.Run does.
func readCursor(t *testing.T, l *Loader) *sink.Cursor {
	t.Helper()
	cursor, rawCursor, _, err := l.GetCursor(context.Background(), testModuleHash)
	require.NoError(t, err)
	l.SetExpectedCursor(rawCursor)
	return cursor
}

// insertBlankCursor writes the initial cursor and has l check it from then on, as SQLSinker.Run does
// when there is none.
func insertBlankCursor(t *testing.T, l *Loader) {
	t.Helper()
	_, _, _, err := l.GetCursor(context.Background(), testModuleHash)
	require.ErrorIs(t, err, ErrCursorNotFound)
	blank := sink.NewBlankCursor()
	require.NoError(t, l.InsertCursor(context.Background(), testModuleHash, blank))
	l.SetExpectedCursor(blank.String())
}

func add(t *testing.T, l *Loader, id, n string) {
	t.Helper()
	require.NoError(t, l.Upsert("acc", map[string]string{"id": id}, map[string]FieldData{"n": {Value: n, UpdateOp: UpdateOpAdd}}, nil))
}

func TestCursorCheck_Postgres(t *testing.T) {
	ctx := context.Background()
	dsn := pgTestSchema(t)

	a := pgTestLoader(t, dsn, true)
	insertBlankCursor(t, a)

	// A second sink that read the same blank cursor
	b := pgTestLoader(t, dsn, true)
	readCursor(t, b)

	add(t, a, "x", "5")
	_, err := a.Flush(ctx, testModuleHash, pgCursor(10, 10), 10)
	require.NoError(t, err)

	t.Run("second writer is refused before writing", func(t *testing.T) {
		add(t, b, "x", "5")
		_, err := b.Flush(ctx, testModuleHash, pgCursor(10, 10), 10)
		require.ErrorIs(t, err, ErrCursorConflict)
		assert.Equal(t, "5", pgAccValue(t, a, "x"))
	})

	t.Run("replaying a commit that went through is refused", func(t *testing.T) {
		// As after a COMMIT whose reply was lost: the loader still expects the table before it
		add(t, a, "x", "7")
		_, err := a.Flush(ctx, testModuleHash, pgCursor(11, 11), 11)
		require.NoError(t, err)

		a.SetExpectedCursor(pgCursor(10, 10).String())
		add(t, a, "x", "7")
		_, err = a.Flush(ctx, testModuleHash, pgCursor(11, 11), 11)
		require.ErrorIs(t, err, ErrCursorConflict)
		assert.Equal(t, "12", pgAccValue(t, a, "x"), "the ADD must not be applied twice")
	})

	t.Run("other modules' cursor rows are not part of the check", func(t *testing.T) {
		c := pgTestLoader(t, dsn, true)
		readCursor(t, c)

		// Another module's row appears, changes and is deleted (e.g. cleanup of an old module version)
		_, err := c.DB.Exec(fmt.Sprintf(`INSERT INTO %s (id, cursor, block_num, block_id) VALUES ('beef', '', 0, '')`, c.cursorTable.identifier))
		require.NoError(t, err)
		add(t, c, "x", "1")
		_, err = c.Flush(ctx, testModuleHash, pgCursor(12, 12), 12)
		require.NoError(t, err)

		_, err = c.DB.Exec(fmt.Sprintf(`DELETE FROM %s WHERE id = 'beef'`, c.cursorTable.identifier))
		require.NoError(t, err)
		add(t, c, "x", "100")
		_, err = c.Flush(ctx, testModuleHash, pgCursor(13, 13), 13)
		require.NoError(t, err)
		assert.Equal(t, "113", pgAccValue(t, c, "x"))
	})

	t.Run("a cursor update outside a transaction is recorded", func(t *testing.T) {
		d := pgTestLoader(t, dsn, true)
		readCursor(t, d)
		require.NoError(t, d.UpdateCursor(ctx, nil, testModuleHash, pgCursor(14, 14)))
		add(t, d, "x", "0")
		_, err := d.Flush(ctx, testModuleHash, pgCursor(14, 14), 14)
		require.NoError(t, err, "its own write must not conflict")
	})

	t.Run("a restarted sink resumes from the stored cursor", func(t *testing.T) {
		c := pgTestLoader(t, dsn, true)
		cursor := readCursor(t, c)
		assert.Equal(t, uint64(14), cursor.Block().Num())

		add(t, c, "x", "1")
		_, err := c.Flush(ctx, testModuleHash, pgCursor(15, 15), 15)
		require.NoError(t, err)
		assert.Equal(t, "114", pgAccValue(t, c, "x"))
	})
}

// commitReplyDropper is a TCP proxy to PostgreSQL. Once armed, it forwards the next COMMIT to the
// server and closes the connection without relaying the reply: the commit happens, and the client
// sees an error.
type commitReplyDropper struct {
	ln      net.Listener
	target  string
	armed   atomic.Bool
	dropped atomic.Int32
}

func newCommitReplyDropper(t *testing.T, target string) *commitReplyDropper {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &commitReplyDropper{ln: ln, target: target}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			client, err := ln.Accept()
			if err != nil {
				return
			}
			go p.serve(client)
		}
	}()
	return p
}

func (p *commitReplyDropper) serve(client net.Conn) {
	defer client.Close()
	server, err := net.Dial("tcp", p.target)
	if err != nil {
		return
	}
	defer server.Close()

	var dropping atomic.Bool
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := server.Read(buf)
			if n > 0 && !dropping.Load() {
				client.Write(buf[:n])
			}
			if err != nil {
				client.Close()
				return
			}
		}
	}()

	in := bufio.NewReader(client)
	readFull := func(n int) ([]byte, error) {
		b := make([]byte, n)
		_, err := io.ReadFull(in, b)
		return b, err
	}

	// Startup message: length-prefixed, no type byte (sslmode=disable, so no SSLRequest)
	header, err := readFull(4)
	if err != nil {
		return
	}
	body, err := readFull(int(binary.BigEndian.Uint32(header)) - 4)
	if err != nil {
		return
	}
	server.Write(append(header, body...))

	for {
		header, err := readFull(5)
		if err != nil {
			return
		}
		body, err := readFull(int(binary.BigEndian.Uint32(header[1:])) - 4)
		if err != nil {
			return
		}
		if header[0] == 'Q' && bytes.HasPrefix(body, []byte("COMMIT")) && p.armed.CompareAndSwap(true, false) {
			dropping.Store(true)
			server.Write(append(header, body...))
			p.dropped.Add(1)
			client.Close()
			time.Sleep(500 * time.Millisecond) // let the server commit before it sees the connection go
			return
		}
		server.Write(append(header, body...))
	}
}

func TestLostCommitReply_Postgres(t *testing.T) {
	ctx := context.Background()
	dsn := pgTestSchema(t)

	u, err := url.Parse(dsn)
	require.NoError(t, err)
	proxy := newCommitReplyDropper(t, u.Host)
	u.Host = proxy.ln.Addr().String()

	a := pgTestLoader(t, u.String(), true)
	insertBlankCursor(t, a)

	add(t, a, "x", "5")
	_, err = a.Flush(ctx, testModuleHash, pgCursor(10, 10), 10)
	require.NoError(t, err)

	proxy.armed.Store(true)
	add(t, a, "x", "7")
	_, err = a.Flush(ctx, testModuleHash, pgCursor(11, 11), 11)
	require.Error(t, err, "the COMMIT reply was dropped")
	require.NotErrorIs(t, err, ErrCursorConflict)
	require.Equal(t, int32(1), proxy.dropped.Load())

	direct := pgTestLoader(t, dsn, true)
	cursor, _, _, err := direct.GetCursor(ctx, testModuleHash)
	require.NoError(t, err)
	require.Equal(t, uint64(11), cursor.Block().Num(), "the commit went through")
	require.Equal(t, "12", pgAccValue(t, direct, "x"))

	// The retry a sink does after a failed flush, with the same pending changes
	_, err = a.Flush(ctx, testModuleHash, pgCursor(11, 11), 11)
	require.ErrorIs(t, err, ErrCursorConflict)
	assert.Equal(t, "12", pgAccValue(t, direct, "x"), "the ADD must not be applied twice")
}
