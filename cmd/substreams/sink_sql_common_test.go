package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSinkUserAgent pins what the server sees: the two engines run the same commands, so
// the mode alone did not say where the rows went.
func TestSinkUserAgent(t *testing.T) {
	assert.Equal(t, "sink_from_proto_ch", sinkUserAgent("sink_from_proto", sinkClickhouseDriver))
	assert.Equal(t, "sink_from_proto_pg", sinkUserAgent("sink_from_proto", sinkPostgresDriver))
	assert.Equal(t, "sink_database_changes_ch", sinkUserAgent("sink_database_changes", sinkClickhouseDriver))
	assert.Equal(t, "sink_database_changes_pg", sinkUserAgent("sink_database_changes", sinkPostgresDriver))
	assert.Equal(t, "sink_from_proto", sinkUserAgent("sink_from_proto", "duckdb"))
}
