package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// errReader yields n good bytes worth of a real serialized stream then fails,
// mimicking an objStore read that returns context.Canceled mid-load.
type cancelAfterReader struct {
	data    []byte
	pos     int
	failAt  int
	failErr error
}

func (r *cancelAfterReader) Read(p []byte) (int, error) {
	if r.pos >= r.failAt {
		return 0, r.failErr
	}
	n := copy(p, r.data[r.pos:min(r.failAt, len(r.data))])
	r.pos += n
	return n, nil
}

// TestMmapLoadCancelCleanup reproduces the tier2 scenario: a mmap-backed FullKV
// is Load()ed, the load fails partway (simulating context cancellation), and
// then the store is Close()d (as the setupSubrequestStores defer does).
// The backing bbolt file MUST be gone afterwards.
func TestMmapLoadCancelCleanup(t *testing.T) {
	t.Setenv("SUBSTREAMS_STORE_BACKEND", "mmap")
	scratch := t.TempDir()

	// Build a real serialized snapshot to load from.
	src := createTestStore(t, "cancel_src", 0, scratch).kvImpl
	kv := make(map[string][]byte, 50_000)
	for i := 0; i < 50_000; i++ {
		kv[fmt.Sprintf("key:%08d", i)] = []byte(fmt.Sprintf("value_%d", i))
	}
	require.NoError(t, src.BatchSet(kv))
	src.Close()

	dst := createTestStore(t, "cancel_dst", 0, scratch)
	mm := dst.kvImpl.(*mmapKVImpl)
	path := mm.path
	// The backing file is unlinked at open time, so no process death can ever
	// orphan it: it is already absent from the directory while fully usable.
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "dst file should already be unlinked after open, stat err=%v", err)

	// Fail the load partway through.
	r := &cancelAfterReader{data: []byte("garbage-that-fails-to-parse-immediately"), failAt: 5, failErr: fmt.Errorf("context canceled")}
	_, loadErr := unmarshalIterInto(dst.kvImpl, dst.marshaller, r, nil)
	require.Error(t, loadErr, "load should fail")
	t.Logf("load error: %v", loadErr)

	// setupSubrequestStores defer path:
	require.NoError(t, dst.Close(), "close should succeed")

	_, statErr := os.Stat(path)
	require.True(t, os.IsNotExist(statErr), "bbolt file %s must be removed after Close, stat err=%v", path, statErr)

	// sanity: no substreams-store-*.db left in scratch
	leftovers, _ := filepath.Glob(filepath.Join(scratch, "substreams-store-*.db"))
	require.Empty(t, leftovers, "no orphan mmap files should remain: %v", leftovers)
}
