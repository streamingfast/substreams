// Package params reads the query parameters of an emitter URL, tracking which ones were consumed so the
// emitter can reject typos or forward the rest.
package params

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// Query reads typed values out of a URL query.
type Query struct {
	values url.Values
	used   map[string]bool
	err    error
}

// New wraps the query of u.
func New(u *url.URL) *Query {
	return &Query{values: u.Query(), used: map[string]bool{}}
}

// String returns the value of name or def when absent.
func (q *Query) String(name, def string) string {
	q.used[name] = true
	if v := q.values.Get(name); v != "" {
		return v
	}
	return def
}

// Int returns the integer value of name or def when absent. A malformed value is remembered, see [Query.Err].
func (q *Query) Int(name string, def int) int {
	q.used[name] = true
	raw := q.values.Get(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		q.fail(name, raw, err)
		return def
	}
	return v
}

// Duration returns the duration value of name or def when absent.
func (q *Query) Duration(name string, def time.Duration) time.Duration {
	q.used[name] = true
	raw := q.values.Get(name)
	if raw == "" {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		q.fail(name, raw, err)
		return def
	}
	return v
}

// Bool returns the boolean value of name or def when absent.
func (q *Query) Bool(name string, def bool) bool {
	q.used[name] = true
	raw := q.values.Get(name)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		q.fail(name, raw, err)
		return def
	}
	return v
}

// WithPrefix returns the values of the parameters named `<prefix><x>` keyed by `x`, marking them used.
func (q *Query) WithPrefix(prefix string) map[string]string {
	out := map[string]string{}
	for name, values := range q.values {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix && len(values) > 0 {
			q.used[name] = true
			out[name[len(prefix):]] = values[0]
		}
	}
	return out
}

// Unused returns the parameters that were never read, as a query.
func (q *Query) Unused() url.Values {
	out := url.Values{}
	for name, values := range q.values {
		if !q.used[name] {
			out[name] = values
		}
	}
	return out
}

// Err returns the first malformed value met, if any.
func (q *Query) Err() error { return q.err }

func (q *Query) fail(name, raw string, err error) {
	if q.err == nil {
		q.err = fmt.Errorf("invalid value %q for parameter %q: %w", raw, name, err)
	}
}
