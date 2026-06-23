# PR Review — `feature/external-load`

Hosted foundational-store resolution via control-plane registry + TLS for foundational store gRPC + auth-header forwarding.

## TL;DR — why it does not build

CI is the failure, not local. `go.mod` and `go.work` were bumped to **`go 1.26.0`**, but `.github/workflows/ci.yml` still pins:

```yaml
matrix:
  go-version: [1.25.x]
```

A `go 1.26.0` directive cannot be built by a 1.25.x toolchain → `go build` / `go run ./docs/.gitbook/validate.go` fail immediately in CI. Locally it builds because this machine has `go1.26.0` and the module cache already holds the private deps.

Secondary CI risks (will bite even after the Go bump):
1. **Private dep** `github.com/streamingfast/services-control-plane` — CI needs `GOPRIVATE`/token auth to fetch it. Not configured in `ci.yml`.
2. **protobuf pinned to an unreleased pseudo-version** `v1.36.12-0.20260120151049-f2248ac996af` (a master snapshot). Fragile; prefer a tagged release.
3. Container builds rely on `GOWORK=off` (Dockerfile + go.work comment) — make sure CI uses the same, since `go.work` adds `./tests_e2e`.

**Fix:** bump `go-version` to `1.26.x` in `ci.yml` (and the PR compliance requirement noted in the comment), add `GOPRIVATE=github.com/streamingfast/*` + auth in CI, pin protobuf to a release tag.

---

## Bugs

- **`service/tier1.go:655`** — `%q` changed to `%T` in an error string:
  ```go
  bsstream.NewErrInvalidArg("Invalid progress_messages_interval_ms %T (minimum 500)", request.ProgressMessagesIntervalMs)
  ```
  `%T` prints the type (`uint32`), not the offending value. Regression — revert to `%q`/`%d`. Unrelated to PR intent; looks like an accidental find/replace.

- **`wasm/wasmtime/module.go:100`** — same `%q`→`%T` swap, on `entrypoint` which is `nil` in that branch. Prints `<nil>` type, useless. Revert. The message is also self-referential (formats the very thing that's nil) — should name the export, not the func.

- **`wasm/call.go:351`** — logs the raw `Authorization` Bearer token at Debug:
  ```go
  logger.Debug("foundational store get_all authorization", zap.String("authorization", a))
  ```
  **Secret in logs.** Redact (length/prefix only) or drop. Present in `DoFoundationalStoreGet`; not in `DoFoundationalStoreGetFirst` (inconsistent).

- **`wasm/call.go`** — `dauth.FromContext(c.ctx).ToOutgoingGRPCContext(...)` assumes auth is always in ctx. If nil → panic. Add a nil guard.

## Correctness / robustness — `pipeline/pipeline.go` registry client

- **Connection leak.** The `cpFSRegistryClient` `grpc.Dial` conn is stored on the pipeline but never added to `foundationalClosers` (or any shutdown path). Register a closer.
- **`context.Background()`** for `GetFoundationStore` — drops request deadline, cancellation, tracing, and auth. Use `p.ctx` (or the request ctx).
- **No timeout** on the registry RPC → can hang the whole render. Wrap with `context.WithTimeout`.
- **Hardcoded `insecure.NewCredentials()`** for the registry dial, while `foudational_store/dgrpc.go` in this same PR adds TLS support. A hosted control-plane registry almost certainly needs TLS — inconsistent and likely wrong in prod.
- **`grpc.Dial` is deprecated** — use `grpc.NewClient`.
- **Lazy client init without sync** — `if p.cpFSRegistryClient == nil { dial }`. Fine only if `renderWasmInputs` is strictly single-goroutine per pipeline; otherwise a race. Confirm or guard.
- **`@v` split** duplicated as ad-hoc string parsing here and mentioned in two commits — extract a helper, add a test.

## `foudational_store/dgrpc.go`

- `:443` TLS heuristic runs on `rawEndpoint`, but for a URL like `https://host:443` none of the scheme branches match and `rawEndpoint` keeps the `https://` prefix → bad dial target. Tighten: derive host first, then decide TLS.
- `credentials.NewTLS(&tls.Config{})` — empty config (no `MinVersion`). Acceptable default; set `MinVersion: tls.VersionTLS12` to be safe.

## Noise / cleanup

- Many leftover debug logs: `"got cpFSRegistryClient"`, `wasm/wasmtime/state_externs.go` `"calling DoFoundationalStoreGet"` / `"DoFoundationalStoreGet called"`, endpoint-resolution Info logs. Drop or downgrade before merge — these are clearly bisect/debugging artifacts (commit `62702e9` "add detailed logging").
- Auth-forwarding block is copy-pasted in both `DoFoundationalStoreGet*` — extract one helper.

## History hygiene

- Duplicate commits: `0e4e7e97` & `e1a1439e` (same message), `0c0c1bf4` & `ccd69172`. Plus a bare `wip`. Squash/rebase before merge.
- `go.work.sum` grew +309 lines — expected from the dep bumps, but verify nothing local-path leaked (`services-control-plane` go.work path was adjusted for cross-repo dev per `5259f89`).

## Verdict

Architecture is reasonable (3-tier resolution: JSON registry → control-plane → identifier fallback, opt-in via `HostedStoreRegistryAddress`, backward compatible). Blockers before merge: **CI Go version**, **token logging**, **two `%T` regressions**, **registry conn leak + `context.Background` + TLS**. Then strip debug logs and squash.
