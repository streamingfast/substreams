package store

// ─────────────────────────────────────────────────────────────────────────────
// Design-verification tests for BadgerBackedStore
//
// These tests probe the current behaviour AND the intended correct behaviour.
// Tests named "BROKEN_*" are expected to FAIL with the current code; they
// document what is wrong.  Tests named "CORRECT_*" are expected to PASS; they
// document what already works.
//
// Run with:
//   go test ./storage/store/... -run "BROKEN_|CORRECT_" -v
//
// Once the design is fixed every test in this file should pass.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"math/big"
	"testing"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helper: simulate exactly what the pipeline does for one block.
//
//	1. SetBlockNum
//	2. WASM writes (via store API)
//	3. wrapDeltasAndOps  →  Flush()
//	4. resetStores       →  Reset()
//
// ─────────────────────────────────────────────────────────────────────────────
func runBlock(t *testing.T, s *BadgerBackedStore, blockNum uint64, writes func()) []*pbsubstreams.StoreDelta {
	t.Helper()
	s.SetBlockNum(blockNum)
	writes()
	require.NoError(t, s.Flush())
	deltas := make([]*pbsubstreams.StoreDelta, len(s.GetDeltas()))
	copy(deltas, s.GetDeltas())
	s.Reset()
	return deltas
}

// ─────────────────────────────────────────────────────────────────────────────
// CORRECT: FullKV baseline — proves what the correct cross-block behaviour is.
//
// FullKV keeps kv across blocks.  An ADD store's delta on block N+1 must have
// OldValue = accumulated value from block N, NewValue = accumulated + delta.
// This test should always pass — it is the reference implementation.
// ─────────────────────────────────────────────────────────────────────────────
func TestCORRECT_FullKV_ADD_DeltaOldValueIsCorrect(t *testing.T) {
	s := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigint", nil)
	fullKV := &FullKV{baseStore: s}

	// Block 10: add 5 → kv["counter"] = "5"
	fullKV.SumBigInt(0, "counter", big.NewInt(5))
	require.NoError(t, fullKV.Flush())
	fullKV.Reset() // kv is NOT cleared by FullKV — this is the key point

	assert.Equal(t, []byte("5"), fullKV.kv["counter"],
		"CORRECT: FullKV.kv must survive Reset() — it is authoritative state")

	// Block 11: add 3 → delta must say UPDATE 5→8
	fullKV.SumBigInt(0, "counter", big.NewInt(3))
	require.NoError(t, fullKV.Flush())
	deltas := fullKV.GetDeltas()

	require.Len(t, deltas, 1)
	assert.Equal(t, pbsubstreams.StoreDelta_UPDATE, deltas[0].Operation,
		"CORRECT: second write to existing key is UPDATE, not CREATE")
	assert.Equal(t, []byte("5"), deltas[0].OldValue,
		"CORRECT: OldValue must be the value from block 10")
	assert.Equal(t, []byte("8"), deltas[0].NewValue,
		"CORRECT: NewValue must be 5+3=8")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCORRECT_BadgerBacked_ADD_DeltaOldValueIsCorrect verifies that ADD across
// blocks produces correct UPDATE deltas with the right OldValue and NewValue.
func TestCORRECT_BadgerBacked_ADD_DeltaOldValueIsCorrect(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	s := newTestBadgerStore(t, addr,
		pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigint")

	// Block 10: add 5
	deltasB10 := runBlock(t, s, 10, func() {
		s.SumBigInt(0, "counter", big.NewInt(5))
	})
	require.Len(t, deltasB10, 1)
	assert.Equal(t, pbsubstreams.StoreDelta_CREATE, deltasB10[0].Operation,
		"block 10 first write is CREATE")
	assert.Equal(t, []byte("5"), deltasB10[0].NewValue)

	// kv must survive Reset()
	assert.Equal(t, []byte("5"), s.kv["counter"],
		"kv must survive Reset() — it is authoritative state")

	// Block 11: add 3 — delta must be UPDATE 5→8
	deltasB11 := runBlock(t, s, 11, func() {
		s.SumBigInt(0, "counter", big.NewInt(3))
	})
	require.Len(t, deltasB11, 1)
	assert.Equal(t, pbsubstreams.StoreDelta_UPDATE, deltasB11[0].Operation,
		"second write to same key across blocks must be UPDATE")
	assert.Equal(t, []byte("5"), deltasB11[0].OldValue,
		"OldValue must be the accumulated value from block 10")
	assert.Equal(t, []byte("8"), deltasB11[0].NewValue,
		"NewValue must be 5+3=8")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCORRECT_BadgerBacked_GetLast_AfterReset_ServesCorrectCrossBlockValue
// verifies that after Reset(), GetLast for a downstream reader returns the
// accumulated value from the previous block (served from kv, which now
// survives Reset()).
func TestCORRECT_BadgerBacked_GetLast_AfterReset_ServesCorrectCrossBlockValue(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	s := newTestBadgerStore(t, addr,
		pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigint")

	// Block 10: add 5, flush, reset
	runBlock(t, s, 10, func() {
		s.SumBigInt(0, "counter", big.NewInt(5))
	})

	// At start of block 11 (after Reset), kv is empty.
	// A downstream get-mode reader calls GetLast("counter").
	// It must return "5" — the accumulated value from block 10.
	s.SetBlockNum(11)
	val, found := s.GetLast("counter")
	require.True(t, found,
		"WANT found=true: GetLast must find the value accumulated in block 10")
	assert.Equal(t, []byte("5"), val,
		"WANT val=5: GetLast must return the block-10 accumulated value")
}

// ─────────────────────────────────────────────────────────────────────────────
// CORRECT: SET store — single block, no cross-block state needed.
//
// A simple SET store is not affected by the kv-clearing bug because each
// block's delta stands alone (no accumulation).  This must always pass.
// ─────────────────────────────────────────────────────────────────────────────
func TestCORRECT_BadgerBacked_SET_SingleBlock_DeltaIsCorrect(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	s := newTestBadgerStore(t, addr,
		pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	deltas := runBlock(t, s, 10, func() {
		s.SetBytes(0, "key", []byte("hello"))
	})

	require.Len(t, deltas, 1)
	assert.Equal(t, pbsubstreams.StoreDelta_CREATE, deltas[0].Operation)
	assert.Equal(t, []byte("hello"), deltas[0].NewValue)
}

// ─────────────────────────────────────────────────────────────────────────────
// CORRECT: SET store — overwrite same key in next block produces UPDATE.
// ─────────────────────────────────────────────────────────────────────────────
func TestCORRECT_BadgerBacked_SET_CrossBlock_DeltaOperationIsUPDATE(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	s := newTestBadgerStore(t, addr,
		pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	// Block 10: set key="hello"
	runBlock(t, s, 10, func() {
		s.SetBytes(0, "key", []byte("hello"))
	})

	// Block 11: set key="world" — kv survives Reset() so Flush sees UPDATE
	deltasB11 := runBlock(t, s, 11, func() {
		s.SetBytes(0, "key", []byte("world"))
	})

	require.Len(t, deltasB11, 1)
	assert.Equal(t, pbsubstreams.StoreDelta_UPDATE, deltasB11[0].Operation,
		"cross-block SET on existing key must produce UPDATE")
	assert.Equal(t, []byte("hello"), deltasB11[0].OldValue,
		"OldValue must be the value from block 10")
	assert.Equal(t, []byte("world"), deltasB11[0].NewValue)
}

// ─────────────────────────────────────────────────────────────────────────────
// BROKEN: FullKV does NOT clear kv in Reset() — proves the contract.
//
// This test verifies the FullKV invariant that Reset() preserves kv.
// If this fails something is very wrong with the baseline.
// ─────────────────────────────────────────────────────────────────────────────
func TestCORRECT_FullKV_Reset_PreservesKV(t *testing.T) {
	s := newTestBaseStore(t, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes", nil)
	s.kv["key"] = []byte("value")
	s.Reset()
	assert.Equal(t, []byte("value"), s.kv["key"],
		"CORRECT: FullKV/baseStore.Reset() must NOT clear kv")
}

// ─────────────────────────────────────────────────────────────────────────────
// TestCORRECT_BadgerBacked_Reset_PreservesKV — BadgerBackedStore.Reset() must
// NOT clear kv, matching the FullKV contract.
func TestCORRECT_BadgerBacked_Reset_PreservesKV(t *testing.T) {
	addr, _ := startTestFoundationalStore(t, t.TempDir())
	s := newTestBadgerStore(t, addr,
		pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "bytes")

	s.kv["key"] = []byte("value")
	s.Reset()

	assert.Equal(t, []byte("value"), s.kv["key"],
		"Reset() must preserve kv as FullKV does")
}
