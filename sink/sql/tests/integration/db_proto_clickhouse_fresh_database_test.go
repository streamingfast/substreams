package tests

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streamingfast/substreams/sink/sql/db_proto"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	pbrelations "github.com/streamingfast/substreams/sink/sql/tests/relations"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
)

// TestDbProtoClickhouseSetupOnAFreshDatabase covers pointing the sink at a server that
// does not hold the database yet, which is what `substreams sink clickhouse setup` is for
// and the one case every other ClickHouse test skips by creating it first.
//
// The schema check that runs ahead of CreateDatabase used to connect to the database it
// was about to create. ClickHouse refuses that with UNKNOWN_DATABASE, and newClient
// retries a failed dial forever on a context of its own, so setup never came back.
func TestDbProtoClickhouseSetupOnAFreshDatabase(t *testing.T) {
	outputMessageDescriptor := (*pbrelations.Output)(nil).ProtoReflect().Descriptor()

	var seededDatabase string
	clickhouseDSN, _ := setupClickhouseContainer(t, func(ctx context.Context, user, password, database, dsn string, container *clickhouse.ClickHouseContainer) error {
		seededDatabase = database
		return nil
	})

	// Deliberately never created: that is the whole point.
	schemaName := "fresh_database"
	testDSN := strings.Replace(clickhouseDSN, seededDatabase, schemaName, 1)

	stateFolder := t.TempDir()
	options := db_proto.SinkerFactoryOptions{
		UseProtoOption: true,
		Constraints:    protosql.DisableAllConstraints(),
		Clickhouse: db_proto.SinkerFactoryClickhouse{
			SinkInfoFolder: stateFolder,
			CursorFilePath: filepath.Join(stateFolder, "cursor.txt"),
		},
	}.Defaults()

	done := make(chan error, 1)
	go func() {
		_, err := db_proto.SetupDatabaseSchema(context.Background(), testDSN, schemaName,
			defaultOutputModuleName, outputMessageDescriptor, options, logger, tracer)
		done <- err
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(20 * time.Second):
		t.Fatal("setup never returned, the schema check is dialing a database that does not exist yet")
	}
}
