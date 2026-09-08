package db

import (
	"fmt"
	"iter"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/drone/envsubst"
)

type DSN struct {
	driver   string
	original string
	scheme   string

	Host     string
	Port     int64
	Username string
	Password string
	Database string
	Options  DSNOptions

	// schema is the extracted schema from the DSN schemaName option (if present)
	schema string
}

var driverMap = map[string]string{
	"psql":       "postgres",
	"postgres":   "postgres",
	"clickhouse": "clickhouse",
	"parquet":    "parquet",
}

func ParseDSN(dsn string) (*DSN, error) {
	expanded, err := envsubst.Eval(dsn, os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("variables expansion failed: %w", err)
	}

	dsnURL, err := url.Parse(expanded)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	driver, ok := driverMap[dsnURL.Scheme]
	if !ok {
		keys := make([]string, len(driverMap))
		i := 0
		for k := range driverMap {
			keys[i] = k
			i++
		}

		return nil, fmt.Errorf("invalid scheme %s, allowed schemes: [%s]", dsnURL.Scheme, strings.Join(keys, ","))
	}

	host := dsnURL.Hostname()

	port := int64(5432)
	if strings.Contains(dsnURL.Host, ":") {
		port, _ = strconv.ParseInt(dsnURL.Port(), 10, 32)
	}

	username := dsnURL.User.Username()
	password, _ := dsnURL.User.Password()
	database := dsnURL.EscapedPath()
	if database != "parquet" {
		database = strings.TrimPrefix(database, "/")
	}

	d := &DSN{
		original: dsn,
		driver:   driver,
		scheme:   dsnURL.Scheme,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Database: database,
		Options:  DSNOptions(dsnURL.Query()),
	}

	schemaName := d.Options.RemoveOr("schemaName", "")

	if driver == "clickhouse" {
		// For ClickHouse, store the target database name in schema, but keep
		// connecting to the original database to allow CREATE DATABASE commands
		if schemaName != "" {
			d.schema = schemaName
		} else {
			d.schema = database
		}
	} else {
		if schemaName == "" {
			schemaName = "public"
		}

		// For other databases (PostgreSQL), schemaName is separate from database
		d.schema = schemaName
	}

	return d, nil
}

func (c *DSN) Driver() string {
	return c.driver
}

func (c *DSN) ConnString() string {
	if c.driver == "clickhouse" {
		scheme := c.driver
		host := c.Host

		baseURL := fmt.Sprintf("%s://%s:%s@%s:%d/%s", scheme, c.Username, c.Password, host, c.Port, c.Database)
		if len(c.Options) > 0 {
			baseURL += "?" + c.Options.Encode()
		}

		return baseURL
	}
	// PostgreSQL connection string uses space-separated options
	options := c.Options.EncodeWithSeparator(" ")
	out := fmt.Sprintf("host=%s port=%d dbname=%s %s", c.Host, c.Port, c.Database, options)
	if c.Username != "" {
		out = out + " user=" + c.Username
	}
	if c.Password != "" {
		out = out + " password=" + c.Password
	}
	return out
}

func (c *DSN) Schema() string {
	return c.schema
}

func (c *DSN) Clone() *DSN {
	return &DSN{
		driver:   c.driver,
		original: c.original,
		scheme:   c.scheme,
		Host:     c.Host,
		Port:     c.Port,
		Username: c.Username,
		Password: c.Password,
		Database: c.Database,
		Options:  c.Options,
		schema:   c.schema,
	}
}

// DSNOptions is a thin wrapper around url.Values to provide helper methods and
// better names.
type DSNOptions url.Values

// Iterate over the first value of each key, to be used in for range loops.
func (v DSNOptions) Iter() iter.Seq2[string, string] {
	return func(yield func(k string, v string) bool) {
		for k, vs := range v {
			if len(vs) > 0 {
				if !yield(k, vs[0]) {
					return
				}
			}
		}
	}
}

// Encode encodes the values into “URL encoded” form ("bar=baz&foo=quux") sorted by key.
func (v DSNOptions) Encode() string {
	return (url.Values(v)).Encode()
}

// EncodeWithSeparator encodes the values into “URL encoded” like form ("bar=baz foo=quux") sorted by key
// where essentially the separator is used instead of '&'.
func (v DSNOptions) EncodeWithSeparator(sep string) string {
	return strings.ReplaceAll((url.Values(v)).Encode(), "&", sep)
}

// Get returns the value associated with the key.
func (v DSNOptions) Get(key string) string {
	return (url.Values(v)).Get(key)
}

// GetOr returns the value associated with the key or defaultValue if not found.
func (v DSNOptions) GetOr(key, defaultValue string) string {
	if val := (url.Values(v)).Get(key); val != "" {
		return val
	}

	return defaultValue
}

// RemoveOr removes the key from the options and returns its value or defaultValue if not found.
func (v DSNOptions) RemoveOr(key, defaultValue string) string {
	val := (url.Values(v)).Get(key)
	(url.Values(v)).Del(key)
	if val != "" {
		return val
	}
	return defaultValue
}
