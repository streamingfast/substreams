package store

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/shopspring/decimal"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (b *baseStore) setKV(k string, v []byte) {
	b.recentlyDeletedPrefixes.RemoveMatching(k)

	if prev, ok := b.kvImpl.Get(k); ok {
		b.totalSizeBytes -= uint64(len(prev))
	} else {
		b.totalSizeBytes += uint64(len(k))
	}
	b.totalSizeBytes += uint64(len(v))
	if err := b.kvImpl.Set(k, v); err != nil {
		panic(fmt.Sprintf("failed to set key %q: %v", k, err))
	}
}

func (b *baseStore) setNewKV(k string, v []byte) {
	b.recentlyDeletedPrefixes.RemoveMatching(k)

	b.totalSizeBytes += uint64(len(k) + len(v))
	if err := b.kvImpl.Set(k, v); err != nil {
		panic(fmt.Sprintf("failed to set key %q: %v", k, err))
	}
}

// Merge nextStore _into_ `s`, where nextStore is for the next contiguous segment's store output.
func (b *baseStore) Merge(kvPartialStore *PartialKV) error {
	b.logger.Debug("merging store", zap.Int("current_key_count", b.kvImpl.KeyCount()), zap.Uint64("mod_init_block", b.moduleInitialBlock), zap.Int("partial_key_count", kvPartialStore.kvImpl.KeyCount()), zap.Uint64("partial_start_block", kvPartialStore.initialBlock))

	if kvPartialStore.updatePolicy != b.updatePolicy {
		return fmt.Errorf("incompatible update policies: policy %q cannot merge policy %q", b.updatePolicy, kvPartialStore.updatePolicy)
	}

	if kvPartialStore.valueType != b.valueType {
		return fmt.Errorf("incompatible value types: cannot merge %q and %q", b.valueType, kvPartialStore.valueType)
	}

	partialKvTime := time.Now()
	for _, prefix := range kvPartialStore.DeletedPrefixes {
		b.DeletePrefix(kvPartialStore.lastOrdinal, prefix)
	}
	if err := b.Flush(); err != nil {
		return err
	}
	if len(kvPartialStore.DeletedPrefixes) > 0 {
		b.logger.Debug("merging: applied delete prefixes", zap.Duration("duration", time.Since(partialKvTime)))
	}

	return b.mergeBatched(kvPartialStore)
}

// mergeBatched is the optimized Merge path.
//
// Instead of Snapshot(partial) + N×Get() + N×Set(), it:
//  1. Streams partial keys via Iter() — no heap copy of the partial store
//  2. Reads existing FullKV values via GetMany() — single read transaction
//  3. Computes merged results in memory
//  4. Writes all results via BatchSet() — ~N/10k transactions instead of N
//
// This reduces mmap Merge from O(N) transactions to O(N/batchSize) transactions,
// cutting 500k-key Merge from ~15s / 30GB heap to ~700ms / 550MB heap.
func (b *baseStore) mergeBatched(kvPartialStore *PartialKV) error {
	intoValueTypeLower := strings.ToLower(b.valueType)

	// Step 1: Stream all partial keys into memory.
	// For mmap partial stores this is a single db.View() scan; for memory it's a sorted walk.
	// We collect into a slice to preserve key list for GetMany.
	type entry struct {
		key string
		val []byte
	}
	partialEntries := make([]entry, 0, kvPartialStore.kvImpl.KeyCount())
	if err := kvPartialStore.kvImpl.Iter(func(key string, value []byte) error {
		// Copy value — Iter contract says value is only valid during callback
		v := make([]byte, len(value))
		copy(v, value)
		partialEntries = append(partialEntries, entry{key, v})
		return nil
	}); err != nil {
		return fmt.Errorf("iterating partial store: %w", err)
	}

	// Step 2: Read existing FullKV values for all partial keys in a single transaction.
	// All policies need this for totalSizeBytes accounting (knowing if a key existed and
	// its old size). Policies other than SET also need the values to compute the merge result.
	keys := make([]string, len(partialEntries))
	for i, e := range partialEntries {
		keys[i] = e.key
	}
	existingKV, err := b.kvImpl.GetMany(keys)
	if err != nil {
		return fmt.Errorf("reading existing keys from full store: %w", err)
	}

	// Step 3: Compute merged results in memory.
	results := make(map[string][]byte, len(partialEntries))

	switch b.updatePolicy {
	case pbsubstreams.Module_KindStore_UPDATE_POLICY_SET:
		for _, e := range partialEntries {
			results[e.key] = e.val
		}

	case pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS:
		// existingKV populated above — only write keys not already present
		for _, e := range partialEntries {
			if _, found := existingKV[e.key]; !found {
				results[e.key] = e.val
			}
		}

	case pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND:
		for _, e := range partialEntries {
			if prevVal, found := existingKV[e.key]; found {
				newLen := len(prevVal) + len(e.val)
				if b.appendLimit > 0 && uint64(newLen) >= b.appendLimit {
					return fmt.Errorf("append would exceed limit of %d bytes", b.appendLimit)
				}
				nextVal := make([]byte, newLen)
				copy(nextVal, prevVal)
				copy(nextVal[len(prevVal):], e.val)
				results[e.key] = nextVal
			} else {
				results[e.key] = e.val
			}
		}

	case pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD:
		switch intoValueTypeLower {
		case manifest.OutputValueTypeInt64:
			sum := func(a, b int64) int64 { return a + b }
			for _, e := range partialEntries {
				v0b, fv0 := existingKV[e.key]
				v0 := foundOrZeroInt64(v0b, fv0)
				v1 := foundOrZeroInt64(e.val, true)
				results[e.key] = []byte(fmt.Sprintf("%d", sum(v0, v1)))
			}
		case manifest.OutputValueTypeFloat64:
			sum := func(a, b float64) float64 { return a + b }
			for _, e := range partialEntries {
				v0b, fv0 := existingKV[e.key]
				v0 := foundOrZeroFloat(v0b, fv0)
				v1 := foundOrZeroFloat(e.val, true)
				results[e.key] = floatToBytes(sum(v0, v1))
			}
		case manifest.OutputValueTypeBigInt:
			sum := func(a, b *big.Int) *big.Int { return new(big.Int).Add(a, b) }
			for _, e := range partialEntries {
				v0b, fv0 := existingKV[e.key]
				v0 := foundOrZeroBigInt(v0b, fv0)
				v1 := foundOrZeroBigInt(e.val, true)
				results[e.key] = []byte(fmt.Sprintf("%d", sum(v0, v1)))
			}
		case manifest.OutputValueTypeBigFloat:
			fallthrough
		case manifest.OutputValueTypeBigDecimal:
			sum := func(a, b decimal.Decimal) decimal.Decimal { return a.Add(b) }
			for _, e := range partialEntries {
				v0b, fv0 := existingKV[e.key]
				v0 := foundOrZeroBigDecimal(v0b, fv0)
				v1 := foundOrZeroBigDecimal(e.val, true)
				results[e.key] = []byte(sum(v0, v1).String())
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %q", b.updatePolicy, b.valueType)
		}

	case pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM:
		switch intoValueTypeLower {
		case manifest.OutputValueTypeInt64:
			sum := func(a, b int64) int64 { return a + b }
			for _, e := range partialEntries {
				if bytes.HasPrefix(e.val, []byte("set:")) {
					results[e.key] = append([]byte("sum:"), e.val[4:]...)
				} else {
					v0b, fv0 := existingKV[e.key]
					v0 := foundOrZeroPrefixedInt64(v0b, fv0)
					v1 := foundOrZeroPrefixedInt64(e.val, true)
					results[e.key] = []byte(fmt.Sprintf("sum:%d", sum(v0, v1)))
				}
			}
		case manifest.OutputValueTypeFloat64:
			sum := func(a, b float64) float64 { return a + b }
			for _, e := range partialEntries {
				if bytes.HasPrefix(e.val, []byte("set:")) {
					results[e.key] = floatToPrefixedBytes("sum:", bytesToFloat(e.val[4:]))
				} else {
					v0b, fv0 := existingKV[e.key]
					v0 := foundOrZeroPrefixedFloat(v0b, fv0)
					v1 := foundOrZeroPrefixedFloat(e.val, true)
					results[e.key] = floatToPrefixedBytes("sum:", sum(v0, v1))
				}
			}
		case manifest.OutputValueTypeBigInt:
			sum := func(a, b *big.Int) *big.Int { return new(big.Int).Add(a, b) }
			for _, e := range partialEntries {
				if bytes.HasPrefix(e.val, []byte("set:")) {
					results[e.key] = append([]byte("sum:"), e.val[4:]...)
				} else {
					v0b, fv0 := existingKV[e.key]
					v0 := foundOrZeroPrefixedBigInt(v0b, fv0)
					v1 := foundOrZeroPrefixedBigInt(e.val, true)
					results[e.key] = []byte(fmt.Sprintf("sum:%d", sum(v0, v1)))
				}
			}
		case manifest.OutputValueTypeBigFloat:
			fallthrough
		case manifest.OutputValueTypeBigDecimal:
			sum := func(a, b decimal.Decimal) decimal.Decimal { return a.Add(b) }
			for _, e := range partialEntries {
				if bytes.HasPrefix(e.val, []byte("set:")) {
					results[e.key] = []byte(fmt.Sprintf("sum:%s", string(e.val[4:])))
				} else {
					v0b, fv0 := existingKV[e.key]
					v0 := foundOrZeroPrefixedBigDecimal(v0b, fv0)
					v1 := foundOrZeroPrefixedBigDecimal(e.val, true)
					results[e.key] = bytes.Join([][]byte{[]byte("sum:"), []byte(sum(v0, v1).String())}, nil)
				}
			}
		}

	case pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX:
		switch intoValueTypeLower {
		case manifest.OutputValueTypeInt64:
			max := func(a, b int64) int64 {
				if a >= b {
					return a
				}
				return b
			}
			for _, e := range partialEntries {
				v1 := foundOrZeroInt64(e.val, true)
				v0b, found := existingKV[e.key]
				if !found {
					results[e.key] = []byte(fmt.Sprintf("%d", v1))
					continue
				}
				v0 := foundOrZeroInt64(v0b, true)
				results[e.key] = []byte(fmt.Sprintf("%d", max(v0, v1)))
			}
		case manifest.OutputValueTypeFloat64:
			max := func(a, b float64) float64 {
				if a < b {
					return b
				}
				return a
			}
			for _, e := range partialEntries {
				v1 := foundOrZeroFloat(e.val, true)
				v0b, found := existingKV[e.key]
				if !found {
					results[e.key] = floatToBytes(v1)
					continue
				}
				v0 := foundOrZeroFloat(v0b, true)
				results[e.key] = floatToBytes(max(v0, v1))
			}
		case manifest.OutputValueTypeBigInt:
			max := func(a, b *big.Int) *big.Int {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for _, e := range partialEntries {
				v1 := foundOrZeroBigInt(e.val, true)
				v0b, found := existingKV[e.key]
				if !found {
					results[e.key] = []byte(v1.String())
					continue
				}
				v0 := foundOrZeroBigInt(v0b, true)
				results[e.key] = []byte(fmt.Sprintf("%d", max(v0, v1)))
			}
		case manifest.OutputValueTypeBigFloat:
			fallthrough
		case manifest.OutputValueTypeBigDecimal:
			max := func(a, b decimal.Decimal) decimal.Decimal {
				if a.Cmp(b) <= 0 {
					return b
				}
				return a
			}
			for _, e := range partialEntries {
				v1 := foundOrZeroBigDecimal(e.val, true)
				v0b, found := existingKV[e.key]
				if !found {
					results[e.key] = []byte(v1.String())
					continue
				}
				v0 := foundOrZeroBigDecimal(v0b, true)
				results[e.key] = []byte(max(v0, v1).String())
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %q", kvPartialStore.updatePolicy, kvPartialStore.valueType)
		}

	case pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN:
		switch intoValueTypeLower {
		case manifest.OutputValueTypeInt64:
			min := func(a, b int64) int64 {
				if a <= b {
					return a
				}
				return b
			}
			for _, e := range partialEntries {
				v1 := foundOrZeroInt64(e.val, true)
				v0b, found := existingKV[e.key]
				if !found {
					results[e.key] = []byte(fmt.Sprintf("%d", v1))
					continue
				}
				v0 := foundOrZeroInt64(v0b, true)
				results[e.key] = []byte(fmt.Sprintf("%d", min(v0, v1)))
			}
		case manifest.OutputValueTypeFloat64:
			min := func(a, b float64) float64 {
				if a < b {
					return a
				}
				return b
			}
			for _, e := range partialEntries {
				v1 := foundOrZeroFloat(e.val, true)
				v0b, found := existingKV[e.key]
				if !found {
					results[e.key] = floatToBytes(v1)
					continue
				}
				v0 := foundOrZeroFloat(v0b, true)
				results[e.key] = floatToBytes(min(v0, v1))
			}
		case manifest.OutputValueTypeBigInt:
			min := func(a, b *big.Int) *big.Int {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for _, e := range partialEntries {
				v1 := foundOrZeroBigInt(e.val, true)
				v0b, found := existingKV[e.key]
				if !found {
					results[e.key] = []byte(v1.String())
					continue
				}
				v0 := foundOrZeroBigInt(v0b, true)
				results[e.key] = []byte(fmt.Sprintf("%d", min(v0, v1)))
			}
		case manifest.OutputValueTypeBigFloat:
			fallthrough
		case manifest.OutputValueTypeBigDecimal:
			min := func(a, b decimal.Decimal) decimal.Decimal {
				if a.Cmp(b) <= 0 {
					return a
				}
				return b
			}
			for _, e := range partialEntries {
				v1 := foundOrZeroBigDecimal(e.val, true)
				v0b, found := existingKV[e.key]
				if !found {
					results[e.key] = []byte(v1.String())
					continue
				}
				v0 := foundOrZeroBigDecimal(v0b, true)
				results[e.key] = []byte(min(v0, v1).String())
			}
		default:
			return fmt.Errorf("update policy %q not supported for value type %q", b.updatePolicy, b.valueType)
		}

	default:
		return fmt.Errorf("update policy %q not supported", b.updatePolicy) // should have been validated already
	}

	// Step 4: Update totalSizeBytes accounting before BatchSet.
	// For each key we're writing: subtract old value size (if existed), add new value size.
	// For new keys: also add the key size.
	for k, newVal := range results {
		if oldVal, existed := existingKV[k]; existed {
			b.totalSizeBytes -= uint64(len(oldVal))
		} else {
			b.totalSizeBytes += uint64(len(k))
		}
		b.totalSizeBytes += uint64(len(newVal))
		b.recentlyDeletedPrefixes.RemoveMatching(k)
	}

	// Step 5: Write all results in batched transactions.
	if err := b.kvImpl.BatchSet(results); err != nil {
		return fmt.Errorf("batch writing merged results: %w", err)
	}

	b.Reset() // Merge should never keep deltas or ordinals
	return nil
}

func foundOrZeroInt64(in []byte, found bool) int64 {
	if !found {
		return 0
	}
	val, err := strconv.ParseInt(string(in), 10, 64)
	if err != nil {
		return 0
	}
	return int64(val)
}

func foundOrZeroPrefixedInt64(in []byte, found bool) int64 {
	if !found {
		return 0
	}
	val, err := strconv.ParseInt(string(in[4:]), 10, 64)
	if err != nil {
		return 0
	}
	return int64(val)
}

func foundOrZeroPrefixedFloat(in []byte, found bool) float64 {
	if !found {
		return 0
	}

	val, err := strconv.ParseFloat(string(in[4:]), 64)
	if err != nil {
		return 0
	}
	return val
}

func foundOrZeroBigDecimal(in []byte, found bool) decimal.Decimal {
	if !found {
		return decimal.NewFromInt(0)
	}
	out, err := decimal.NewFromString(string(in))
	if err != nil {
		panic(err)
	}
	return out.Truncate(34)
}

func foundOrZeroPrefixedBigDecimal(in []byte, found bool) decimal.Decimal {
	if !found {
		return decimal.NewFromInt(0)
	}

	out, err := decimal.NewFromString(string(in[4:]))
	if err != nil {
		panic(err)
	}
	return out.Truncate(34)
}

func foundOrZeroBigFloat(in []byte, found bool) *big.Float {
	if !found {
		return new(big.Float).SetPrec(100)
	}
	return bytesToBigFloat(in)
}

func foundOrZeroBigInt(in []byte, found bool) *big.Int {
	if !found {
		return new(big.Int)
	}
	return bytesToBigInt(in)
}

func foundOrZeroPrefixedBigInt(in []byte, found bool) *big.Int {
	if !found {
		return new(big.Int)
	}
	return bytesToBigInt(in[4:])
}

func foundOrZeroFloat(in []byte, found bool) float64 {
	if !found {
		return float64(0)
	}

	f, err := strconv.ParseFloat(string(in), 64)
	if err != nil {
		return float64(0)
	}
	return f
}

func strToBigFloat(in string) *big.Float {
	newFloat, _, err := big.ParseFloat(in, 10, 100, big.ToNearestEven)
	if err != nil {
		panic(fmt.Sprintf("cannot load float %q: %s", in, err))
	}
	return newFloat.SetPrec(100)
}

func strToFloat(in string) float64 {
	newFloat, _, err := big.ParseFloat(in, 10, 100, big.ToNearestEven)
	if err != nil {
		panic(fmt.Sprintf("cannot load float %q: %s", in, err))
	}
	f, _ := newFloat.SetPrec(100).Float64()
	return f
}

func strToBigInt(in string) *big.Int {
	bi := &big.Int{}
	_, success := bi.SetString(in, 10)
	if !success {
		panic(fmt.Sprintf("cannot load int %q", in))
	}
	return bi
}

func bytesToFloat(in []byte) float64 {
	return strToFloat(string(in))
}

func bytesToBigFloat(in []byte) *big.Float {
	return strToBigFloat(string(in))
}

func bytesToBigInt(in []byte) *big.Int {
	return strToBigInt(string(in))
}

func floatToStr(f float64) string {
	return big.NewFloat(f).Text('g', -1)
}

func floatToBytes(f float64) []byte {
	return []byte(floatToStr(f))
}

func floatToPrefixedBytes(prefix string, f float64) []byte {
	return []byte(fmt.Sprintf("%s%s", prefix, floatToStr(f)))
}

func bigFloatToStr(f *big.Float) string {
	return f.Text('g', -1)
}

func bigFloatToBytes(f *big.Float) []byte {
	return []byte(bigFloatToStr(f))
}
