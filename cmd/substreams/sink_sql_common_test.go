package main

import (
	"testing"

	"github.com/streamingfast/substreams/sink/sql/db_changes/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSinkUserAgent pins what the server sees: the two engines run the same commands, so
// the mode alone did not say where the rows went.
func TestSinkUserAgent(t *testing.T) {
	assert.Equal(t, "sink_relational_ch", sinkUserAgent("sink_relational", sinkClickhouseDriver))
	assert.Equal(t, "sink_relational_pg", sinkUserAgent("sink_relational", sinkPostgresDriver))
	assert.Equal(t, "sink_dbchanges_ch", sinkUserAgent("sink_dbchanges", sinkClickhouseDriver))
	assert.Equal(t, "sink_dbchanges_pg", sinkUserAgent("sink_dbchanges", sinkPostgresDriver))
	assert.Equal(t, "sink_dbchanges", sinkUserAgent("sink_dbchanges", "duckdb"))
}

func TestInjectDSNPassword(t *testing.T) {
	const password = `p@:/?#% '\${X}`
	const escaped = `p%40%3A%2F%3F%23%25%20%27%5C%24%7BX%7D`

	tests := []struct {
		name   string
		dsn    string
		expect string
		ok     bool
	}{
		{"user only", "clickhouse://default@host:9000/db", "clickhouse://default:" + escaped + "@host:9000/db", true},
		{"empty password", "clickhouse://default:@host:9000/db", "clickhouse://default:" + escaped + "@host:9000/db", true},
		{"no user info", "psql://localhost:5432/db?sslmode=disable", "psql://:" + escaped + "@localhost:5432/db?sslmode=disable", true},
		{"no path", "psql://postgres@localhost", "psql://postgres:" + escaped + "@localhost", true},
		{"query has an @", "psql://postgres@localhost/db?x=a@b", "psql://postgres:" + escaped + "@localhost/db?x=a@b", true},
		{"variable in user and host", "psql://${PGUSER}@${PGHOST}/db", "psql://${PGUSER}:" + escaped + "@${PGHOST}/db", true},
		{"password already set", "psql://postgres:secret@localhost/db", "psql://postgres:secret@localhost/db", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := injectDSNPassword(test.dsn, password)
			assert.Equal(t, test.expect, got)
			assert.Equal(t, test.ok, ok)
		})
	}
}

func TestInjectDSNPassword_RoundTrip(t *testing.T) {
	const password = `p@:/?#% '\${X}`
	for _, dsn := range []string{"clickhouse://default@host:9000/db", "psql://postgres@localhost/db"} {
		withPassword, ok := injectDSNPassword(dsn, password)
		require.True(t, ok)
		parsed, err := db.ParseDSN(withPassword)
		require.NoError(t, err, dsn)
		assert.Equal(t, password, parsed.Password, dsn)
	}
}
