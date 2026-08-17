package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
