package store

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMmapKeyCount_ConcurrentAccessNoRace exercises keyCount under concurrent
// writers and readers. The mmap struct documents that write ops run
// concurrently (they hold only the shared snapMu.RLock side), so a plain-int
// keyCount mutated there and read by KeyCount() is a data race — this test
// fails under `go test -race` on the pre-fix code and passes once keyCount is
// atomic. It also asserts the final count is exact (no torn increments).
func TestMmapKeyCount_ConcurrentAccessNoRace(t *testing.T) {
	impl, err := newMmapKVImplWithConfig("kc", "h", &MmapBackendConfig{ScratchSpace: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { impl.Close() })

	const writers = 8
	const perWriter = 500
	want := writers * perWriter

	var writersWG sync.WaitGroup
	// Concurrent writers, each on a disjoint key range so every Set is a genuine
	// new key and the final count is deterministic.
	for w := 0; w < writers; w++ {
		writersWG.Add(1)
		go func(w int) {
			defer writersWG.Done()
			for i := 0; i < perWriter; i++ {
				_ = impl.Set(fmt.Sprintf("w%02d-key-%06d", w, i), []byte("v"))
			}
		}(w)
	}

	// Concurrent readers hammering KeyCount() while writes are in flight — the
	// read side of the race.
	stop := make(chan struct{})
	var readersWG sync.WaitGroup
	for r := 0; r < 3; r++ {
		readersWG.Add(1)
		go func() {
			defer readersWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = impl.KeyCount()
				}
			}
		}()
	}

	writersWG.Wait()
	close(stop)
	readersWG.Wait()

	require.Equal(t, want, impl.KeyCount(), "final key count must be exact (no torn/lost increments)")
}
