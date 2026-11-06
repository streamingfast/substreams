package store

// Micro-benchmarks to isolate WHERE Merge time is spent with mmap backend.
//
// The question: is bbolt the problem, or is our implementation (N individual
// transactions instead of one)?
//
// Three suspects for the 50x Merge slowdown vs memory:
//   A. N × db.View()  — one read tx per key (in setKV, checking prev size)
//   B. N × db.Update() — one write tx per key (in Set, called by setKV/setNewKV)
//   C. Snapshot() → map materialization — allocating the full partial store into heap
//
// This file adds four benchmarks that surgically isolate each path:
//
//   BenchmarkMergePath_NReads       — cost of N db.View() calls (suspect A alone)
//   BenchmarkMergePath_NWrites      — cost of N db.Update() calls (suspect B alone)
//   BenchmarkMergePath_BatchWrite   — N writes in ONE transaction (the fix for B)
//   BenchmarkMergePath_IterVsSnap   — Iter (streaming) vs Snapshot (materialized map)
//
// Run with:
//   go test -run='^$' -bench='BenchmarkMergePath' -benchmem -count=6 ./storage/store/ \
//     2>&1 | tee ~/t/merge_micro.txt

import (
	"fmt"
	"testing"

	"go.etcd.io/bbolt"
)

type storeSize struct {
	name      string
	numKeys   int
	valueSize int
}

// mergeMicroSizes covers the same range as the main benchmarks.
var mergeMicroSizes = []storeSize{
	{"10k_keys", 10_000, 1_024},
	{"100k_keys", 100_000, 1_024},
}

// BenchmarkMergePath_NReads measures the cost of reading every key once via
// individual db.View() calls — the pattern used by setKV to get the previous
// value length for totalSizeBytes accounting.
// This isolates suspect A: per-key read transaction overhead.
func BenchmarkMergePath_NReads(b *testing.B) {
	for _, sz := range mergeMicroSizes {
		b.Run(sz.name, func(b *testing.B) {
			impl := newBenchKVImpl(b)
			kv := makeKV(sz.numKeys, sz.valueSize)
			if _, err := impl.Load(mapToIter(kv)); err != nil {
				b.Fatal(err)
			}
			keys := make([]string, 0, sz.numKeys)
			for k := range kv {
				keys = append(keys, k)
			}
			totalBytes := int64(sz.numKeys) * int64(sz.valueSize)

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				for _, k := range keys {
					_, _ = impl.Get(k)
				}
			}
		})
	}
}

// BenchmarkMergePath_NWrites measures the cost of writing every key once via
// individual db.Update() calls — the pattern used by Set() which is called
// once per key by setKV/setNewKV during Merge.
// This isolates suspect B: per-key write transaction overhead.
func BenchmarkMergePath_NWrites(b *testing.B) {
	for _, sz := range mergeMicroSizes {
		b.Run(sz.name, func(b *testing.B) {
			kv := makeKV(sz.numKeys, sz.valueSize)
			keys := make([]string, 0, sz.numKeys)
			for k := range kv {
				keys = append(keys, k)
			}
			totalBytes := int64(sz.numKeys) * int64(sz.valueSize)

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				b.StopTimer()
				impl := newBenchKVImpl(b)
				b.StartTimer()

				for _, k := range keys {
					if err := impl.Set(k, kv[k]); err != nil {
						b.Fatal(err)
					}
				}

				b.StopTimer()
				impl.Close()
				b.StartTimer()
			}
		})
	}
}

// BenchmarkMergePath_BatchWrite measures the cost of writing every key in
// a single batched transaction — the proposed fix for Merge.
// Compares directly against BenchmarkMergePath_NWrites.
func BenchmarkMergePath_BatchWrite(b *testing.B) {
	for _, sz := range mergeMicroSizes {
		b.Run(sz.name, func(b *testing.B) {
			kv := makeKV(sz.numKeys, sz.valueSize)
			totalBytes := int64(sz.numKeys) * int64(sz.valueSize)

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				b.StopTimer()
				impl := newBenchKVImpl(b)
				b.StartTimer()

				if err := impl.BatchSet(kv); err != nil {
					b.Fatal(err)
				}

				b.StopTimer()
				impl.Close()
				b.StartTimer()
			}
		})
	}
}

// BenchmarkMergePath_IterVsSnap compares streaming via Iter (zero extra heap)
// against materializing via Snapshot (full copy into map[string][]byte).
// This isolates suspect C: map materialization cost.
//
// Two sub-benchmarks per size:
//
//	snap — current Merge path: Snapshot() → range over map
//	iter — proposed fix: Iter() → callback per key
func BenchmarkMergePath_IterVsSnap(b *testing.B) {
	for _, sz := range mergeMicroSizes {
		kv := makeKV(sz.numKeys, sz.valueSize)
		totalBytes := int64(sz.numKeys) * int64(sz.valueSize)

		b.Run(fmt.Sprintf("%s/snap", sz.name), func(b *testing.B) {
			impl := newBenchKVImpl(b)
			if _, err := impl.Load(mapToIter(kv)); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				snap, err := saveToMap(impl.Save())
				if err != nil {
					b.Fatal(err)
				}
				count := 0
				for range snap {
					count++
				}
				_ = count
			}
		})

		b.Run(fmt.Sprintf("%s/iter", sz.name), func(b *testing.B) {
			impl := newBenchKVImpl(b)
			if _, err := impl.Load(mapToIter(kv)); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				count := 0
				if err := impl.Iter(func(key string, value []byte) error {
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

// BenchmarkMergePath_NReadsInOneTx measures reading every key inside a SINGLE
// db.View() transaction — what a long-lived read tx or a merged Iter would give.
// Compares against BenchmarkMergePath_NReads (one tx per read).
func BenchmarkMergePath_NReadsInOneTx(b *testing.B) {
	for _, sz := range mergeMicroSizes {
		b.Run(sz.name, func(b *testing.B) {
			mmapImpl, ok := newBenchKVImpl(b).(*mmapKVImpl)
			if !ok {
				b.Skip("mmap backend required")
			}
			kv := makeKV(sz.numKeys, sz.valueSize)
			if _, err := mmapImpl.Load(mapToIter(kv)); err != nil {
				b.Fatal(err)
			}
			keys := make([]string, 0, sz.numKeys)
			for k := range kv {
				keys = append(keys, k)
			}
			totalBytes := int64(sz.numKeys) * int64(sz.valueSize)

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(totalBytes)

			for b.Loop() {
				_ = mmapImpl.db.View(func(tx *bbolt.Tx) error {
					bucket := tx.Bucket(defaultBucket)
					if bucket == nil {
						return nil
					}
					for _, k := range keys {
						v := bucket.Get([]byte(k))
						_ = v
					}
					return nil
				})
			}
		})
	}
}
