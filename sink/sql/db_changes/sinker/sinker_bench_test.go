package sinker

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	pbdatabase "github.com/streamingfast/substreams-sink-database-changes/pb/sf/substreams/sink/database/v1"
	"github.com/streamingfast/substreams/manifest"
	sink "github.com/streamingfast/substreams/sink"
	db2 "github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/stretchr/testify/require"
)

// BenchmarkSinker_BackgroundFlush times catching up against the database in PG_DSN, with flushes
// running in the background ("on") or each one waited for before the next block ("off", as near
// the head). Each block takes blockWork to arrive and decode, then adds rowsPerBlock rows; a flush
// runs every flushEvery blocks, as by default. PG_DSN can go through a proxy to add network latency (see
// BenchmarkPostgresFlush).
func BenchmarkSinker_BackgroundFlush(b *testing.B) {
	// The CLI's default flush intervals.
	const (
		rowsPerBlock = 50
		flushEvery   = 1_000
		flushRows    = 100_000
		blockWork    = time.Millisecond
	)
	dsnString := os.Getenv("PG_DSN")
	if dsnString == "" {
		b.Skip("PG_DSN not set")
	}
	ctx := context.Background()
	moduleHash := manifest.ModuleHash([]byte("unused"))

	for _, background := range []bool{false, true} {
		name := map[bool]string{false: "off", true: "on"}[background]
		b.Run(name, func(b *testing.B) {
			rawDSN, err := db2.ParseDSN(dsnString)
			require.NoError(b, err)
			raw, err := sql.Open("pgx", rawDSN.ConnString())
			require.NoError(b, err)
			defer raw.Close()
			schema := fmt.Sprintf("background_flush_bench_%d", time.Now().UnixNano())
			_, err = raw.Exec(fmt.Sprintf(`CREATE SCHEMA %[1]s; CREATE TABLE %[1]s.xfer (id text PRIMARY KEY, "from" text, "to" text)`, schema))
			require.NoError(b, err)
			defer raw.Exec("DROP SCHEMA " + schema + " CASCADE")

			dsn, err := db2.ParseDSN(dsnString + "&schemaName=" + schema)
			require.NoError(b, err)
			l, err := db2.NewLoader(dsn, "cursors", "substreams_history", "", flushEvery, flushRows, 1, db2.OnModuleHashMismatchError.String(), nil, logger, tracer)
			require.NoError(b, err)
			require.NoError(b, l.Setup(ctx, schema, "", false))
			require.NoError(b, l.LoadTables(schema, "cursors", "substreams_history"))

			s, err := sink.NewFromConfig(&sink.SinkerConfig{
				Pkg:              testPackage,
				OutputModule:     testPackage.Modules.Modules[0],
				OutputModuleHash: moduleHash,
				ClientConfig:     testClientConfig,
				Mode:             sink.SubstreamsModeDevelopment,
				Logger:           logger,
			})
			require.NoError(b, err)
			sinker, err := New(s, l, logger, nil, 0, 0)
			require.NoError(b, err)
			require.NoError(b, l.InsertCursor(ctx, s.OutputModuleHash(), sink.MustNewCursor(simpleCursor(0, 0))))

			catchingUp := false
			block := uint64(0)
			for b.Loop() {
				block++
				changes := make([]*pbdatabase.TableChange, rowsPerBlock)
				for i := range changes {
					changes[i] = insertRowSinglePK("xfer", fmt.Sprintf("%d-%d", block, i), "from", "a", "to", "b")
				}
				time.Sleep(blockWork)
				require.NoError(b, sinker.HandleBlockScopedData(ctx, blockScopedData("db_out", changes, block, block), &catchingUp, sink.MustNewCursor(simpleCursor(block, block))))
				if !background {
					require.NoError(b, sinker.waitBackgroundFlush())
				}
			}
			require.NoError(b, sinker.waitBackgroundFlush())
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "blocks/s")
		})
	}
}
