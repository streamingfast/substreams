package db

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
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

// postgresTestEnv clears what the environment could otherwise change in a connection's config.
func postgresTestEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"PGSSLMODE", "PGSERVICE", "PGSERVICEFILE", "PGAPPNAME", "PGTZ", "PGHOSTADDR", "SSLPGMINPROTOCOLVERSION", "SSLPGMAXPROTOCOLVERSION", "PGLOADBALANCEHOSTS"} {
		t.Setenv(name, "")
	}
}

func postgresTestConfig(t *testing.T, options string) (*pgx.ConnConfig, error) {
	t.Helper()
	parsed, err := ParseDSN("psql://user:pass@db.example:5432/db?" + options)
	require.NoError(t, err)
	return PostgresConnConfig(parsed)
}

func mustPostgresTestConfig(t *testing.T, options string) *pgx.ConnConfig {
	t.Helper()
	config, err := postgresTestConfig(t, options)
	require.NoError(t, err)
	return config
}

func TestPostgresConnConfig(t *testing.T) {
	postgresTestEnv(t)
	required := mustPostgresTestConfig(t, "")
	assert.NotNil(t, required.TLSConfig, "TLS by default")
	assert.Empty(t, required.Fallbacks, "no fallback to plain")
	assert.Nil(t, mustPostgresTestConfig(t, "sslmode=disable").TLSConfig)
}

func TestPostgresConnConfig_Service(t *testing.T) {
	postgresTestEnv(t)
	file := filepath.Join(t.TempDir(), "services.conf")
	require.NoError(t, os.WriteFile(file, []byte("[unset]\nport=5432\n[off]\nsslmode=disable\n[prefer]\nsslmode=prefer\n"), 0o600))
	t.Setenv("PGSERVICEFILE", file)
	config := func(options string) *pgx.ConnConfig { return mustPostgresTestConfig(t, options) }

	unset := config("service=unset")
	assert.NotNil(t, unset.TLSConfig, "a service without sslmode requires TLS")
	assert.Empty(t, unset.Fallbacks, "and doesn't fall back to plain")
	assert.Nil(t, config("service=off").TLSConfig, "a service can turn TLS off")
	assert.Empty(t, config("service=prefer").Fallbacks, "falling back to plain takes the DSN or PGSSLMODE")
	assert.Len(t, config("service=unset&sslmode=prefer").Fallbacks, 1)

	t.Setenv("PGSSLMODE", "prefer")
	assert.Len(t, config("service=unset").Fallbacks, 1)
	t.Setenv("PGSSLMODE", "")

	_, err := postgresTestConfig(t, "service=missing")
	assert.Error(t, err, "a missing service stays an error")

	t.Setenv("PGSERVICE", "unset")
	assert.Empty(t, config("").Fallbacks, "same for a service named by PGSERVICE")
}

// A service with a root certificate and no sslmode verifies the server, as sslmode=require does.
func TestPostgresConnConfig_ServiceRootCert(t *testing.T) {
	postgresTestEnv(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	dir := t.TempDir()
	rootCert := filepath.Join(dir, "root.crt")
	require.NoError(t, os.WriteFile(rootCert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	file := filepath.Join(dir, "services.conf")
	require.NoError(t, os.WriteFile(file, []byte("[verified]\nsslrootcert="+rootCert+"\n"), 0o600))
	t.Setenv("PGSERVICEFILE", file)

	config := mustPostgresTestConfig(t, "service=verified")
	assert.Empty(t, config.Fallbacks)
	assert.NotNil(t, config.TLSConfig.VerifyPeerCertificate, "the server's certificate is checked")
}

func TestPostgresFallsBackToPlain(t *testing.T) {
	postgresTestEnv(t)
	for connString, expected := range map[string]bool{
		"host=a":                 true,
		"host=a,b":               true,
		"host=/tmp,a":            true,
		"host=a,/tmp":            true,
		"host=/tmp":              false,
		"host=a sslmode=require": false,
		"host=a sslmode=disable": false,
		"host=a,b sslmode=allow": false,
	} {
		config, err := pgx.ParseConfig(connString)
		require.NoError(t, err, connString)
		assert.Equal(t, expected, postgresFallsBackToPlain(config), connString)
	}
}

func TestPostgresConnConfig_LibpqOptions(t *testing.T) {
	postgresTestEnv(t)
	config := func(options string) (*pgx.ConnConfig, error) { return postgresTestConfig(t, options) }

	c, err := config("fallback_application_name=sink&tz=UTC&binary_parameters=yes&disable_prepared_binary_result=yes")
	require.NoError(t, err)
	assert.Equal(t, "sink", c.RuntimeParams["application_name"])
	for _, option := range []string{"fallback_application_name", "tz", "binary_parameters", "disable_prepared_binary_result"} {
		assert.NotContains(t, c.RuntimeParams, option, "the server would refuse it")
	}

	c, err = config("fallback_application_name=sink&application_name=mine")
	require.NoError(t, err)
	assert.Equal(t, "mine", c.RuntimeParams["application_name"])

	c, err = config("ssl_min_protocol_version=TLSv1.3&ssl_max_protocol_version=TLSv1.3")
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS13), c.TLSConfig.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), c.TLSConfig.MaxVersion)

	c, err = config("hostaddr=127.0.0.1&sslmode=verify-full")
	require.NoError(t, err)
	assert.Equal(t, "db.example", c.TLSConfig.ServerName, "TLS still checks the host name")
	assert.False(t, c.TLSConfig.InsecureSkipVerify)
	addrs, err := c.LookupFunc(context.Background(), "db.example")
	require.NoError(t, err)
	assert.Equal(t, []string{"127.0.0.1"}, addrs)

	for _, refused := range []string{"hostaddr=127.0.0.1,127.0.0.2", "ssl_min_protocol_version=TLSv9", "sslinline=true", "load_balance_hosts=random"} {
		_, err := config(refused)
		assert.Error(t, err, refused)
	}
	for _, off := range []string{"false", "False", "0", "f", "no", "off"} {
		_, err = config("load_balance_hosts=disable&sslinline=" + off)
		assert.NoError(t, err, off)
	}

	c, err = config("geqo=invalid")
	require.NoError(t, err)
	assert.NotContains(t, c.RuntimeParams, "geqo", "lib/pq never used it")

	// A service setting application_name empty keeps it empty.
	file := filepath.Join(t.TempDir(), "services.conf")
	require.NoError(t, os.WriteFile(file, []byte("[empty]\napplication_name=\n"), 0o600))
	t.Setenv("PGSERVICEFILE", file)
	c, err = config("service=empty&fallback_application_name=sink")
	require.NoError(t, err)
	assert.Equal(t, "", c.RuntimeParams["application_name"])
}

func TestPostgresConnConfig_LibpqEnv(t *testing.T) {
	postgresTestEnv(t)
	config := func(options string) (*pgx.ConnConfig, error) { return postgresTestConfig(t, options) }

	t.Setenv("SSLPGMINPROTOCOLVERSION", "TLSv1.3")
	t.Setenv("SSLPGMAXPROTOCOLVERSION", "TLSv1.3")
	c, err := config("")
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS13), c.TLSConfig.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), c.TLSConfig.MaxVersion)
	c, err = config("ssl_min_protocol_version=TLSv1.2")
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS12), c.TLSConfig.MinVersion, "the DSN comes first")

	t.Setenv("PGHOSTADDR", "127.0.0.1")
	c, err = config("")
	require.NoError(t, err)
	addrs, err := c.LookupFunc(context.Background(), "db.example")
	require.NoError(t, err)
	assert.Equal(t, []string{"127.0.0.1"}, addrs)
	t.Setenv("PGHOSTADDR", "")

	t.Setenv("PGLOADBALANCEHOSTS", "random")
	_, err = config("")
	assert.Error(t, err)
	t.Setenv("PGLOADBALANCEHOSTS", "")

	t.Setenv("PGTZ", "Invalid/Zone")
	c, err = config("")
	require.NoError(t, err)
	assert.NotContains(t, c.RuntimeParams, "timezone", "lib/pq left it to the server")
	t.Setenv("PGTZ", "UTC")
	c, err = config("timezone=UTC")
	require.NoError(t, err)
	assert.Equal(t, "UTC", c.RuntimeParams["timezone"], "asked for in the DSN")
}

// hostaddr only makes sense for a single host reached over the network.
func TestPostgresConnConfig_HostaddrHosts(t *testing.T) {
	postgresTestEnv(t)
	config := &pgx.ConnConfig{}
	parse := func(connString string) {
		var err error
		config, err = pgx.ParseConfig(connString)
		require.NoError(t, err)
	}

	parse("host=/tmp hostaddr=127.0.0.1")
	assert.ErrorContains(t, applyLibpqOptions(&DSN{}, config), "socket")
	parse("host=a,b hostaddr=127.0.0.1")
	assert.ErrorContains(t, applyLibpqOptions(&DSN{}, config), "several hosts")
	parse("host=a hostaddr=127.0.0.1")
	assert.NoError(t, applyLibpqOptions(&DSN{}, config), "prefer's two tries of one host are fine")
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
