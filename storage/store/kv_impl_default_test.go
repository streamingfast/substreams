package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultBackendIsMemory pins that mmap is OPT-IN: when no backend is
// explicitly selected (empty config backend, no SUBSTREAMS_STORE_BACKEND env),
// the store resolves to the in-memory backend, never mmap. mmap only engages on
// an explicit "mmap". If this flips, every deployment that doesn't pass a
// backend silently moves onto the experimental mmap backend.
func TestDefaultBackendIsMemory(t *testing.T) {
	cases := []struct {
		name   string
		env    string // value for SUBSTREAMS_STORE_BACKEND ("" = unset)
		setEnv bool
		want   KVImplType
	}{
		{"env unset -> memory", "", false, KVImplTypeMemory},
		{"env empty -> memory", "", true, KVImplTypeMemory},
		{"env garbage -> memory", "bbolt", true, KVImplTypeMemory},
		{"env memory -> memory", "memory", true, KVImplTypeMemory},
		{"env mem -> memory", "mem", true, KVImplTypeMemory},
		{"env mmap -> mmap (opt-in)", "mmap", true, KVImplTypeMmap},
		{"env MMAP uppercase -> mmap", "MMAP", true, KVImplTypeMmap},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			os.Unsetenv("SUBSTREAMS_STORE_BACKEND")
			if c.setEnv {
				t.Setenv("SUBSTREAMS_STORE_BACKEND", c.env)
			}
			require.Equal(t, c.want, getKVImplTypeFromEnv())
		})
	}
}

// TestDefaultKVImplConfig_NilBackendDefaultsToMemory pins the same invariant at
// the config-resolution layer: a nil backend with no env resolves to memory.
func TestDefaultKVImplConfig_NilBackendDefaultsToMemory(t *testing.T) {
	os.Unsetenv("SUBSTREAMS_STORE_BACKEND")
	cfg := DefaultKVImplConfig("s", "h", nil)
	require.Equal(t, KVImplTypeMemory, cfg.Type)
}
