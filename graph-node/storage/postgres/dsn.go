package postgres

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/drone/envsubst"
)

func ParseDSN(dsn string) (*DSN, error) {
	return parseDSN(dsn, os.Getenv)
}

func parseDSN(dsn string, mapper func(s string) string) (*DSN, error) {
	expanded, err := envsubst.Eval(dsn, mapper)
	if err != nil {
		return nil, fmt.Errorf("variables expansion failed: %w", err)
	}

	dsnURL, err := url.Parse(expanded)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if dsnURL.Scheme != "postgresql" {
		return nil, fmt.Errorf(`invalid scheme %q, should be "postgresql"`, dsnURL.Scheme)
	}

	host := dsnURL.Hostname()

	port := int64(5432)
	if strings.Contains(dsnURL.Host, ":") {
		port, _ = strconv.ParseInt(dsnURL.Port(), 10, 32)
	}

	username := dsnURL.User.Username()
	password, _ := dsnURL.User.Password()
	database := strings.TrimPrefix(dsnURL.EscapedPath(), "/")

	query := dsnURL.Query()
	keys := make([]string, 0, len(query))
	for key := range dsnURL.Query() {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	options := make([]string, len(query))
	for i, key := range keys {
		options[i] = fmt.Sprintf("%s=%s", key, strings.Join(query[key], ","))
	}

	return &DSN{dsn, host, port, database, username, password, options}, nil
}

type DSN struct {
	original string

	host     string
	port     int64
	database string
	username string
	password string
	options  []string
}

func (c *DSN) DSN() string {
	out := fmt.Sprintf("host=%s port=%d user=%s dbname=%s %s", c.host, c.port, c.username, c.database, strings.Join(c.options, " "))
	if c.password != "" {
		out = out + " password=" + c.password
	}
	return out
}

func (c *DSN) String() string {
	return c.original
}
