package db

import (
	"fmt"
	"iter"
	"maps"
	"net/url"
	"os"
	"slices"
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

	// The url.Parse error is not wrapped: it quotes the input, password included.
	dsnURL, err := url.Parse(expanded)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN %q: not a valid URL, percent-encode reserved characters in the user name and password (e.g. '/' as %%2F, '?' as %%3F, '#' as %%23, '%%' as %%25)", redactDSN(dsn))
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

		u := url.URL{
			Scheme:   scheme,
			User:     url.UserPassword(c.Username, c.Password),
			Host:     fmt.Sprintf("%s:%d", host, c.Port),
			Path:     "/" + c.Database,
			RawQuery: c.Options.Encode(),
		}

		return u.String()
	}
	// PostgreSQL's keyword/value form: values are quoted where needed, not URL encoded, which it
	// would not decode.
	database, err := url.PathUnescape(c.Database)
	if err != nil {
		database = c.Database
	}
	parts := []string{"host=" + keywordValue(c.Host), fmt.Sprintf("port=%d", c.Port), "dbname=" + keywordValue(database)}
	for _, key := range slices.Sorted(maps.Keys(c.Options)) {
		parts = append(parts, key+"="+keywordValue(c.Options.Get(key)))
	}
	if c.Username != "" {
		parts = append(parts, "user="+keywordValue(c.Username))
	}
	if c.Password != "" {
		parts = append(parts, "password="+keywordValue(c.Password))
	}
	return strings.Join(parts, " ")
}

// keywordValue writes value for a keyword/value connection string, quoted when it is empty or
// holds whitespace, a quote or a backslash.
func keywordValue(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n\r\f\v'\\") {
		return value
	}
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value) + "'"
}

// redactDSN masks the password of a DSN that may not parse as a URL. It cuts at
// the last '@', so it masks too much rather than too little when the query
// also contains one.
func redactDSN(dsn string) string {
	schemeEnd := strings.Index(dsn, "://")
	if schemeEnd < 0 {
		return "<redacted>"
	}
	rest := dsn[schemeEnd+3:]
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return dsn
	}
	userinfo := rest[:at]
	if colon := strings.Index(userinfo, ":"); colon >= 0 {
		userinfo = userinfo[:colon] + ":xxxxx"
	}
	return dsn[:schemeEnd+3] + userinfo + rest[at:]
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
