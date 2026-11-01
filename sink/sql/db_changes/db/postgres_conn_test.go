package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Values of a column are written according to its scan type: quoted for
// text, parsed for a timestamp, as the module gives them for most other types.
func TestPostgresScanType(t *testing.T) {
	dsnString := pgTestSchema(t)
	dsn, err := ParseDSN(dsnString)
	require.NoError(t, err)
	raw, err := sql.Open("pgx", dsn.ConnString())
	require.NoError(t, err)
	defer raw.Close()

	anyType := reflect.TypeFor[any]()
	expected := map[string]reflect.Type{
		"1::smallint": reflect.TypeFor[int16](), "1::int": reflect.TypeFor[int32](), "1::bigint": reflect.TypeFor[int64](),
		"1.5::real": reflect.TypeFor[float32](), "1.5::float8": reflect.TypeFor[float64](),
		"'t'::text": reflect.TypeFor[string](), "'v'::varchar(9)": reflect.TypeFor[string](), "B'1'::bit(1)": reflect.TypeFor[string](),
		"true": reflect.TypeFor[bool](), `'\x01'::bytea`: reflect.TypeFor[[]byte](),
		"now()::timestamp": reflectTypeTime, "now()": reflectTypeTime, "now()::date": reflectTypeTime, "now()::time": reflectTypeTime, "now()::timetz": reflectTypeTime,
		"1.5::numeric": anyType, "1.5::money": anyType, "'c'::char(3)": anyType, "'1 day'::interval": anyType, "'{}'::json": anyType, "'{}'::jsonb": anyType,
		"gen_random_uuid()": anyType, "'{1}'::int[]": anyType, "'{a}'::text[]": anyType, "'127.0.0.1'::inet": anyType, "'a'::name": anyType, "1::oid": anyType,
		"B'11'::varbit": reflect.TypeFor[string](), "'a'::tsvector": anyType, "'<a/>'::xml": anyType, "'[1,2)'::int4range": anyType, "'pg_class'::regclass": anyType,
		"'x'::mood": anyType, "ROW(1,'a')::pair": anyType, "'a'::label": reflect.TypeFor[string](), "1::amount": reflect.TypeFor[int32](),
	}
	_, err = raw.Exec(`DROP TYPE IF EXISTS mood; CREATE TYPE mood AS ENUM ('x'); DROP TYPE IF EXISTS pair; CREATE TYPE pair AS (a int, b text);
		DROP DOMAIN IF EXISTS label; CREATE DOMAIN label AS text; DROP DOMAIN IF EXISTS amount; CREATE DOMAIN amount AS int`)
	require.NoError(t, err)
	for expression, scanType := range expected {
		rows, err := raw.Query("SELECT " + expression)
		require.NoError(t, err, expression)
		columns, err := rows.ColumnTypes()
		require.NoError(t, err, expression)
		assert.Equal(t, scanType, postgresScanType(columns[0]), expression)
		rows.Close()
	}
}

func TestPostgresConnConfig(t *testing.T) {
	t.Setenv("PGSSLMODE", "")
	config := func(dsn string) *pgx.ConnConfig {
		parsed, err := ParseDSN(dsn)
		require.NoError(t, err)
		config, err := postgresConnConfig(parsed)
		require.NoError(t, err)
		return config
	}

	required := config("psql://user:pass@db.example:5432/db")
	assert.NotNil(t, required.TLSConfig, "TLS by default")
	assert.Empty(t, required.Fallbacks, "no fallback to plain")
	assert.Nil(t, config("psql://user:pass@db.example:5432/db?sslmode=disable").TLSConfig)
}

// A session reads statements the same whatever the role's defaults, and old DSN options still work.
func TestPostgresConnConfig_Session(t *testing.T) {
	dsnString := pgTestSchema(t)
	dsn, err := ParseDSN(dsnString)
	require.NoError(t, err)
	admin, err := sql.Open("pgx", dsn.ConnString())
	require.NoError(t, err)
	defer func() { _ = admin.Close() }()

	role := fmt.Sprintf("session_test_%d", time.Now().UnixNano())
	_, err = admin.Exec(fmt.Sprintf(`CREATE ROLE %[1]s LOGIN PASSWORD 'secret';
		ALTER ROLE %[1]s SET datestyle = 'ISO, DMY';
		ALTER ROLE %[1]s SET standard_conforming_strings = off;
		ALTER ROLE %[1]s SET client_encoding = 'LATIN1'`, role))
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec("DROP ROLE " + role) })

	u, err := url.Parse(dsnString)
	require.NoError(t, err)
	u.User = url.UserPassword(role, "secret")
	q := u.Query()
	q.Set("binary_parameters", "yes")
	u.RawQuery = q.Encode()
	roleDSN, err := ParseDSN(u.String())
	require.NoError(t, err)
	db, err := OpenDB(roleDSN)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var date, encoding, conforming string
	require.NoError(t, db.QueryRow(`SELECT '01/02/2026'::date::text, current_setting('client_encoding'), current_setting('standard_conforming_strings')`).Scan(&date, &encoding, &conforming))
	assert.Equal(t, "2026-01-02", date)
	assert.Equal(t, "UTF8", encoding)
	assert.Equal(t, "on", conforming)
}
