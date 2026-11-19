package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/streamingfast/bstream"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/sinker"
	"github.com/stretchr/testify/require"
)

const clickhouseStatefulSchema = `
	CREATE TABLE IF NOT EXISTS balances (
		id String,
		amount Int64,
		peak Nullable(Int64),
		_version_ UInt64,
		_deleted_ UInt8
	) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY id SETTINGS index_granularity = 256;

	CREATE TABLE IF NOT EXISTS transfers (
		id String,
		amount Int64,
		_version_ UInt64
	) ENGINE = MergeTree ORDER BY id;
`

type statefulBalanceRow struct {
	ID     string `db:"id"`
	Amount int64  `db:"amount"`
	Peak   *int64 `db:"peak"`
}

func equalsStatefulBalances(expected []*statefulBalanceRow, expectedTransfers []string) func(t *testing.T, dbx *sqlx.DB, schema string) {
	return func(t *testing.T, dbx *sqlx.DB, schema string) {
		var balances []*statefulBalanceRow
		require.NoError(t, dbx.SelectContext(context.Background(), &balances,
			fmt.Sprintf("SELECT id, amount, peak FROM `%s`.balances FINAL WHERE _deleted_ = 0 ORDER BY id", schema)))
		require.Equal(t, expected, balances)

		var transfers []string
		require.NoError(t, dbx.SelectContext(context.Background(), &transfers,
			fmt.Sprintf("SELECT id FROM `%s`.transfers ORDER BY id", schema)))
		require.Equal(t, expectedTransfers, transfers)
	}
}

func statefulOptions(defaults sinker.SinkerFactoryOptions) sinker.SinkerFactoryOptions {
	defaults.ClickhouseStateful = true
	return defaults
}

func TestClickhouseSinker_Integration_Stateful(t *testing.T) {
	tests := []sinkerTestCase{
		{
			"deltas over final blocks",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("balances", "alice", "amount", deltaAdd("10"), "peak", deltaMax("10")),
					insertRowSinglePK("transfers", "t1", "amount", "10"),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					upsertRowSinglePK("balances", "alice", "amount", deltaSub("4"), "peak", deltaMax("6")),
					upsertRowSinglePK("balances", "bob", "amount", deltaAdd("4")),
					insertRowSinglePK("transfers", "t2", "amount", "4"),
				),
				dbChangesBlockData(t, "12a", finalBlock("12a"),
					upsertRowSinglePK("balances", "bob", "amount", deltaAdd("1"), "peak", setIfNull("5")),
					upsertRowSinglePK("balances", "carol", "amount", "7"),
				),
			),
			equalsStatefulBalances([]*statefulBalanceRow{
				{ID: "alice", Amount: 6, Peak: new(int64(10))},
				{ID: "bob", Amount: 5, Peak: new(int64(5))},
				{ID: "carol", Amount: 7},
			}, []string{"t1", "t2"}),
			"Block #12 (12a) - LIB #12 (12a)",
		},
		{
			"delete then upsert again",
			streamMock(
				dbChangesBlockData(t, "10a", finalBlock("10a"),
					upsertRowSinglePK("balances", "alice", "amount", deltaAdd("10")),
				),
				dbChangesBlockData(t, "11a", finalBlock("11a"),
					deleteRowMultiplePK("balances", map[string]string{"id": "alice"}),
				),
				dbChangesBlockData(t, "12a", finalBlock("12a"),
					upsertRowSinglePK("balances", "alice", "amount", deltaAdd("3")),
				),
			),
			equalsStatefulBalances([]*statefulBalanceRow{
				{ID: "alice", Amount: 3},
			}, nil),
			"Block #12 (12a) - LIB #12 (12a)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runClickhouseSinkerTest(t, sharedDbChangesClickhouseContainer, nil, statefulOptions,
				rawSQLInput(func(string) string { return clickhouseStatefulSchema }),
				test.responses, test.expected, test.expectedFinalCursor)
		})
	}
}

// TestClickhouseSinker_Integration_StatefulUndoBuffer checks that with the undo buffer, the
// deltas of a forked block never reach ClickHouse.
func TestClickhouseSinker_Integration_StatefulUndoBuffer(t *testing.T) {
	runClickhouseSinkerTest(t, sharedDbChangesClickhouseContainer,
		func(defaults []sink.Option) []sink.Option {
			return append(defaults, sink.WithBlockDataBuffer(2))
		},
		statefulOptions,
		rawSQLInput(func(string) string { return clickhouseStatefulSchema }),
		streamMock(
			dbChangesBlockData(t, "10a", finalBlock("9a"), upsertRowSinglePK("balances", "alice", "amount", deltaAdd("10"))),
			dbChangesBlockData(t, "11a", finalBlock("9a"), upsertRowSinglePK("balances", "alice", "amount", deltaAdd("100"))),
			blockUndo(t, "10a", finalBlock("9a")),
			dbChangesBlockData(t, "11b", finalBlock("9a"), upsertRowSinglePK("balances", "alice", "amount", deltaAdd("1"))),
			dbChangesBlockData(t, "12b", finalBlock("9a"), upsertRowSinglePK("balances", "alice", "amount", deltaAdd("2"))),
			dbChangesBlockData(t, "13b", finalBlock("9a"), upsertRowSinglePK("balances", "alice", "amount", deltaAdd("3"))),
		),
		// With a buffer of 2, blocks up to 11b are written: 10 + 1, never the 100 of 11a.
		equalsStatefulBalances([]*statefulBalanceRow{
			{ID: "alice", Amount: 11},
		}, nil),
		"Block #11 (11b) - LIB #9 (9a)",
	)
}

// TestClickhouseSinker_Integration_StatefulReorgs runs the cases of
// TestSinker_Integration_ReversibleBlocksInOneBatch: undos revert exactly the orphaned blocks,
// flushed or still pending.
func TestClickhouseSinker_Integration_StatefulReorgs(t *testing.T) {
	type counterRow struct {
		ID    string `db:"id"`
		Count int64  `db:"count"`
	}
	equalsCounter := func(count int64) func(t *testing.T, dbx *sqlx.DB, schema string) {
		return func(t *testing.T, dbx *sqlx.DB, schema string) {
			var rows []*counterRow
			require.NoError(t, dbx.SelectContext(context.Background(), &rows,
				fmt.Sprintf("SELECT id, count FROM `%s`.counters FINAL WHERE _deleted_ = 0 ORDER BY id", schema)))
			require.Equal(t, []counterRow{{ID: "counter1", Count: count}}, derefCounters(rows))
		}
	}

	tests := []struct {
		sinkerTestCase
		liveBlockFlushInterval int
		stopBlock              uint64
	}{
		{
			sinkerTestCase{
				"add to same row in two reversible blocks of one batch, undo the later block",
				streamMock(
					dbChangesBlockData(t, "9a", finalBlock("8a")),
					dbChangesBlockData(t, "10a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					dbChangesBlockData(t, "11a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					blockUndo(t, "10a", finalBlock("8a")),
				),
				equalsCounter(1),
				"Block #10 (10a) - LIB #8 (8a)",
			},
			2, 0,
		},
		{
			sinkerTestCase{
				"add to same row in a final and a reversible block of one batch, undo the reversible block",
				streamMock(
					dbChangesBlockData(t, "7a", finalBlock("7a")),
					dbChangesBlockData(t, "8a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					dbChangesBlockData(t, "9a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					blockUndo(t, "8a", finalBlock("8a")),
				),
				equalsCounter(1),
				"Block #8 (8a) - LIB #8 (8a)",
			},
			2, 0,
		},
		{
			sinkerTestCase{
				"undo with pending changes writes the valid ones in the revert",
				streamMock(
					dbChangesBlockData(t, "10a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					dbChangesBlockData(t, "11a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					blockUndo(t, "10a", finalBlock("8a")),
				),
				equalsCounter(1),
				"Block #10 (10a) - LIB #8 (8a)",
			},
			1000, 0,
		},
		{
			sinkerTestCase{
				"undo with pending changes drops the orphaned block and writes the valid one",
				streamMock(
					dbChangesBlockData(t, "10a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					dbChangesBlockData(t, "11a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					blockUndo(t, "10a", finalBlock("8a")),
					dbChangesBlockData(t, "11b", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("10"))),
				),
				equalsCounter(11),
				"Block #11 (11b) - LIB #8 (8a)",
			},
			1000, 12,
		},
		{
			sinkerTestCase{
				"flushed reversible blocks reverted, new fork applied",
				streamMock(
					dbChangesBlockData(t, "10a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1"))),
					dbChangesBlockData(t, "11a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("100"))),
					dbChangesBlockData(t, "12a", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("1000"))),
					blockUndo(t, "10a", finalBlock("8a")),
					dbChangesBlockData(t, "11b", finalBlock("8a"), upsertRowSinglePK("counters", "counter1", "count", deltaAdd("2"))),
				),
				equalsCounter(3),
				"Block #11 (11b) - LIB #8 (8a)",
			},
			1, 12,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runClickhouseSinkerTest(
				t,
				sharedDbChangesClickhouseContainer,
				func(defaults []sink.Option) []sink.Option {
					if test.stopBlock == 0 {
						return defaults
					}
					return append(defaults, sink.WithBlockRange(bstream.MustParseRange(fmt.Sprintf("1-%d", test.stopBlock), bstream.WithExclusiveEnd())))
				},
				func(defaults sinker.SinkerFactoryOptions) sinker.SinkerFactoryOptions {
					defaults.ClickhouseStateful = true
					defaults.HandleReorgs = true
					defaults.LiveBlockFlushInterval = test.liveBlockFlushInterval
					defaults.BatchRowFlushInterval = 1000
					return defaults
				},
				rawSQLInput(func(string) string {
					return `
						CREATE TABLE IF NOT EXISTS counters (
							id String,
							count Int64,
							_version_ UInt64,
							_deleted_ UInt8
						) ENGINE = ReplacingMergeTree(_version_, _deleted_) ORDER BY id;
					`
				}),
				test.responses,
				test.expected,
				test.expectedFinalCursor,
			)
		})
	}
}

func derefCounters[T any](rows []*T) []T {
	out := make([]T, len(rows))
	for i, r := range rows {
		out[i] = *r
	}
	return out
}
