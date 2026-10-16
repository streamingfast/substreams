# Sparse store snapshots: design & feasibility report

## Context

Today tier1's squasher (`orchestrator/stage/squash.go:475-562`, `singleSquash`)
merges every `PartialKV` into the cached `FullKV` in strict segment order, and
because `segmentEndsOnInterval` (`squash.go:483`, backed by
`block.Segmenter.EndsOnInterval`, `block/segmenter.go:110-115`) is true for
essentially every non-final segment — the segmenter's `interval` is always the
job's `segmentInterval`/`SegmentSize`
(`orchestrator/plan/requestplan.go:46,53,58`) — a full snapshot is written and
its source partial deleted (`squash.go:537-557`, delete at `squash.go:549`) at
*every* segment boundary. This investigation asks whether the squasher can
instead persist a full snapshot only every N segments, keep the intervening
partials on disk instead of deleting them, and let any reader reconstruct
state at an arbitrary boundary k by loading the nearest full snapshot ≤ k and
replaying the partials after it, in order, through the exact same merge code
the squasher already uses (`storage/store/merge.go:42`, `baseStore.Merge`).

This was triggered by evaluating a third-party proposal
(streamlined/layered-tree stores) that was set aside because it needs
associative combines across segments, which breaks bigdecimal/float sums and
moves cost to readers. The design below avoids that failure mode by
construction: it never combines partials out of order or in parallel, it only
ever replays them sequentially through `Merge`, which is the same operation
the squasher performs today. A separate, already-in-progress effort is adding
copy-on-write snapshots and parallel uploads in the squasher; this design does
not touch that work — it only changes *how often* a full snapshot write
happens and *what readers do* when one isn't there.

**Bottom line:** the core correctness claim holds — sequential partial replay
via `storage/store/merge.go` is bit-identical to today's incremental merge for
every update policy, including bigdecimal and float sums (see
[Determinism](#determinism-sequential-replay--incremental-merge)). The
storage-savings claim is plausible but unverified: it depends on the ratio of
partial size to full size, which is a function of each store's per-segment
write density, not something derivable from the code (see
[Partial retention and pruning](#4-partial-retention-and-pruning)). The
scheduling-latency claim is real at the squash layer (less synchronous save
I/O per batch) but is **not safely exploitable today** — tier2's store setup
(`pipeline/pipeline.go:514-519,572`) does a hard, exact-boundary file load with
no fallback, so a job scheduled against a lower stage that only merged-but-
didn't-save will fail outright until that load path gains a replay fallback
(see [Scheduler and plan](#2-scheduler-and-plan-changes)).

## 1. Full-snapshot readers inventory

Every place that currently builds an exact full-snapshot filename for an
arbitrary boundary and assumes it exists needs to learn "find nearest full
snapshot ≤ k, then replay partials forward" instead. Grouped by subsystem:

**tier2 job startup (hard failure path, highest priority to fix)**
- `pipeline/pipeline.go:464` `setupSubrequestStores`, in particular
  `pipeline.go:514-519` (`store.NewCompleteFileInfo(..., reqDetails.ResolvedStartBlockNum)`)
  and `pipeline.go:572` (`loadable.fullKVStore.Load(...)`) — a tier2 worker is
  a separate process from the tier1 orchestrator; it cannot fall back to any
  in-memory cache, so it must find a loadable artifact on durable storage at
  exactly its job's start block. This is the one caller where "full snapshot
  exists at any completed segment boundary" is a hard correctness invariant,
  not an optimization. `pipeline.go:579` even deletes the file on
  `ErrInvalidFullKVFile`, assuming corruption rather than "doesn't exist by
  design" — that assumption needs revisiting too.
- `pipeline/pipeline.go:274,320` (`openStoresFromQuickload`/`finishStoresFromQuickload`)
  resume from a `.quicksave` object keyed by block hash, a separate mechanism
  from segment-boundary snapshots; likely unaffected but shares the
  "primes a FullKV outside the segment-boundary path" shape worth re-checking
  once the segment-boundary path changes.

**tier1 squasher itself**
- `orchestrator/stage/modstate.go:67-98` (`getStore`) and `:111-135`
  (`tryLoadFullKV`) build exact-boundary `FileInfo`s and `Load` directly,
  used by `squash.go:488,496`. In-process caching (`cachedStore`) usually
  avoids hitting disk here as long as the squash chain runs in one process
  without restarting, but an orchestrator restart does hit this path.
- `orchestrator/stage/squash.go:328-366` (`getPartialOrFullKV`) races a
  partial load against `modState.tryLoadFullKV` for the *destination*
  boundary — an exact-match short-circuit, not a replay mechanism. It
  remains valid under sparse snapshots, it will simply hit far less often.
- `orchestrator/stage/stages.go:888-985` (`FinalStoreMap`) — loads the final
  store map at the linear-handoff boundary via the same exact-boundary
  `modState.getStore` call (line 928); used by tier1's own takeover into
  live processing (see below).
- `orchestrator/stage/fetchstorage.go:286-322` (`resumeSegment`) already
  implements a **list-based, nearest-boundary-with-caveats** scan: it walks
  backward in growing windows (`scanStoreSnapshots`, `fetchstorage.go:253-287`)
  and returns the highest segment ≤ target where *every* store module has a
  FullKV simultaneously. This is the closest existing analog to the new
  lookup and the best reuse candidate, but it still requires exact per-module
  boundary coincidence and does not replay partials afterward — downstream
  code still loads that exact FullKV and relies on in-memory `cachedStore`
  carry-over for everything after it, which breaks across an orchestrator
  restart once full snapshots are sparse.

**Production-mode / linear / live handoff**
- `pipeline/linear_handoff_lag.go:30-44`, `pipeline/resolve.go:129-179`
  (`computeLinearHandoffBlockNum`) always round the handoff block down to a
  multiple of `segmentSize`, implicitly assuming a full snapshot is loadable
  there via `FinalStoreMap` above.
- `service/tier1.go:1257-1289` (`configureLiveBackFillerFromQuickload`) is
  the **one existing caller that is already tolerant of gaps**: it walks
  `ListSnapshotFiles` backward and, via `lastLoadableSnapshotEnd`
  (`service/tier1.go:1289-1296,1304-1311`), accepts either a partial or a
  full KV whose start equals the module's initial block as a valid rewind
  point. It only picks a *segment* to re-request, though — the actual
  store rebuild for that segment still goes through the exact-boundary
  paths above.

**CLI / tools**
- `tools/decode.go:170` rounds a requested block down by `--save-interval`
  (default 1000, `decode.go:85`) and `tools/decode.go:480-489,533-542`
  (`printStateModule`/`searchStateModule`) load the exact FullKV at that
  rounded boundary with no fallback — will simply fail once snapshots are
  sparse at arbitrary rounded blocks.
- `tools/cleanup.go:11-86` assumes any partial older than the newest FullKV
  it has seen is safe to delete (see [§4](#4-partial-retention-and-pruning) —
  this is the one tool whose core assumption the design directly breaks).
- `tools/check.go:26-58` only checks partial contiguity, not full-KV density;
  compatible as-is, but would be a natural place to add a "max gap between
  full snapshots" invariant.
- `tools/module.go:111-162` and `tools/analytics_store_stats.go:249-299`
  (`getStore`) are informational/reporting only; the latter already does
  "list, pick newest FullKV, load it" but never replays partials after it —
  fine today since its only caller always asks for "latest," would need a
  replay step if ever asked for "as of block X."

**Remote squasher protocol (`squash/`)**
- `proto/sf/substreams/squasher/v1/service.proto:9-16` documents the wire
  contract explicitly: *"the RPC returns after the last full KV is on
  disk."* `squash/grpc/client.go:58-91` and
  `proto/.../service.proto:48-52` (`loaded_existing_full_kv`) are both
  framed purely in terms of "a FullKV exists at exclusive_end_block." The
  actual remote squasher server implementation lives outside this repo —
  any cadence change needs a wire-protocol change (a field carrying N, or a
  response flag meaning "stopped at a partial, not a full KV") plus a
  change in that external service. This is the one reader class this
  investigation **cannot fully resolve from this repo alone** and should be
  called out explicitly to whoever owns that service before N > 1 ships.

## 2. Scheduler and plan changes

**Can segmentation and snapshot interval be decoupled with existing interval
plumbing? No — not as-is.** `block.Segmenter`'s index space is itself defined
in units of its one `interval` field (`idx = blockNum/interval`,
`block/segmenter.go:54-67,73-100`), so a second `Segmenter` built with a
larger interval cannot be indexed by the same segment-index values the stage
uses — `Range(idx)`'s `baseBlock := idx*interval` would compute the wrong
block range entirely. `orchestrator/plan/requestplan.go` bakes this in
structurally: `RequestPlan` has exactly one `segmentInterval` field
(`requestplan.go:46`), threaded into every segmenter constructor it owns
(`requestplan.go:58,108-155`). `pipeline/stores.go:30`'s parameter named
`storeSnapshotSaveInterval` is suggestive scaffolding — the name already
matches this investigation's concept — but today it is always wired to equal
`segmentSize`/`SegmentSize` at both call sites
(`service/tier1.go:883`, `service/tier2.go:475`), so no decoupled value
exists in practice.

**What would need to change:**
- A new scalar (e.g. `snapshotInterval`, in segments) on `RequestPlan`
  (`requestplan.go:46`) and a parameter on `BuildTier1RequestPlan`
  (`requestplan.go:53`).
- A new field on `StoreModuleState` (`orchestrator/stage/modstate.go:15-28`)
  — a plain `uint64`, not a second `*block.Segmenter`, for the index-space
  reason above — and a parameter on `NewModuleState`
  (`modstate.go:30-37`), threaded from `stages.go:164`.
- `squash.go:483`'s `segmentEndsOnInterval := modState.segmenter.EndsOnInterval(...)`
  becomes a plain block-number check against the new interval
  (`rng.ExclusiveEndBlock % modState.snapshotInterval == 0`), with the
  identical replacement needed in `copySnapshots`/`copyModuleSnapshots`
  (`squash.go:275,297`).
- `orchestrator/stage/modstate.go:23-28`'s existing `snapshotEnd` field is
  already documented as "the end block of the last full store this module
  saved to or loaded from storage," distinct from `lastBlockInStore` — this
  is exactly the right conceptual split and already exists; today it's just
  always kept equal to `lastBlockInStore`. The `canCopySnapshots` invariant
  requiring `modState.snapshotEnd == rng.StartBlock` needs generalizing once
  `snapshotEnd` can trail `lastBlockInStore` by up to N-1 segments.
- `pipeline/pipeline.go:514-519,572` (`setupSubrequestStores`) needs genuinely
  new logic, not just new config: "load nearest full snapshot ≤ boundary,
  then replay partials forward" where today there is only an exact single-
  file load with a hard failure on miss. The symmetric case in
  `orchestrator/stage/modstate.go:67-98`/`111-135` needs the same treatment
  for when the squasher itself resumes without an in-memory cache (e.g.
  after an orchestrator restart).

**Does this change the scheduler's critical path, or only snapshot cadence?**
Grounded in `orchestrator/stage/stages.go:855-880` (`dependenciesCompleted`,
`previousUnitComplete`): a stage-N+1 job at segment k is unblocked once the
lower stage's merge chain reaches `UnitCompleted` for segment k-1 — a state
flipped by `MergeRunCompleted`/`MoveSegmentCompletedForward`
(`stages.go:525-553`) when `MsgMergeFinished` arrives
(`orchestrator/scheduler/scheduler.go:264-265`), with no notion of "was a
snapshot saved to disk" distinct from "was it merged." Since this design keeps
merging every segment in the same order and only makes the `Save()` call
conditional, **the segment at which a unit becomes `UnitCompleted` doesn't
move** — the scheduler unblocks the same job at the same segment-count
granularity as today. What does change is wall-clock latency: a squash run
batches up to `maxSquashRunSegments=1000` units (`squash.go:63-64,80-91`) and
today pays a synchronous `Save`+`Write` round trip on nearly every one of them
before reporting `MsgMergeFinished` for the batch; skipping that I/O for N-1
out of every N segments removes it from most units in the batch, so the batch
completes — and dependents get scheduled — sooner in real time.

That latency win is **real but not safely usable without the fix above**: a
stage-N+1 job scheduled sooner because its dependency merged-but-didn't-save
will, when it runs (possibly on a different tier2 worker process), hit
`pipeline.go:572`'s exact-file load and fail, because no replay-from-older-
snapshot path exists yet anywhere in the codebase. So: real win at the
squash/merge layer, illusory/unsafe as a whole-system win until `pipeline.go`
(tier2 store setup) gains the replay fallback described in §1.

The `stages.go:855-864` comment is worth quoting directly, since it already
states the assumption this design has to break: *"A job loads the lower
stages' fullKV at its segment start block, which is only guaranteed to exist
once the previous segment of every lower stage is done."* That assumption —
"completed implies fullKV exists at exactly this boundary" — needs to become
"completed implies state is reconstructable at this boundary," which is a
materially weaker and more accurate invariant once sparse snapshots land.

## 3. File naming, discovery, and compatibility

**Naming is already well-suited to this.** `storage/store/filename.go:90-92`
(`FullStateFileName`) and `:86-88` (`PartialFileName`) both lead with the
exclusive end block (`%010d-%010d`, end before start), so filenames already
sort lexicographically by end block. `storage/store/config.go:213-230`
(`ListSnapshotFiles`) exploits exactly this: it turns a `[from, inclusiveTo]`
end-block range into prefix bounds (`FilenamePrefix`, `filename.go:82-84`) and
calls `dstore.Store.WalkFromTo` — a single range-bounded listing, not a
filename-exists probe and not a full bucket scan. Each store module already
has its own dstore sub-path (`config.go:50-53`), so "enumerate partials
between full snapshot X and boundary k" is already a cheap, scoped listing on
GCS/S3 (native server-side range/prefix listing, confirmed in the vendored
`dstore` GCS/S3 `WalkFromTo` implementations — one paginated List call,
range-bounded, not a scan) and a bounded directory walk with early-stop on
local disk. Increasing the gap between full snapshots only adds listing cost
roughly linear in the number of partials in that gap — the same cost shape
`fetchstorage.go`'s existing widening-window scan already amortizes for "the
snapshot might be far behind."

**Discovery today is exact-match, not nearest-below, almost everywhere.**
`storage/store/state/snapshot.go:12-35` (`listSnapshots`) only accepts a
FullKV whose `StartBlock` equals the module's configured initial block, and
every correctness-critical caller
(`fetchstorage.go`'s `resumeSegment`/`checkStoresBelowFirstStreamableBlock`,
`pipeline/stores.go:91-101`, `service/tier2.go:987-999`) requires an exact
boundary match, with the sole exception of `service/tier1.go`'s live-
backfiller rewind (`lastLoadableSnapshotEnd`, §1 above), which tolerates gaps
but doesn't replay. None of this needs to change for the listing primitive
itself — `Config.ListSnapshotFiles` is already the right building block —
but a new "nearest full snapshot ≤ k, then enumerate partials after it"
helper needs to be written and substituted in at each of the exact-match call
sites named in §1.

**No version marker exists anywhere** — not in filenames, not in the
`StoreData` proto payload (`storage/store/marshaller/`). The only precedent
for "old and new files coexisting in one bucket" is the unrelated legacy
trace-ID-suffixed filename cleanup in `config.go:236-247`. This matters for
rollout: **the failure mode for a missing full snapshot is already loud, not
silent.** `storage/store/full_kv.go:164-182` defines `ErrSnapshotMissing`
("typically pruned while the request was running... cannot recover by
retrying; its resume point must be computed again") and
`service/tier2.go:824-829` maps it to gRPC `FailedPrecondition:
"...; restart the request"`. An **old binary** pointed at a bucket a **new**
tier1 has made sparse will hit exactly this existing, already-wired error
path — a request failure asking for a restart, not silent wrong output. The
one thing worth flagging: today's error message/semantics assume the gap is
sporadic/transient pruning, retryable by restart; under deliberate, steady-
state sparsity, restarting doesn't fix anything until the reader itself
understands sparse layout. Mixed-version rollout (old tier2 against a bucket
written by new tier1, or vice versa) needs either a hard version gate (reject
the version skew) or the reader-side fallback shipped everywhere before any
tier1 is allowed to skip saves — see [Rollout](#rollout-plan).

**No existing config field carries a decoupled interval.** The closest
candidate for a per-request/deployment knob is
`pb/sf/substreams/intern/v2/service.pb.go:86`
(`ProcessRangeRequest.SegmentSize`), which already crosses the tier1→tier2
wire (`service/tier1.go:278`→`orchestrator/work/worker.go:93`→`service/tier2.go`).
A new sibling field (e.g. `full_snapshot_interval`, in segments) on the same
message is the natural place to carry N explicitly tier1→tier2, rather than
having tier2 infer cadence purely from what it finds in storage.

## 4. Partial retention and pruning

**Deletion today is already conditional on exactly the right thing** —
`squash.go:549`'s `go partialKV.DeleteStore(...)` fires only inside the
`if segmentEndsOnInterval { ... }` block, after a successful full-store
write (`squash.go:537-557`); the symmetric batch path
(`copySnapshots`/`copyModuleSnapshots`, `squash.go:268-319`) has the same
gating. **When `segmentEndsOnInterval` is false, the partial is already kept**
— the code already supports "merge but keep the partial around," it's simply
never exercised today because the interval is always the segment size. Making
the interval genuinely N-segments-wide (§2) is therefore sufficient to get
retention "for free" at this call site; no new retention logic is needed
*here*, only the interval decoupling itself. Both deletes are fire-and-forget
(`go ...`, unawaited, best-effort logged-only failure,
`storage/store/partial_kv.go:88-95`, `storage/store/emptypartial.go:167-181`)
— acceptable today because `tools/cleanup.go` exists as a catch-up net; that
net's own logic needs to change (next paragraph).

**`tools/cleanup.go:11-86` is the one place whose core assumption this design
breaks**, and it needs a real rewrite, not just a parameter: today it treats
any partial older than *the newest FullKV it has seen* as safe to delete
(lines 58-77). Under sparse snapshots, the partials between two full
snapshots must survive until the *next* full snapshot exists — cleanup needs
"keep every partial whose segment is after the nearest-preceding full
snapshot and at or before the nearest-following one," not "newest FullKV
anywhere makes everything before it disposable." While reading this file the
research also surfaced a pre-existing, unrelated bug worth fixing as part of
the same change: the loop that tracks `highestKVBlock` (`cleanup.go:45-48`)
`return`s on the very first non-partial file it encounters, so in practice it
uses "first FullKV filename seen in listing order," not the true maximum —
flagging it here since the rewrite touches this exact loop anyway.

There is **no in-repo automatic pruning by age/count** for full snapshots
today — only an external tool (`firecore tools substreams purge`, in
`firehose-core`, not this repo) that reads a per-plan-tier "last used" marker
this repo writes (`service/tier1.go:682-694,869`) and applies retention
externally. Any new partial-retention policy introduced here should either
plug into that same external mechanism or stay purely "N segments relative to
the nearest full snapshot," which is self-contained and doesn't need
coordination with the external purge tool.

**Storage cost, grounded:** default segment size is 1000 blocks
(`tools/tier2call.go:39`, `service/tier1_estimate_test.go:30`). With N=10,
full-snapshot writes drop 10x (one every 10,000 blocks instead of every
1,000) — a hard fact from the interval math. The trade-off is retention
duration, not count: a partial today lives from its own merge until the very
next full-snapshot write succeeds (effectively one segment); under the
proposal, partials for all N-1 interior segments must live up to N segments
— retention-duration scales up by roughly the same factor the write-count
scales down by.

**Net byte impact is genuinely unverified and is the single biggest open
risk in this proposal.** `storage/store/full_kv.go:41-54`
(`DerivePartialStore`) starts a partial from a *fresh, empty* `kvImpl`, not a
clone of the FullKV's key space — `PartialKV.Save`
(`storage/store/partial_kv.go:58-77`) serializes only keys/deletions actually
touched during that segment. So partials are genuinely delta-shaped, not full
copies — this supports the "far below one full snapshot" claim in principle.
But the actual ratio depends entirely on each store's per-segment write
locality: a module that rewrites most of its keyspace every segment produces
a partial close to full size, in which case retaining N-1 of them per cycle
could net *more* total bytes than today's "full every segment, partial
deleted immediately" baseline, not less. This ratio is a runtime/workload
property, not something the code bounds — it must be measured per
representative module/chain before committing to the storage-savings claim,
and is exactly the kind of input the per-store N selection in §5 should use.

## 5. Reader cost model and choosing N

**The correctness mechanism and the cost mechanism are the same code.**
`storage/store/merge.go:42` (`baseStore.Merge`) is the *literal* function call
the squasher already uses (`squash.go:521`, `fullKV.Merge(partialKV)`); the
proposed reader-side replay calls it the same number of times, in the same
order, on the same files. `mergeBatched`/`mergeChunk`
(`merge.go:81-470`) stream the **partial's** keys (`KeyCount()`-sized
allocation, `merge.go:87`) and only do targeted reads/writes against the full
store for those keys (`GetManySizes`/`BatchSet`, `merge.go:153,158,465`) —
cost is `O(P)` where P is the partial's key count, never `O(S)` (full-store
key count). The doc comment at `merge.go:67-76` gives a concrete number: a
500k-key merge window costs ~700ms CPU / 550MB heap. This means:

```
reader_cost(N) ≈ full_snapshot_load_cost(S) + (N-1) × avg_partial_merge_cost(P_i)
```

where `full_snapshot_load_cost(S)` is `O(S)` (confirmed via
`pipeline/pipeline.go:572`→`storage/store/full_kv.go:174-209`→
`kv_impl_mmap.go:398-463`, a full rewrite of the local bbolt B+tree, matching
`BenchmarkKV_LoadFromStream` in `kv_impl_bench_test.go`), and
`avg_partial_merge_cost(P_i)` is `O(P_i)` — a function of per-segment *write
density* for that module, independent of total store size S.

**This argues for choosing N per-store by write density, not a single global
constant.** A store that rewrites 50 keys/segment can tolerate a large N
with negligible reader cost; a store that rewrites 500k keys/segment should
use a small N regardless of its total size S, since each replay step is
expensive. A fixed global N either over-penalizes high-touch stores or
under-amortizes full-snapshot writes for low-touch ones. The data needed to
make this choice already exists as logging, not yet as a persisted metric:
`merge.go:43` and `partial_kv.go:54` both log `partial_key_count`/`key_count`
per module per segment at Debug level; `orchestrator/stage/metrics.go` plus
`squash.go:477-528` already log per-module, per-segment merge/load timing;
`metrics/stats.go:541-555` (`RecordModuleMerging`) accumulates per-module
merge time. Promoting these from Debug logs to a persisted, queryable metric
is sufficient to derive "keys touched per segment" per store without adding
new instrumentation, and should be the first implementation step before
picking any N (fixed or per-store).

## Determinism: sequential replay = incremental merge

This is the load-bearing correctness claim and it holds, for every update
policy including bigdecimal and float sums. Grounds:

- The replay mechanism is not "equivalent logic" — it is the same function
  call, `fullKV.Merge(partialKV)` (`merge.go:42`), applied in the same
  order, on the same files the squasher already produces. Nothing about
  where a full snapshot happened to be persisted changes which partials get
  applied or in what order.
- `UPDATE_POLICY_SET` (`merge.go:172-175`), `SET_IF_NOT_EXISTS`
  (`merge.go:177-183`), and `APPEND` (`merge.go:185-199`, byte
  concatenation) are all strictly order-dependent across segments by
  design — correct only under strict, non-reordered sequential application,
  which both the squasher and the proposed replay do identically.
- `SET_SUM` (`merge.go:241-293`, `store_setsum.go`) implements an explicit
  `"set:"`/`"sum:"` prefix state machine that resets on `"set:"` regardless
  of prior state — this is the strongest evidence the design must never
  reorder or batch-combine partials, and it already doesn't: sequential
  replay preserves exactly the order the prefix state machine depends on.
- `ADD` (`merge.go:201-239`) and `MAX`/`MIN` (`merge.go:295-445`) across
  int64/float64/bigInt/bigDecimal are implemented as a strict two-operand
  fold — "current full-store value" + "this partial's delta" — repeated
  once per `Merge` call; no code path assumes or requires associativity
  across more than two operands at a time, so nothing here depends on how
  many full snapshots were skipped in between.
- Bigdecimal values go through `Truncate(34)` symmetrically on both
  operands at every single `Merge` call (`merge.go:233-234,506-527`, backed
  by `shopspring/decimal`'s exact big.Int mantissa — no floating-point
  rounding). Float64 values round-trip exactly through Go's `'g', -1`
  text formatting (`merge.go:550-559,604-606`). Truncation/rounding
  happens on every merge regardless of whether the prior state came from
  disk or stayed in memory — skipping intermediate full-snapshot writes
  introduces no new rounding behavior.
- Test coverage already exercises exactly this shape:
  `storage/store/merge_test.go:488-516` runs two sequential `Merge` calls
  and asserts size-accounting correctness after each;
  `storage/store/merge_chunk_test.go:45-48` proves windowed/chunked
  application is byte-identical to a single pass across every policy;
  `storage/store/merge_mmap_test.go:44-104` cross-validates the bigdecimal
  ADD path between mmap and in-memory backends with zero divergence.
- The orchestrator already guarantees strictly ascending, one-at-a-time
  merge order (`squash.go:80-132`, `claimMergeRun`/`squashRun`/
  `planSquashSteps`); the design does not need to add this guarantee, only
  preserve it on the read side.

The one caveat worth stating plainly in any follow-up spec: this guarantee is
about *merge order*, not abstract floating-point associativity. A design that
combined two partials before feeding them into `Merge`, or applied them out
of order, would **not** be guaranteed bit-identical for `ADD`/`SET_SUM` on
float64/bigDecimal. "Replay partials strictly in order through the existing
`Merge` call" avoids that failure mode by construction — which is exactly why
this design was chosen over the layered-tree alternative that required
associative combines.

## Risks

1. **Partial size vs. full size ratio is unmeasured.** The entire storage
   win depends on it (§4) and it varies per module/workload. Needs
   benchmark data before committing to any specific N as a default.
2. **Scheduling-latency win is currently unsafe to exploit.** `pipeline.go`'s
   tier2 store setup has no replay fallback; shipping the save-skip without
   first shipping that fallback would turn "merged but not saved" into
   outright tier2 job failures (§1, §2).
3. **Remote squasher protocol (`squash/`) has a hard "FullKV on disk when
   the RPC returns" contract** that this repo cannot unilaterally change —
   needs coordination with whoever owns that external server before any
   non-1 N ships for requests that can route through it.
4. **`tools/cleanup.go`'s core assumption is backwards for this design** and
   must be rewritten before partials are retained longer, or it will delete
   exactly the artifacts the new reader path depends on.
5. **Mixed-version rollout.** No version marker exists in snapshot
   filenames or content today. An old tier2/tool binary reading a bucket a
   new tier1 made sparse gets today's existing `FailedPrecondition` /
   "may have been pruned; restart the request" error — loud, not silently
   wrong, but "restart" won't help until the reader itself understands
   sparse layout. Needs either a version gate or reader-side fallback
   deployed everywhere first.
6. **Remote/live backfill and linear handoff round down to segment
   boundaries** (`pipeline/resolve.go:129-179`) and assume a loadable
   snapshot there; these need the same fallback as tier2 job startup.

## Rollout plan

1. Ship the reader-side fallback (nearest-full-snapshot + sequential
   partial replay) everywhere a full snapshot is currently loaded by exact
   boundary (§1), while N stays effectively 1 (current behavior). This is
   pure risk reduction: it changes nothing observable but proves the new
   lookup path end-to-end against today's always-dense snapshots.
2. Instrument per-store key-touch-rate as a persisted metric (§5), still
   with N=1, to gather the data needed to choose N sensibly.
3. Rewrite `tools/cleanup.go`'s retention logic (§4) to the new policy,
   deployed and verified against real buckets, still with N=1 in
   production (so nothing it protects against actually triggers yet).
4. Coordinate the wire-protocol change with the remote squasher owner
   (§1), or scope N > 1 to exclude requests routed through that service
   until it's updated.
5. Only then enable N > 1 (fixed default first, per-store sizing later),
   behind a config value that can be dialed per-deployment, with the old
   binaries' existing `FailedPrecondition` error as the safety net if
   anything is still pointed at a sparse bucket without the fallback.
6. Roll back by setting N back to 1; already-sparse buckets remain
   readable by the fallback path shipped in step 1, so rollback does not
   require backfilling missing full snapshots.

## Phased implementation plan and test strategy

**Phase 0 — reader fallback, N stays 1.**
- Add a "list snapshots in `[moduleInitialBlock, k]`, pick nearest FullKV,
  collect partials after it" helper on top of the existing
  `storage/store/state`/`config.ListSnapshotFiles` primitives (§3) — no new
  listing mechanism needed, just a new selection function.
- Wire it into `pipeline/pipeline.go` tier2 store setup, `modstate.go`'s
  `getStore`/`tryLoadFullKV`, and `fetchstorage.go`'s resume scan, replacing
  their exact-match-or-fail logic with "exact match if present, else
  fallback replay."
- Test: existing e2e/integration tests should pass unchanged (N=1 means the
  fallback path is never exercised in the exact-match case); add a test
  that deletes an intermediate full snapshot and asserts the fallback
  produces identical store state to a run where it wasn't deleted — this is
  the direct test of the determinism claim in this document, not just a
  unit test of `merge.go`.

**Phase 1 — decouple the interval, keep partials, N configurable.**
- Add `snapshotInterval` through `RequestPlan`, `StoreModuleState`, and the
  `squash.go` gating (§2), and the new `ProcessRangeRequest` proto field for
  tier1→tier2 (§3).
- Rewrite `tools/cleanup.go` retention (§4), including the pre-existing
  `highestKVBlock` loop bug.
- Test: unit tests on the interval math (segment boundary → "is this a
  snapshot boundary" for various N); an integration test running a full
  sync with N=10 and comparing final + intermediate store contents against
  an N=1 run of the same input, for at least one module per update policy
  (SET, ADD, SET_SUM, MAX, MIN, APPEND) including a bigdecimal-sum module.

**Phase 2 — remote squasher protocol.**
- Land the wire-protocol change (new field or response flag per §1) and
  coordinate the external server-side change; gate N > 1 to exclude
  requests on that path until both sides are updated.

**Phase 3 — per-store N sizing.**
- Once the key-touch-rate metric from §5/Phase 0 has real production data,
  revisit whether a fixed default N is good enough or whether per-store
  sizing is worth the added complexity.

Throughout, rollback at any phase is "set N back to 1 for new requests";
Phase 0's fallback path means already-sparse data remains readable
regardless of which phase is rolled back from.
