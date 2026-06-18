package store

// Benchmarks for store KV backends (memory vs mmap/bbolt).
//
// Each benchmark isolates exactly the operation under test:
//   - All setup (impl creation, data loading, serialization) happens outside b.Loop()
//     or inside b.StopTimer()/b.StartTimer() blocks.
//   - Keys are pre-computed to avoid fmt.Sprintf overhead inside hot loops.
//   - Impls are reused across iterations where the operation resets state (Load, LoadFromStream).
//
// Run with:
//
//	go test -run='^$' -bench='BenchmarkKV' -benchmem -count=6 ./storage/store/ | tee ~/t/stores-perf/v8_memory.txt
//	SUBSTREAMS_STORE_BACKEND=mmap go test -run='^$' -bench='BenchmarkKV' -benchmem -count=6 ./storage/store/ | tee ~/t/stores-perf/v8_mmap.txt
//	SUBSTREAMS_STORE_BACKEND=mmap_nosync go test -run='^$' -bench='BenchmarkKV' -benchmem -count=6 ./storage/store/ | tee ~/t/stores-perf/v8_mmap_nosync.txt
//	benchstat ~/t/stores-perf/v8_memory.txt ~/t/stores-perf/v8_mmap.txt
//
// Benchmarks:
//   - BenchmarkKV_Get            — N random reads from a populated store
//   - BenchmarkKV_Set            — N writes into a populated store
//   - BenchmarkKV_Load           — bulk-load from map[string][]byte (old path)
//   - BenchmarkKV_LoadFromStream  — streaming load from serialized bytes (production path)
//   - BenchmarkKV_Iter           — full sequential scan
//   - BenchmarkFullKV_Save       — serialize FullKV to bytes via MarshalStreamIter
//   - BenchmarkKV_SteadyState    — load once, hammer Get+Scan per block (Tier2 production shape)
//   - BenchmarkKV_Merge          — merge a small partial into a large FullKV

import (
	"bytes"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"testing"

	pbssinternal "github.com/streamingfast/substreams/pb/sf/substreams/intern/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"go.uber.org/zap"
)

// mapToIter converts a map[string][]byte into an iter.Seq2[StoreDataEntry, error]
// that yields sorted KV entries followed by a trailer. Used in tests/benchmarks
// to call the new Load(iter) interface method with pre-built maps.
func mapToIter(kv map[string][]byte) iter.Seq2[marshaller.StoreDataEntry, error] {
	return func(yield func(marshaller.StoreDataEntry, error) bool) {
		keys := make([]string, 0, len(kv))
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var totalSize uint64
		for _, k := range keys {
			v := kv[k]
			totalSize += uint64(len(k) + len(v))
			if !yield(marshaller.NewKVEntry(k, v), nil) {
				return
			}
		}
		yield(marshaller.NewTrailerEntry(&marshaller.StoreDataTrailer{TotalSizeBytes: totalSize}), nil)
	}
}

// saveToMap collects a Save() iterator into a map[string][]byte for test assertions.
func saveToMap(it iter.Seq2[marshaller.StoreDataEntry, error]) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for entry, err := range it {
		if err != nil {
			return nil, err
		}
		if entry.Trailer() != nil {
			continue
		}
		kv := entry.KV()
		result[kv.Key] = kv.Value
	}
	return result, nil
}

// newBenchKVImpl creates a KVImpl using the current SUBSTREAMS_STORE_BACKEND env var.
// Valid values: "memory" (default), "mmap", "mmap_nosync"
func newBenchKVImpl(b *testing.B) KVImpl {
	b.Helper()
	backend := os.Getenv("SUBSTREAMS_STORE_BACKEND")
	switch backend {
	case "mmap", "mmap_nosync", "mmap_batch", "mmap_batch_nosync":
		noSync := backend == "mmap_nosync" || backend == "mmap_batch_nosync"
		var impl *mmapKVImpl
		var err error
		if noSync {
			impl, err = newMmapKVImplNoSync(b.TempDir() + "/bench.db")
		} else {
			impl, err = newMmapKVImplWithConfig(filepath.Base(b.Name()), "benchhash", &MmapBackendConfig{ScratchSpace: b.TempDir()})
		}
		if err != nil {
			b.Fatalf("newMmapKVImpl: %v", err)
		}
		b.Cleanup(func() { impl.Close() })
		return impl
	default:
		return newMemoryKVImpl()
	}
}

// makeKV generates a map and pre-computed key slice of numKeys keys each with valueSize bytes.
func makeKV(numKeys, valueSize int) map[string][]byte {
	kv := make(map[string][]byte, numKeys)
	val := make([]byte, valueSize)
	for i := range val {
		val[i] = byte(i % 256)
	}
	for i := range numKeys {
		key := fmt.Sprintf("key:%08d", i)
		v := make([]byte, valueSize)
		copy(v, val)
		kv[key] = v
	}
	return kv
}

// makeKeys pre-computes key strings to avoid fmt.Sprintf in hot benchmark loops.
func makeKeys(numKeys int) []string {
	keys := make([]string, numKeys)
	for i := range numKeys {
		keys[i] = fmt.Sprintf("key:%08d", i)
	}
	return keys
}

// newBenchPartialStore creates a PartialKV backed by an in-memory impl (always),
// pre-loaded with kv. Partials are always in-memory in production.
func newBenchPartialStore(kv map[string][]byte) *PartialKV {
	impl := newMemoryKVImpl()
	impl.Load(mapToIter(kv))
	b := &baseStore{
		kvImpl: impl,
		Config: &Config{
			updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
			valueType:    "bytes",
		},
		recentlyDeletedPrefixes: make(DeletedPrefixes),
	}
	return &PartialKV{baseStore: b, seen: make(map[string]bool)}
}

// newBenchFullKVFromImpl creates a FullKV wrapping an already-loaded impl.
func newBenchFullKVFromImpl(impl KVImpl) *FullKV {
	bs := &baseStore{
		kvImpl: impl,
		kvOps:  &pbssinternal.Operations{},
		Config: &Config{
			updatePolicy: pbsubstreams.Module_KindStore_UPDATE_POLICY_SET,
			valueType:    "bytes",
		},
		logger:                  zap.NewNop(),
		recentlyDeletedPrefixes: make(DeletedPrefixes),
	}
	return &FullKV{baseStore: bs}
}

// storeSizes defines the FullKV sizes we benchmark against.
// Ranges from small (10MB) to production-scale (1GB).
// Production limit: SUBSTREAMS_STORE_SIZE_LIMIT=3.5GB, typical large stores are 500MB-1GB.
var storeSizes = []struct {
	name      string
	numKeys   int
	valueSize int
}{
	{"10k_keys", 10_000, 1_024},
	{"100k_keys", 100_000, 1_024},
	{"500k_keys", 500_000, 1_024},
	{"1M_keys", 1_000_000, 1_024},
}

// opCounts defines how many operations to perform per benchmark iteration.
var opCounts = []int{1_000, 10_000}

// BenchmarkKV_Get measures N random reads from a pre-populated store.
// Setup (impl creation + load) is outside the timed loop.
func BenchmarkKV_Get(b *testing.B) {
	for _, sz := range storeSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		keys := makeKeys(sz.numKeys)
		for _, ops := range opCounts {
			b.Run(fmt.Sprintf("store=%s/ops=%d", sz.name, ops), func(b *testing.B) {
				impl := newBenchKVImpl(b)
				if _, err := impl.Load(mapToIter(kv)); err != nil {
					b.Fatal(err)
				}

				b.ResetTimer()
				b.ReportAllocs()
				b.SetBytes(int64(ops) * int64(sz.valueSize))

				for b.Loop() {
					for j := range ops {
						_, _ = impl.Get(keys[j%sz.numKeys])
					}
				}
			})
		}
	}
}

// BenchmarkKV_Set measures N writes into a pre-populated store.
// Setup is outside the timed loop.
func BenchmarkKV_Set(b *testing.B) {
	val := make([]byte, 1_024)
	for _, sz := range storeSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		keys := makeKeys(sz.numKeys)
		for _, ops := range opCounts {
			b.Run(fmt.Sprintf("store=%s/ops=%d", sz.name, ops), func(b *testing.B) {
				impl := newBenchKVImpl(b)
				if _, err := impl.Load(mapToIter(kv)); err != nil {
					b.Fatal(err)
				}

				b.ResetTimer()
				b.ReportAllocs()
				b.SetBytes(int64(ops) * int64(sz.valueSize))

				for b.Loop() {
					for j := range ops {
						if err := impl.Set(keys[j%sz.numKeys], val); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}

// BenchmarkKV_Load measures bulk-loading from map[string][]byte (old path).
// Measures only Load + Close; impl creation is outside the timer.
func BenchmarkKV_Load(b *testing.B) {
	for _, sz := range storeSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		totalBytes := int64(sz.numKeys) * int64(sz.valueSize)
		b.Run(sz.name, func(b *testing.B) {
			// Create one impl to reuse — Load clears and repopulates the store.
			impl := newBenchKVImpl(b)

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				if _, err := impl.Load(mapToIter(kv)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkKV_LoadFromStream measures the production load path:
// serialized bytes → UnmarshalIter → LoadFromStream → bbolt.
// Impl is reused across iterations (LoadFromStream clears the bucket internally).
// Only the unmarshal+write is timed; reader reset and impl creation are excluded.
func BenchmarkKV_LoadFromStream(b *testing.B) {
	sm := &marshaller.VTproto{}

	for _, sz := range storeSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		totalBytes := int64(sz.numKeys) * int64(sz.valueSize)

		// Pre-serialize once outside any timing.
		srcImpl := newMemoryKVImpl()
		srcImpl.Load(mapToIter(kv))
		rc := sm.MarshalStreamIter(srcImpl.Iter, nil, totalBytes)
		serialized, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			b.Fatal(err)
		}

		b.Run(sz.name, func(b *testing.B) {
			impl := newBenchKVImpl(b)

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {

				reader := bytes.NewReader(serialized)

				if _, err := unmarshalIterInto(impl, sm, reader, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkKV_Iter measures a full sequential scan of a loaded store.
// Setup is outside the timed loop.
func BenchmarkKV_Iter(b *testing.B) {
	for _, sz := range storeSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		totalBytes := int64(sz.numKeys) * int64(sz.valueSize)
		b.Run(sz.name, func(b *testing.B) {
			impl := newBenchKVImpl(b)
			if _, err := impl.Load(mapToIter(kv)); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				count := 0
				if err := impl.Iter(func(_ string, _ []byte) error {
					count++
					return nil
				}); err != nil {
					b.Fatal(err)
				}
				_ = count
			}
		})
	}
}

// BenchmarkFullKV_Save measures the production save path:
// kvImpl.Iter → MarshalStreamIter → bytes.
// Impl is loaded once; only serialization is timed.
func BenchmarkFullKV_Save(b *testing.B) {
	sm := &marshaller.VTproto{}

	for _, sz := range storeSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		totalBytes := int64(sz.numKeys) * int64(sz.valueSize)

		b.Run(sz.name, func(b *testing.B) {
			impl := newBenchKVImpl(b)
			if _, err := impl.Load(mapToIter(kv)); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				rc := sm.MarshalStreamIter(impl.Iter, nil, totalBytes)
				if _, err := io.ReadAll(rc); err != nil {
					b.Fatal(err)
				}
				rc.Close()
			}
		})
	}
}

// BenchmarkKV_SteadyState measures the read-only workload of a long-lived FullKV.
// Models Tier2: store loaded once, hammered with Get+Scan every block.
// Keys are pre-computed; no allocation inside the timed loop except from the impl itself.
func BenchmarkKV_SteadyState(b *testing.B) {
	getsPerBlock := 100
	scansPerBlock := 10
	scanPrefix := "key:0000" // matches keys 00000000-00009999 (exists in all store sizes)

	for _, sz := range storeSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		keys := makeKeys(sz.numKeys)
		b.Run(sz.name, func(b *testing.B) {
			impl := newBenchKVImpl(b)
			if _, err := impl.Load(mapToIter(kv)); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(int64(getsPerBlock+scansPerBlock) * int64(sz.valueSize))

			for b.Loop() {
				for j := range getsPerBlock {
					_, _ = impl.Get(keys[j%sz.numKeys])
				}
				for range scansPerBlock {
					_ = impl.Scan(scanPrefix, func(_ string, _ []byte) bool { return true })
				}
			}
		})
	}
}

// BenchmarkKV_SteadyState_GetOnly measures a pure Get workload with no scans.
// Many production modules (counters, balances) only use Get/Set, never prefix scans.
// This isolates whether mmap's B+tree advantage holds without the Scan contribution.
func BenchmarkKV_SteadyState_GetOnly(b *testing.B) {
	getsPerBlock := 200

	for _, sz := range storeSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		keys := makeKeys(sz.numKeys)
		b.Run(sz.name, func(b *testing.B) {
			impl := newBenchKVImpl(b)
			if _, err := impl.Load(mapToIter(kv)); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(int64(getsPerBlock) * int64(sz.valueSize))

			for b.Loop() {
				for j := range getsPerBlock {
					_, _ = impl.Get(keys[j%sz.numKeys])
				}
			}
		})
	}
}

// BenchmarkKV_Merge measures merging a small partial into a large FullKV.
// Partial and full store setup is excluded from timing via StopTimer.
func BenchmarkKV_Merge(b *testing.B) {
	partialSizes := []int{1_000, 10_000, 50_000}

	for _, sz := range storeSizes {
		fullKV := makeKV(sz.numKeys, sz.valueSize)
		for _, partialN := range partialSizes {
			if partialN > sz.numKeys {
				continue
			}
			partialKV := makeKV(partialN, sz.valueSize)
			b.Run(fmt.Sprintf("store=%s/partial=%d", sz.name, partialN), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(partialN) * int64(sz.valueSize))

				for b.Loop() {
					b.StopTimer()
					partial := newBenchPartialStore(partialKV)
					impl := newBenchKVImpl(b)
					if _, err := impl.Load(mapToIter(fullKV)); err != nil {
						b.Fatal(err)
					}
					full := newBenchFullKVFromImpl(impl)
					b.StartTimer()

					if err := full.Merge(partial); err != nil {
						b.Fatal(err)
					}

					b.StopTimer()
					impl.Close()
					b.StartTimer()
				}
			})
		}
	}
}
