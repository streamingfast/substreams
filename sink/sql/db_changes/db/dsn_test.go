package db

import (
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDSN(t *testing.T) {
	tests := []struct {
		name             string
		dns              string
		expectError      bool
		expectConnString string
		expectSchema     string
		expectPassword   string
	}{
		{
			name:             "golden path",
			dns:              "psql://postgres:postgres@localhost/substreams-dev?enable_incremental_sort=off&sslmode=disable",
			expectConnString: "host=localhost port=5432 dbname=substreams-dev enable_incremental_sort=off sslmode=disable user=postgres password=postgres",
			expectSchema:     "public",
			expectPassword:   "postgres",
		},
		{
			name:             "with schemaName",
			dns:              "psql://postgres:postgres@localhost/substreams-dev?enable_incremental_sort=off&sslmode=disable&schemaName=foo",
			expectConnString: "host=localhost port=5432 dbname=substreams-dev enable_incremental_sort=off sslmode=disable user=postgres password=postgres",
			expectSchema:     "foo",
			expectPassword:   "postgres",
		},
		{
			name:             "with password",
			dns:              "clickhouse://default:password@host:9000/default",
			expectConnString: "clickhouse://default:password@host:9000/default",
			expectSchema:     "default",
			expectPassword:   "password",
		},
		{
			name:             "with blank password",
			dns:              "clickhouse://default:@host:9000/default",
			expectConnString: "clickhouse://default:@host:9000/default",
			expectSchema:     "default",
			expectPassword:   "",
		},
		{
			name:             "clickhouse with schemaName",
			dns:              "clickhouse://default:password@host:9000/default?schemaName=testdb",
			expectConnString: "clickhouse://default:password@host:9000/default",
			expectSchema:     "testdb",
			expectPassword:   "password",
		},
		{
			name:             "postgres password with reserved characters",
			dns:              "psql://postgres:p%40:%2F%3F%23%25%20'%5C@localhost/db",
			expectConnString: `host=localhost port=5432 dbname=db user=postgres password='p@:/?#% \'\\'`,
			expectSchema:     "public",
			expectPassword:   `p@:/?#% '\`,
		},
		{
			name:             "clickhouse password with reserved characters",
			dns:              "clickhouse://default:p%40:%2F%3F%23%25@host:9000/default",
			expectConnString: "clickhouse://default:p%40%3A%2F%3F%23%25@host:9000/default",
			expectSchema:     "default",
			expectPassword:   "p@:/?#%",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d, err := ParseDSN(test.dns)
			if test.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectConnString, d.ConnString())
				assert.Equal(t, test.expectSchema, d.schema)
				assert.Equal(t, test.expectPassword, d.Password)
			}
		})
	}

}

// Values reach PostgreSQL as given in the DSN, whatever characters they hold.
func TestDSNConnString_PostgresValues(t *testing.T) {
	rootCert := writeTestRootCert(t, t.TempDir())
	d, err := ParseDSN(`psql://user:p%40ss%20w'rd@localhost:5432/my%20db?sslmode=verify-ca&sslrootcert=` + url.QueryEscape(rootCert) + `&timezone=Europe/London&options=-c%20search_path%3Dx`)
	require.NoError(t, err)
	config, err := pgx.ParseConfig(d.ConnString())
	require.NoError(t, err, "the root certificate's path is read as given")
	assert.NotNil(t, config.TLSConfig.VerifyPeerCertificate)

	assert.Equal(t, "p@ss w'rd", config.Password)
	assert.Equal(t, "my db", config.Database)
	assert.Equal(t, "Europe/London", config.RuntimeParams["timezone"])
	assert.Equal(t, "-c search_path=x", config.RuntimeParams["options"])
}

func TestParseDSN_ErrorRedactsPassword(t *testing.T) {
	for _, dsn := range []string{
		"psql://user:s3cr/et@localhost:5432/db",
		"psql://user:s3cr#et@localhost:5432/db",
		"psql://user:s3cr%et@localhost:5432/db",
		"clickhouse://user:s3cr?et@localhost:9000/db",
	} {
		_, err := ParseDSN(dsn)
		require.Error(t, err, dsn)
		assert.NotContains(t, err.Error(), "s3cr", dsn)
		assert.Contains(t, err.Error(), "user:xxxxx@localhost", dsn)
	}
}
