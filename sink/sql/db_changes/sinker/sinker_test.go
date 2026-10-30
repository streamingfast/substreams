package sinker

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/streamingfast/bstream"
	"github.com/streamingfast/logging"
	sink "github.com/streamingfast/substreams/sink"
	pbdatabase "github.com/streamingfast/substreams-sink-database-changes/pb/sf/substreams/sink/database/v1"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/anypb"
)

var logger *zap.Logger
var tracer logging.Tracer

func init() {
	logger, tracer = logging.ApplicationLogger("test", "test")
}

func TestSinker_SQLStatements(t *testing.T) {
	tests := []struct {
		name      string
		events    []event
		expectSQL []string
	}{
		{
			name: "insert final block",
			events: []event{
				{
					blockNum:     10,
					libNum:       10,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 10;`,
				`UPDATE "testschema"."cursors" set cursor = 'bN7dsAhRyo44yl_ykkjA36WwLpc_DFtvXwrlIBBBj4r2', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},
		{
			name: "insert two final blocks",
			events: []event{
				{
					blockNum:     10,
					libNum:       10,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")},
				},
				{
					blockNum:     11,
					libNum:       11,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "2345", "from", "sender2", "to", "receiver2")},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 10;`,
				`UPDATE "testschema"."cursors" set cursor = 'bN7dsAhRyo44yl_ykkjA36WwLpc_DFtvXwrlIBBBj4r2', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender2','2345','receiver2');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 11;`,
				`UPDATE "testschema"."cursors" set cursor = 'dR5-m-1v1TQvlVRfIM9SXaWwLpc_DFtuXwrkIBBAj4r3', block_num = 11, block_id = '11' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},
		{
			name: "insert a reversible blocks",
			events: []event{
				{
					blockNum:     10,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234"}',10);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},
		{
			name: "insert, then update",
			events: []event{
				{
					blockNum:     10,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowMultiplePK("xfer", map[string]string{"id": "1234", "idx": "3"}, "from", "sender1", "to", "receiver1")},
				},
				{
					blockNum: 11,
					libNum:   6,
					tableChanges: []*pbdatabase.TableChange{
						updateRowMultiplePK("xfer", map[string]string{"id": "2345", "idx": "3"}, "from", "sender2", "to", "receiver2"),
					},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234","idx":"3"}',10);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,prev_value,block_num) SELECT 'U','"testschema"."xfer"','{"id":"2345","idx":"3"}',row_to_json("xfer"),11 FROM "testschema"."xfer" WHERE "id" = '2345' AND "idx" = '3';`,
				`UPDATE "testschema"."xfer" SET "from"='sender2', "to"='receiver2' WHERE "id" = '2345' AND "idx" = '3'`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 6;`,
				`UPDATE "testschema"."cursors" set cursor = 'LamYQ1PoEJyzLTRd7kdEiKWwLpcyB1tlVArvLBtH', block_num = 11, block_id = '11' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},

		{
			name: "insert, then update, then delete (update disappears)",
			events: []event{
				{
					blockNum:     10,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowMultiplePK("xfer", map[string]string{"id": "1234", "idx": "3"}, "from", "sender1", "to", "receiver1")},
				},
				{
					blockNum: 11,
					libNum:   6,
					tableChanges: []*pbdatabase.TableChange{
						updateRowMultiplePK("xfer", map[string]string{"id": "2345", "idx": "3"}, "from", "sender2", "to", "receiver2"),
						deleteRowMultiplePK("xfer", map[string]string{"id": "2345", "idx": "3"}),
					},
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234","idx":"3"}',10);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
				// the following gets deduped
				//`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,prev_value,block_num) SELECT 'U','"testschema"."xfer"','{"id":"2345","idx":"3"}',row_to_json("xfer"),11 FROM "testschema"."xfer" WHERE "id" = '2345' AND "idx" = '3';` +
				//	`UPDATE "testschema"."xfer" SET "from"='sender2', "to"='receiver2' WHERE "id" = '2345' AND "idx" = '3'`,
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,prev_value,block_num) SELECT 'D','"testschema"."xfer"','{"id":"2345","idx":"3"}',row_to_json("xfer"),11 FROM "testschema"."xfer" WHERE "id" = '2345' AND "idx" = '3';`,
				`DELETE FROM "testschema"."xfer" WHERE "id" = '2345' AND "idx" = '3'`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 6;`,
				`UPDATE "testschema"."cursors" set cursor = 'LamYQ1PoEJyzLTRd7kdEiKWwLpcyB1tlVArvLBtH', block_num = 11, block_id = '11' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},

		{
			name: "insert two reversible blocks, then UNDO last",
			events: []event{
				{
					blockNum:     10,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")},
				},
				{
					blockNum:     11,
					libNum:       5,
					tableChanges: []*pbdatabase.TableChange{insertRowSinglePK("xfer", "2345", "from", "sender2", "to", "receiver2")},
				},
				{
					blockNum:   10, // undo everything above 10
					libNum:     5,
					undoSignal: true,
				},
			},
			expectSQL: []string{
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234"}',10);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
				`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"2345"}',11);`,
				`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender2','2345','receiver2');`,
				`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
				`UPDATE "testschema"."cursors" set cursor = 'Euaqz6R-ylLG0gbdej7Me6WwLpcyB1tlVArvLxtE', block_num = 11, block_id = '11' WHERE id = '756e75736564';`,
				`COMMIT`,
				`SELECT op,table_name,pk,prev_value,block_num FROM "testschema"."substreams_history" WHERE "block_num" > 10 ORDER BY "block_num" DESC`,

				//`DELETE FROM "testschema"."xfer" WHERE "id" = "2345";`, // this mechanism is tested in db.revertOp
				`DELETE FROM "testschema"."substreams_history" WHERE "block_num" > 10;`,
				`UPDATE "testschema"."cursors" set cursor = 'i4tY9gOcWnhKoGjRCl2VUKWwLpcyB1plVAvvLxtE', block_num = 10, block_id = '10' WHERE id = '756e75736564';`,
				`COMMIT`,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			tx := &db2.TestTx{}
			l := db2.NewTestLoader(
				t,
				"psql://x:5432/x?schemaName=testschema",
				tx,
				db2.TestSinglePrimaryKeyTables("testschema"),
				logger,
				tracer,
			)
			s, err := sink.NewFromConfig(&sink.SinkerConfig{
				Pkg:              testPackage,
				OutputModule:     testPackage.Modules.Modules[0],
				OutputModuleHash: manifest.ModuleHash([]byte("unused")),
				ClientConfig:     testClientConfig,
				Mode:             sink.SubstreamsModeDevelopment,
				NoopMode:         false,
				Logger:           logger,
				Tracer:           nil,
			})
			require.NoError(t, err)
			sinker, _ := New(s, l, logger, nil, 3, 1*time.Second)

			for _, evt := range test.events {
				if evt.undoSignal {
					cursor := simpleCursor(evt.blockNum, evt.libNum)
					err := sinker.HandleBlockUndoSignal(ctx, &pbsubstreamsrpc.BlockUndoSignal{
						LastValidBlock:  &pbsubstreams.BlockRef{Id: fmt.Sprintf("%d", evt.blockNum), Number: evt.blockNum},
						LastValidCursor: cursor,
					}, sink.MustNewCursor(cursor))
					require.NoError(t, err)
					continue
				}

				err := sinker.HandleBlockScopedData(
					ctx,
					blockScopedData("db_out", evt.tableChanges, evt.blockNum, evt.libNum),
					flushEveryBlock, sink.MustNewCursor(simpleCursor(evt.blockNum, evt.libNum)),
				)
				require.NoError(t, err)
			}

			results := tx.Results()
			assert.Equal(t, test.expectSQL, results)
		})
	}
}

type event struct {
	blockNum     uint64
	libNum       uint64
	tableChanges []*pbdatabase.TableChange
	undoSignal   bool
}

// TestSinker_CursorCheck checks that once the stored cursor is known, each flush and revert starts by
// checking it is still the one this sink last read or wrote, keeps the cursor update last, and that a
// conflict rolls the transaction back without retrying.
func TestSinker_CursorCheck(t *testing.T) {
	const module = "756e75736564" // manifest.ModuleHash("unused")
	cursorAt := func(num, lib uint64) string { return sink.MustNewCursor(simpleCursor(num, lib)).String() }
	update := func(num, lib uint64) string {
		return fmt.Sprintf(`UPDATE "testschema"."cursors" set cursor = '%s', block_num = %d, block_id = '%d' WHERE id = '%s';`, cursorAt(num, lib), num, num, module)
	}
	isCheck := func(query string) bool { return strings.HasSuffix(query, `COLLATE "C" FOR UPDATE;`) }

	newSinker := func(t *testing.T, tx *db2.TestTx, expected string) (*SQLSinker, *db2.Loader) {
		l := db2.NewTestLoader(t, "psql://x:5432/x?schemaName=testschema", tx, db2.TestSinglePrimaryKeyTables("testschema"), logger, tracer)
		l.SetExpectedCursor(expected)
		s, err := sink.NewFromConfig(&sink.SinkerConfig{
			Pkg:              testPackage,
			OutputModule:     testPackage.Modules.Modules[0],
			OutputModuleHash: manifest.ModuleHash([]byte("unused")),
			ClientConfig:     testClientConfig,
			Mode:             sink.SubstreamsModeDevelopment,
			Logger:           logger,
		})
		require.NoError(t, err)
		sinker, err := New(s, l, logger, nil, 3, 1*time.Second)
		require.NoError(t, err)
		return sinker, l
	}

	t.Run("flushes and revert", func(t *testing.T) {
		ctx := context.Background()
		tx := &db2.TestTx{}
		sinker, l := newSinker(t, tx, "")

		require.NoError(t, sinker.HandleBlockScopedData(ctx,
			blockScopedData("db_out", []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")}, 10, 5),
			flushEveryBlock, sink.MustNewCursor(simpleCursor(10, 5))))
		require.NoError(t, sinker.HandleBlockScopedData(ctx,
			blockScopedData("db_out", []*pbdatabase.TableChange{insertRowSinglePK("xfer", "2345", "from", "sender2", "to", "receiver2")}, 11, 5),
			flushEveryBlock, sink.MustNewCursor(simpleCursor(11, 5))))
		require.NoError(t, sinker.HandleBlockUndoSignal(ctx, &pbsubstreamsrpc.BlockUndoSignal{
			LastValidBlock:  &pbsubstreams.BlockRef{Id: "10", Number: 10},
			LastValidCursor: simpleCursor(10, 5),
		}, sink.MustNewCursor(simpleCursor(10, 5))))

		assert.Equal(t, []string{
			l.CursorCheckQuery(module, ""),
			`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"1234"}',10);`,
			`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender1','1234','receiver1');`,
			`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
			update(10, 5),
			`COMMIT`,
			l.CursorCheckQuery(module, cursorAt(10, 5)),
			`INSERT INTO "testschema"."substreams_history" (op,table_name,pk,block_num) values ('I','"testschema"."xfer"','{"id":"2345"}',11);`,
			`INSERT INTO "testschema"."xfer" ("from","id","to") VALUES ('sender2','2345','receiver2');`,
			`DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`,
			update(11, 5),
			`COMMIT`,
			l.CursorCheckQuery(module, cursorAt(11, 5)),
			`SELECT op,table_name,pk,prev_value,block_num FROM "testschema"."substreams_history" WHERE "block_num" > 10 ORDER BY "block_num" DESC`,
			`DELETE FROM "testschema"."substreams_history" WHERE "block_num" > 10;`,
			update(10, 5),
			`COMMIT`,
		}, tx.Results())
	})

	t.Run("conflict is not retried", func(t *testing.T) {
		ctx := context.Background()
		tx := &db2.TestTx{NoRowsAffected: isCheck}
		sinker, l := newSinker(t, tx, cursorAt(9, 5))

		err := sinker.HandleBlockScopedData(ctx,
			blockScopedData("db_out", []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")}, 10, 5),
			flushEveryBlock, sink.MustNewCursor(simpleCursor(10, 5)))
		require.ErrorIs(t, err, db2.ErrCursorConflict)

		// One attempt, rolled back before any write
		assert.Equal(t, []string{l.CursorCheckQuery(module, cursorAt(9, 5)), `ROLLBACK`}, tx.Results())
	})

	t.Run("conflict while stopping is returned as a conflict", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		tx := &db2.TestTx{NoRowsAffected: isCheck}
		sinker, _ := newSinker(t, tx, cursorAt(9, 5))

		err := sinker.HandleBlockScopedData(ctx,
			blockScopedData("db_out", []*pbdatabase.TableChange{insertRowSinglePK("xfer", "1234", "from", "sender1", "to", "receiver1")}, 10, 5),
			flushEveryBlock, sink.MustNewCursor(simpleCursor(10, 5)))
		require.ErrorIs(t, err, db2.ErrCursorConflict)
		require.NotErrorIs(t, err, context.Canceled, "a conflict is a failure, not the stop")
	})

	t.Run("conflict on revert", func(t *testing.T) {
		ctx := context.Background()
		tx := &db2.TestTx{NoRowsAffected: isCheck}
		sinker, l := newSinker(t, tx, cursorAt(11, 5))

		err := sinker.HandleBlockUndoSignal(ctx, &pbsubstreamsrpc.BlockUndoSignal{
			LastValidBlock:  &pbsubstreams.BlockRef{Id: "10", Number: 10},
			LastValidCursor: simpleCursor(10, 5),
		}, sink.MustNewCursor(simpleCursor(10, 5)))
		require.ErrorIs(t, err, db2.ErrCursorConflict)
		assert.Equal(t, []string{l.CursorCheckQuery(module, cursorAt(11, 5)), `ROLLBACK`}, tx.Results())
	})
}

// TestSinker_CompletionFlush checks the final flush when the stream ends, whether stopped or at its end:
// it writes the handled blocks with their own cursor and keeps the undo history of the blocks that are
// still reversible, instead of treating the stream's latest block as final. Nothing is written after a
// block failed to be handled.
func TestSinker_CompletionFlush(t *testing.T) {
	newSinker := func(t *testing.T, tx *db2.TestTx) *SQLSinker {
		l := db2.NewTestLoader(t, "psql://x:5432/x?schemaName=testschema", tx, db2.TestSinglePrimaryKeyTables("testschema"), logger, tracer)
		l.SetFlushIntervals(1000, 1_000_000, 1)
		s, err := sink.NewFromConfig(&sink.SinkerConfig{
			Pkg:              testPackage,
			OutputModule:     testPackage.Modules.Modules[0],
			OutputModuleHash: manifest.ModuleHash([]byte("unused")),
			ClientConfig:     testClientConfig,
			Mode:             sink.SubstreamsModeDevelopment,
			Logger:           logger,
		})
		require.NoError(t, err)
		sinker, err := New(s, l, logger, nil, 3, time.Second)
		require.NoError(t, err)
		return sinker
	}

	handle := func(t *testing.T, sinker *SQLSinker, n uint64, table string) error {
		catchingUp := false
		return sinker.HandleBlockScopedData(context.Background(),
			blockScopedData("db_out", []*pbdatabase.TableChange{insertRowSinglePK(table, fmt.Sprintf("%d", n), "from", "a", "to", "b")}, n, 5),
			&catchingUp, sink.MustNewCursor(simpleCursor(n, 5)))
	}

	// The stream's cursor is at 13 while the sink handled 10-11 (blocks 12-13 still in an undo buffer)
	streamCursor := sink.MustNewCursor(simpleCursor(13, 5))
	expectHandledBlocksFlushed := func(t *testing.T, results []string) {
		assert.Contains(t, results, `DELETE FROM "testschema"."substreams_history" WHERE block_num <= 5;`)
		assert.Contains(t, results, fmt.Sprintf(`UPDATE "testschema"."cursors" set cursor = '%s', block_num = 11, block_id = '11' WHERE id = '756e75736564';`, sink.MustNewCursor(simpleCursor(11, 5))))
		for _, q := range results {
			assert.NotContains(t, q, "block_num <= 13", "the stream's block is not final")
			assert.NotContains(t, q, "block_num = 13")
		}
	}

	t.Run("stopped", func(t *testing.T) {
		tx := &db2.TestTx{}
		sinker := newSinker(t, tx)
		require.NoError(t, handle(t, sinker, 10, "xfer"))
		require.NoError(t, handle(t, sinker, 11, "xfer"))
		require.Empty(t, tx.Results(), "nothing flushed yet")

		sinker.Shutdown(nil)
		require.NoError(t, sinker.HandleBlockRangeCompletion(context.Background(), streamCursor))
		expectHandledBlocksFlushed(t, tx.Results())
	})

	t.Run("context cancelled", func(t *testing.T) {
		tx := &db2.TestTx{}
		sinker := newSinker(t, tx)
		require.NoError(t, handle(t, sinker, 10, "xfer"))
		require.NoError(t, handle(t, sinker, 11, "xfer"))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, sinker.HandleBlockRangeCompletion(ctx, streamCursor))
		expectHandledBlocksFlushed(t, tx.Results())
	})

	t.Run("stream ended", func(t *testing.T) {
		tx := &db2.TestTx{}
		sinker := newSinker(t, tx)
		require.NoError(t, handle(t, sinker, 10, "xfer"))
		require.NoError(t, handle(t, sinker, 11, "xfer"))

		require.NoError(t, sinker.HandleBlockRangeCompletion(context.Background(), streamCursor))
		expectHandledBlocksFlushed(t, tx.Results())
	})

	t.Run("nothing written after a failed block", func(t *testing.T) {
		tx := &db2.TestTx{}
		sinker := newSinker(t, tx)
		require.NoError(t, handle(t, sinker, 10, "xfer"))
		require.Error(t, handle(t, sinker, 11, "unknown_table"))

		sinker.Shutdown(nil)
		require.NoError(t, sinker.HandleBlockRangeCompletion(context.Background(), streamCursor))
		assert.Empty(t, tx.Results())
	})
}

func TestSinker_StallTimeout(t *testing.T) {
	newSinker := func(t *testing.T) *SQLSinker {
		l := db2.NewTestLoader(t, "psql://x:5432/x?schemaName=testschema", &db2.TestTx{}, db2.TestSinglePrimaryKeyTables("testschema"), logger, tracer)
		s, err := sink.NewFromConfig(&sink.SinkerConfig{
			Pkg:              testPackage,
			OutputModule:     testPackage.Modules.Modules[0],
			OutputModuleHash: manifest.ModuleHash([]byte("unused")),
			ClientConfig:     testClientConfig,
			Mode:             sink.SubstreamsModeDevelopment,
			Logger:           logger,
		})
		require.NoError(t, err)
		sinker, err := New(s, l, logger, nil, 3, time.Second)
		require.NoError(t, err)
		sinker.SetStallTimeout(100 * time.Millisecond)
		return sinker
	}

	t.Run("stops when live and no block arrives", func(t *testing.T) {
		sinker := newSinker(t)
		sinker.recordBlockReceived(flushEveryBlock) // live
		go sinker.watchStall()

		select {
		case <-sinker.Terminating():
			require.ErrorContains(t, sinker.Err(), "no block received")
		case <-time.After(2 * time.Second):
			t.Fatal("stalled sink was not stopped")
		}
	})

	t.Run("does not stop while catching up", func(t *testing.T) {
		sinker := newSinker(t)
		catchingUp := false
		sinker.recordBlockReceived(&catchingUp)
		go sinker.watchStall()
		defer sinker.Shutdown(nil)

		select {
		case <-sinker.Terminating():
			t.Fatalf("stopped while catching up: %v", sinker.Err())
		case <-time.After(500 * time.Millisecond):
		}
	})
}

// The active sink keeps the writer lock's session from idling out, and stops once that session is gone.
func TestSinker_WriterSessionKeepalive_Postgres(t *testing.T) {
	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		t.Skip("PG_DSN not set")
	}
	ctx := context.Background()

	raw, err := sql.Open("postgres", mustDSN(t, dsnString).ConnString())
	require.NoError(t, err)
	defer raw.Close()
	schema := fmt.Sprintf("keepalive_test_%d", time.Now().UnixNano())
	_, err = raw.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	defer raw.Exec("DROP SCHEMA " + schema + " CASCADE")

	dsn := mustDSN(t, dsnString+"&schemaName="+schema)
	l, err := db2.NewLoader(dsn, "cursors", "substreams_history", "", 0, 0, 0, db2.OnModuleHashMismatchError.String(), nil, logger, tracer)
	require.NoError(t, err)
	require.NoError(t, l.Setup(ctx, schema, "", false))
	require.NoError(t, l.LoadTables(schema, "cursors", "substreams_history"))

	s, err := sink.NewFromConfig(&sink.SinkerConfig{
		Pkg:              testPackage,
		OutputModule:     testPackage.Modules.Modules[0],
		OutputModuleHash: manifest.ModuleHash([]byte("unused")),
		ClientConfig:     testClientConfig,
		Mode:             sink.SubstreamsModeDevelopment,
		Logger:           logger,
	})
	require.NoError(t, err)
	sinker, err := New(s, l, logger, nil, 3, time.Second)
	require.NoError(t, err)
	defer sinker.Close()
	sinker.SetWriterLock(100 * time.Millisecond)
	require.True(t, sinker.acquireWriterLock(ctx))

	var pid int
	require.NoError(t, raw.QueryRow(`SELECT pid FROM pg_locks WHERE locktype = 'advisory' AND granted AND objid = $1`, int64(uint32(l.WriterLockKey()))).Scan(&pid))

	stop := sinker.keepWriterSessionAlive(ctx)
	defer stop()

	t.Run("an idle session is kept", func(t *testing.T) {
		var idle time.Duration
		for range 10 {
			time.Sleep(50 * time.Millisecond)
			var secs float64
			require.NoError(t, raw.QueryRow(`SELECT EXTRACT(EPOCH FROM now() - state_change) FROM pg_stat_activity WHERE pid = $1`, pid).Scan(&secs))
			idle = max(idle, time.Duration(secs*float64(time.Second)))
		}
		require.Less(t, idle, 400*time.Millisecond, "the session was left idle")
		require.False(t, sinker.IsTerminating())
	})

	t.Run("the sink stops once the session is gone", func(t *testing.T) {
		_, err := raw.Exec("SELECT pg_terminate_backend($1)", pid)
		require.NoError(t, err)
		select {
		case <-sinker.Terminating():
			require.ErrorContains(t, sinker.Err(), "lost the database session holding the writer lock")
		case <-time.After(3 * time.Second):
			t.Fatal("the sink kept running without its session")
		}
	})
}

func mustDSN(t *testing.T, s string) *db2.DSN {
	dsn, err := db2.ParseDSN(s)
	require.NoError(t, err)
	return dsn
}

var T = true
var flushEveryBlock = &T

var testPackage = &pbsubstreams.Package{
	Modules: &pbsubstreams.Modules{
		Modules: []*pbsubstreams.Module{
			{
				Name: "db_out",
				Kind: &pbsubstreams.Module_KindMap_{},
				Output: &pbsubstreams.Module_Output{
					Type: "proto:sf.substreams.sink.database.v1.DatabaseChanges",
				},
			},
		},
	},
}

var testClientConfig = &client.SubstreamsClientConfig{}

func getFields(fieldsAndValues ...string) (out []*pbdatabase.Field) {
	if len(fieldsAndValues)%2 != 0 {
		panic("tableChangeSinglePK needs even number of fieldsAndValues")
	}
	for i := 0; i < len(fieldsAndValues); i += 2 {
		out = append(out, &pbdatabase.Field{
			Name:  fieldsAndValues[i],
			Value: fieldsAndValues[i+1],
		})
	}
	return
}

func insertRowSinglePK(table string, pk string, fieldsAndValues ...string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_Pk{
			Pk: pk,
		},
		Operation: pbdatabase.TableChange_OPERATION_CREATE,
		Fields:    getFields(fieldsAndValues...),
	}
}

func insertRowMultiplePK(table string, pk map[string]string, fieldsAndValues ...string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_CREATE,
		Fields:    getFields(fieldsAndValues...),
	}
}

func updateRowMultiplePK(table string, pk map[string]string, fieldsAndValues ...string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_UPDATE,
		Fields:    getFields(fieldsAndValues...),
	}
}
func deleteRowMultiplePK(table string, pk map[string]string) *pbdatabase.TableChange {
	return &pbdatabase.TableChange{
		Table: table,
		PrimaryKey: &pbdatabase.TableChange_CompositePk{
			CompositePk: &pbdatabase.CompositePrimaryKey{
				Keys: pk,
			},
		},
		Operation: pbdatabase.TableChange_OPERATION_DELETE,
	}
}

func blockScopedData(module string, changes []*pbdatabase.TableChange, blockNum uint64, finalBlockNum uint64) *pbsubstreamsrpc.BlockScopedData {
	mapOutput, err := anypb.New(&pbdatabase.DatabaseChanges{
		TableChanges: changes,
	})
	if err != nil {
		panic(err)
	}

	return &pbsubstreamsrpc.BlockScopedData{
		Output: &pbsubstreamsrpc.MapModuleOutput{
			Name:      module,
			MapOutput: mapOutput,
		},
		Clock:            clock(fmt.Sprintf("%d", blockNum), blockNum),
		Cursor:           simpleCursor(blockNum, finalBlockNum),
		FinalBlockHeight: finalBlockNum,
	}
}

func clock(id string, num uint64) *pbsubstreams.Clock {
	return &pbsubstreams.Clock{Id: id, Number: num}
}

func simpleCursor(num, finalNum uint64) string {
	id := fmt.Sprintf("%d", num)
	finalID := fmt.Sprintf("%d", finalNum)
	blk := bstream.NewBlockRef(id, num)
	lib := bstream.NewBlockRef(finalID, finalNum)
	step := bstream.StepNew
	if id == finalID {
		step = bstream.StepNewIrreversible
	}

	return (&bstream.Cursor{
		Step:      step,
		Block:     blk,
		LIB:       lib,
		HeadBlock: blk,
	}).ToOpaque()
}

// TestProtoUpdateOpToDbUpdateOp tests the proto-to-db UpdateOp converter
func TestProtoUpdateOpToDbUpdateOp(t *testing.T) {
	tests := []struct {
		name     string
		protoOp  pbdatabase.Field_UpdateOp
		expected db2.UpdateOp
	}{
		{"SET (default)", pbdatabase.Field_UPDATE_OP_SET, db2.UpdateOpSet},
		{"ADD", pbdatabase.Field_UPDATE_OP_ADD, db2.UpdateOpAdd},
		{"MAX", pbdatabase.Field_UPDATE_OP_MAX, db2.UpdateOpMax},
		{"MIN", pbdatabase.Field_UPDATE_OP_MIN, db2.UpdateOpMin},
		{"SET_IF_NULL", pbdatabase.Field_UPDATE_OP_SET_IF_NULL, db2.UpdateOpSetIfNull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := protoUpdateOpToDbUpdateOp(tt.protoOp)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestProtoUpdateOpToDbUpdateOp_UnknownValue tests that unknown proto values default to SET
func TestProtoUpdateOpToDbUpdateOp_UnknownValue(t *testing.T) {
	// Test with an unknown/invalid proto value - should default to SET
	unknownOp := pbdatabase.Field_UpdateOp(999)
	result := protoUpdateOpToDbUpdateOp(unknownOp)
	assert.Equal(t, db2.UpdateOpSet, result, "unknown proto values should default to SET")
}
