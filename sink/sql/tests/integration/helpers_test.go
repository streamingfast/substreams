package tests

import (
	"context"
	"fmt"
	protosql "github.com/streamingfast/substreams/sink/sql/db_proto/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/moby/moby/api/types/network"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/logging/zapx"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/sink/sinktest"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

var logger *zap.Logger
var tracer logging.Tracer

// Short names for the shared fake stream helpers, see package sinktest
const defaultOutputModuleName = sinktest.DefaultOutputModuleName

type finalBlock = sinktest.FinalBlock

var (
	setupFakeSubstreamsServer = sinktest.SetupFakeSubstreamsServer
	streamMock                = sinktest.StreamMock
	blockScopedData           = sinktest.BlockScopedData
	blockUndo                 = sinktest.BlockUndo
	blockTime                 = sinktest.BlockTime
	blockTimepb               = sinktest.BlockTimepb
	fixedBaseTime             = sinktest.FixedBaseTime
)

func init() {
	logger, tracer = logging.ApplicationLogger("test", "test", logging.WithDefaultLevel(zap.ErrorLevel))
}

// SchemaName represents a database schema name with both escaped and unescaped versions
type SchemaName struct {
	Unescaped string // The original unescaped schema name
	Escaped   string // The database-escaped version (ClickHouse uses backticks, PostgreSQL uses quotes)
}

// NewSchemaName creates a new SchemaName with automatic escaping for ClickHouse
func NewSchemaName(unescaped string) SchemaName {
	return SchemaName{
		Unescaped: unescaped,
		Escaped:   "`" + unescaped + "`", // ClickHouse escaping
	}
}

// NewPostgresSchemaName creates a new SchemaName with automatic escaping for PostgreSQL
func NewPostgresSchemaName(unescaped string) SchemaName {
	return SchemaName{
		Unescaped: unescaped,
		Escaped:   `"` + unescaped + `"`, // PostgreSQL escaping
	}
}

// NewRandomSchemaName creates a new SchemaName with a random unescaped name (ClickHouse escaping)
func NewRandomSchemaName() SchemaName {
	return NewSchemaName(randomSchemaName())
}

// NewRandomPostgresSchemaName creates a new SchemaName with a random unescaped name (PostgreSQL escaping)
func NewRandomPostgresSchemaName() SchemaName {
	return NewPostgresSchemaName(randomSchemaName())
}

// String returns the escaped version for default string representation
func (s SchemaName) String() string {
	return s.Escaped
}

type PostgresSeeder = func(ctx context.Context, user, password, database, schema, dsn string, container *postgres.PostgresContainer) error

type PostgresContainerConfig struct {
	Image string
}

// setupRawPostgresContainer spins up a Postgres Docker container and let a seeder function seed the database.
func setupRawPostgresContainer(config PostgresContainerConfig) (*PostgresContainerExt, func()) {
	ctx := context.Background()

	dbName := "users"
	dbUser := "user"
	dbPassword := "password"

	// Wait by connecting, not by reading the log, and mirror the ClickHouse strategy
	// below. The log line this used to watch for is printed twice — once by the
	// temporary server initdb runs over a Unix socket, once by the real one — so
	// matching the second occurrence is a proxy for "listening on TCP" rather than
	// proof of it. Running a query is the proof. The old 5s startup budget was also
	// well inside what a cold CI runner takes to start Postgres, and a timeout here
	// panics the whole package rather than failing one test.
	postgresContainer, err := postgres.Run(ctx,
		config.Image,
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForSQL("5432/tcp", "postgres", func(host string, port network.Port) string {
				return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPassword, host, port.Port(), dbName)
			}).WithStartupTimeout(60*time.Second).WithQuery("SELECT 1"),
		),
	)
	if err != nil {
		panic(fmt.Errorf("setting up postgres container: %w", err))
	}

	return &PostgresContainerExt{
			PostgresContainer: postgresContainer,
			Configuration:     &config,
			ConnectionString:  postgresContainer.MustConnectionString(ctx, "sslmode=disable"),
		}, func() {
			_ = testcontainers.TerminateContainer(postgresContainer)
		}
}

type PostgresContainerExt struct {
	*postgres.PostgresContainer
	Configuration *PostgresContainerConfig
	// ConnectionString should be used instead of calling ConnectionString() on the embedded PostgresContainer
	// because this one is properly configured with sslmode=disable.
	ConnectionString string
}

type ClickhouseContainerConfig struct {
	Image string
}

type ClickhouseContainerExt struct {
	*clickhouse.ClickHouseContainer
	Configuration    *ClickhouseContainerConfig
	ConnectionString string
}

type ClickhouseSeeder = func(ctx context.Context, user, password, database, dsn string, container *clickhouse.ClickHouseContainer) error

// setupRawClickhouseContainer spins up a ClickHouse Docker container
func setupRawClickhouseContainer(config ClickhouseContainerConfig) (*ClickhouseContainerExt, func()) {
	ctx := context.Background()
	dbName := "default"
	dbUser := "default"
	dbPassword := "clickhouse"

	// Use SQL-based wait strategy because ClickHouse logs to file, not stdout
	clickhouseContainer, err := clickhouse.Run(ctx,
		config.Image,
		clickhouse.WithDatabase(dbName),
		clickhouse.WithUsername(dbUser),
		clickhouse.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForSQL("9000/tcp", "clickhouse", func(host string, port network.Port) string {
				return fmt.Sprintf("clickhouse://%s:%s@%s:%s/%s", dbUser, dbPassword, host, port.Port(), dbName)
			}).WithStartupTimeout(30*time.Second).WithQuery("SELECT 1"),
		),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to start ClickHouse container: %s", err))
	}

	// Get the mapped port for the native ClickHouse protocol (9000)
	mappedPort, err := clickhouseContainer.MappedPort(ctx, "9000")
	if err != nil {
		panic(fmt.Sprintf("failed to get ClickHouse native port: %s", err))
	}

	host, err := clickhouseContainer.Host(ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to get ClickHouse host: %s", err))
	}

	connectionString := fmt.Sprintf("clickhouse://%s:%s@%s:%s/%s", dbUser, dbPassword, host, mappedPort.Port(), dbName)

	return &ClickhouseContainerExt{
			ClickHouseContainer: clickhouseContainer,
			Configuration:       &config,
			ConnectionString:    connectionString,
		}, func() {
			_ = testcontainers.TerminateContainer(clickhouseContainer)
		}
}

// setupClickhouseContainer spins up a ClickHouse Docker container and let a seeder function seed the database.
func setupClickhouseContainer(t *testing.T, seedDb ClickhouseSeeder) (dbConnectionString string, container *clickhouse.ClickHouseContainer) {
	t.Helper()

	start := time.Now()
	defer func() {
		logger.Debug("setupClickhouseContainer duration", zapx.HumanDuration("duration", time.Since(start)))
	}()

	ctx := context.Background()

	dbName := "test_schema"
	dbUser := "default"
	dbPassword := "clickhouse"

	// Use SQL-based wait strategy because ClickHouse logs to file, not stdout
	clickhouseContainer, err := clickhouse.Run(ctx,
		"clickhouse/clickhouse-server:26.1-alpine",
		clickhouse.WithDatabase(dbName),
		clickhouse.WithUsername(dbUser),
		clickhouse.WithPassword(dbPassword),
		testcontainers.WithWaitStrategy(
			wait.ForSQL("9000/tcp", "clickhouse", func(host string, port network.Port) string {
				return fmt.Sprintf("clickhouse://%s:%s@%s:%s/%s", dbUser, dbPassword, host, port.Port(), dbName)
			}).WithStartupTimeout(30*time.Second).WithQuery("SELECT 1"),
		),
	)
	require.NoError(t, err)

	dbConnectionString, err = clickhouseContainer.ConnectionString(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		if os.Getenv("DEBUG_CLICKHOUSE_TEST") != "" {
			containerName, err := clickhouseContainer.Name(ctx)
			require.NoError(t, err)

			fmt.Println()
			fmt.Println("ClickHouse container started with connection string:", dbConnectionString)
			fmt.Println("You can connect to it using:")
			fmt.Printf("  docker exec -it %s clickhouse-client -u %s\n", containerName, dbUser)

			timeout, err := time.ParseDuration(os.Getenv("DEBUG_CLICKHOUSE_TEST"))
			require.NoError(t, err)
			fmt.Println("Waiting for", timeout, "before cleaning up container...")

			time.Sleep(timeout)
		}

		testcontainers.TerminateContainer(clickhouseContainer, testcontainers.StopTimeout(0*time.Second))
	})

	require.NoError(t, seedDb(ctx, dbUser, dbPassword, dbName, dbConnectionString, clickhouseContainer))

	return dbConnectionString, clickhouseContainer
}

// substreamsTestPackage creates a test package with the given output module name and file descriptor.
//
// File descriptor can usually be obtained from a generate Golang proto file and the field to look for
// look like:
//
//	pbrelations.File_test_relations_relations_proto
func substreamsTestPackage(fileDescriptor protoreflect.FileDescriptor, outputDesc protoreflect.MessageDescriptor) *pbsubstreams.Package {
	// Create dummy base sink (we won't actually use it for streaming)
	fileDescriptorPb := protodesc.ToFileDescriptorProto(fileDescriptor)
	outputType := string(outputDesc.FullName())

	return &pbsubstreams.Package{
		ProtoFiles: []*descriptorpb.FileDescriptorProto{
			fileDescriptorPb,
		},
		Modules: &pbsubstreams.Modules{
			Modules: []*pbsubstreams.Module{
				{
					Name: defaultOutputModuleName,
					Output: &pbsubstreams.Module_Output{
						Type: outputType,
					},
					Kind: &pbsubstreams.Module_KindMap_{
						KindMap: &pbsubstreams.Module_KindMap{
							OutputType: outputType,
						},
					}},
			},
		},
	}
}

type isAlwaysLiveChecker struct{}

func (c *isAlwaysLiveChecker) IsLive(block *pbsubstreams.Clock) bool {
	return true
}

func readRowsBy[T any](t *testing.T, db *sqlx.DB, tableAndOrSchema, orderBy string) []*T {
	t.Helper()

	tableIdentifier := strings.TrimSpace(tableAndOrSchema)
	if !strings.HasPrefix(tableIdentifier, `"`) {
		tableIdentifier = `"` + tableIdentifier
	}

	if !strings.HasSuffix(tableIdentifier, `"`) {
		tableIdentifier += `"`
	}

	var rows []*T
	err := db.SelectContext(context.Background(), &rows, fmt.Sprintf(`SELECT * FROM %s ORDER BY %s;`, tableIdentifier, orderBy))
	require.NoError(t, err)

	return rows
}

func readDbChangesRows[T any](t *testing.T, db *sqlx.DB, schema string, table string) []*T {
	t.Helper()

	return readRowsBy[T](t, db, fmt.Sprintf(`"%s"."%s"`, schema, table), "id")
}

// constraintPolicy maps a test's "with constraints" boolean onto a policy: created before
// the load when asked for, none at all otherwise. The sink's own default — everything,
// created once the backfill is done — is exercised separately.
func constraintPolicy(useConstraints bool) protosql.ConstraintPolicy {
	if !useConstraints {
		return protosql.DisableAllConstraints()
	}

	return protosql.ConstraintPolicy{Timing: protosql.ConstraintsAlways}
}
