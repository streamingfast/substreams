package rpc

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRpcCacheNewLoadSave(t *testing.T) {
	dir, err := ioutil.TempDir("", "substreams_test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	writeStore, err := dstore.NewStore(dir, "json.zst", "zstd", true)
	require.NoError(t, err)

	ctx := context.Background()
	wc := NewCache(nil, writeStore, 1000, 11000)

	wc.kv = map[CacheKey][]byte{
		"foo": []byte(`bar`),
		"bar": []byte(`foo`),
	}
	wc.Save(ctx)

	readStore, err := dstore.NewStore(dir, "json.zst", "zstd", false)
	require.NoError(t, err)

	rc := NewCache(readStore, nil, 1000, 11000)
	rc.Load(ctx)

	assert.Equal(t, map[CacheKey][]byte{
		"foo": []byte(`bar`),
		"bar": []byte(`foo`),
	}, rc.kv)
}

func TestRpcCacheKey(t *testing.T) {
	c := &Cache{}
	k := c.Key("testtype", 234, "something", "blah:\nblah")
	assert.Equal(t, k, CacheKey("testtype:234:something:blah:\nblah"))
}

func TestRpcCacheSet(t *testing.T) {
	var cases = []struct {
		name   string
		in     interface{}
		expect []byte
	}{
		{
			name:   "simple entity",
			in:     &fakeEntity{"a", 2, "c"},
			expect: []byte(`{"A":"a","B":2,"C":"c"}`),
		},
		{
			name: "slice of entities",
			in: []*fakeEntity{
				{"aa", 22, "cc"},
				{"aaa", 222, "ccc"},
			},
			expect: []byte(`[{"A":"aa","B":22,"C":"cc"},{"A":"aaa","B":222,"C":"ccc"}]`),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cache := NewCache(nil, nil, 0, 0)
			cache.Set("foo", c.in)
			assert.Equal(t, c.expect, cache.kv["foo"])
		})
	}
}

func TestRpcCacheGet(t *testing.T) {
	var cases = []struct {
		name      string
		kv        map[CacheKey][]byte
		expect    interface{}
		intoSlice bool
		expectOK  bool
	}{
		{
			name:     "simple entity",
			kv:       map[CacheKey][]byte{"foo": []byte(`{"A":"a","B":2,"C":"c"}`)},
			expect:   &fakeEntity{"a", 2, "c"},
			expectOK: true,
		},
		{
			name:      "slice of entities",
			kv:        map[CacheKey][]byte{"foo": []byte(`[{"A":"aa","B":22,"C":"cc"},{"A":"aaa","B":222,"C":"ccc"}]`)},
			intoSlice: true,
			expect:    []*fakeEntity{{"aa", 22, "cc"}, {"aaa", 222, "ccc"}},
			expectOK:  true,
		},
		{
			name:     "invalid",
			kv:       map[CacheKey][]byte{"foo": []byte(`}{`)},
			expectOK: false,
		},
		{
			name:      "notslice into slice is invalid",
			kv:        map[CacheKey][]byte{"foo": []byte(`{"A":"a","B":2,"C":"c"}`)},
			intoSlice: true,
			expectOK:  false,
		},
		{
			name:     "not found",
			kv:       map[CacheKey][]byte{"wrongkey": []byte(`{}`)},
			expectOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cache := NewCache(nil, nil, 0, 0)
			cache.kv = c.kv

			var ok bool
			var out interface{}

			if c.intoSlice {
				outEnt := []*fakeEntity{}
				ok = cache.Get("foo", &outEnt)
				out = outEnt
			} else {
				outEnt := &fakeEntity{}
				ok = cache.Get("foo", &outEnt)
				out = outEnt
			}

			if c.expectOK {
				assert.Equal(t, c.expect, out)
				assert.True(t, ok)
			} else {
				assert.False(t, ok)
			}

		})
	}
}

type fakeEntity struct {
	A string
	B int
	C interface{}
}
