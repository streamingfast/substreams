---
description: Client guide for writing to and reading from a StreamingFast-managed Remote Feed Hosted Store
---

# Remote Feed Hosted Store — Client Guide

A **Remote Feed Hosted Store** is a managed, block‑aware key/value store that
StreamingFast runs for you. You write key/value entries into it over gRPC, and your
Substreams modules — or any gRPC client — read those entries back at a given block
height.

This guide is for the **Remote feed** flavor of a Hosted Store: you (or another
service) push values in. For a package that reads the chain and populates the
store, see the [Substreams Feed Hosted Store Guide](hosted-store-substreams-feed.md).

This guide shows you how to:

1. Understand what a hosted store is and when to use it
2. Create one and get its endpoint
3. Authenticate
4. Write entries into it (`Feed.Set`) and delete them (`Feed.Delete`)
5. Read entries back, either directly (`Store.Get` / `Store.GetFirst`) or from a Substreams module
6. Look up the proto reference

---

## 1. Concepts

A hosted store is a **key → value** map with a few blockchain‑specific properties:

- **Keys** are raw `bytes` — they can be a string, a hash, an address, or any composite key you choose.
- **Values** are a `google.protobuf.Any` — i.e. any protobuf message you define, tagged with its `@type`.
- **Readiness** — a store starts **not ready**. Direct `Get`/`GetFirst` then report the store hasn't reached the requested block yet (even if the keys are already written). A Substreams that queries a not‑ready store **hangs** until you flip it ready with `Feed.SetReady`.
- **Block‑aware reads** — every read targets a specific block number. The response tells you whether the store has ingested up to that block yet (`block_reached = true`). If `block_reached` is `false`, treat the result as not-yet-final rather than as "key not found".

You populate a hosted store by writing entries directly over gRPC with `Feed.Set`, and
remove them with `Feed.Delete` (the **remote feed** flow). Data written or deleted this
way is treated as **already final**. You can query the same keys directly over gRPC or
from a Substreams module — but a Substreams **hangs** if the store is not marked ready
(see below), while a direct `Get` returns immediately with `block_reached = false`.

---

## 2. Create a Remote Feed Hosted Store

Stores are provisioned through [**The Graph Market**](https://thegraph.market), under
**Hosted Services**:

- [https://thegraph.market/sinks/new](https://thegraph.market/sinks/new) — create a new service.
  1. Choose **Hosted Store**.
  2. Set the feed mode to **Remote feed** (off‑chain writes over gRPC). For a
     package that populates the store from the chain, use the
     [Substreams Feed Hosted Store Guide](hosted-store-substreams-feed.md) instead.
  3. Give it a **name** and a **Type URL**.
- [https://thegraph.market/sinks](https://thegraph.market/sinks) — list your services and copy
  the store's deployment ID.

Creating a Remote Feed Hosted Store only requires two fields:

- **Name** — a label for the store.
- **Type URL** — the fully‑qualified protobuf type of the values you'll feed in
  (e.g. `com.acme.example.v1.TestValue`). The protobuf package and message name are entirely
  free‑form — pick whatever fits your own project, this is not a StreamingFast‑defined type.

When you create one you get back a **deployment ID** (e.g. `depxado8480c1026b3edb7f`).
That ID is used in two places:

- as the **hostname** of the store's gRPC endpoint, and
- as the **store reference** in a Substreams manifest.

### Endpoint convention

Once deployed, the store is reachable at:

```
<deployment-id>.hs.streamingfast.io:443
```

Example: `depxado8480c1026b3edb7f.hs.streamingfast.io:443`

- TLS on port **443**, gRPC over HTTP/2.
- Authenticated with an **API key** (see next section).

### Generating client bindings

The store's Protobuf definitions are published on the Buf Schema Registry at
[buf.build/streamingfast/substreams-foundational-store](https://buf.build/streamingfast/substreams-foundational-store) —
you don't need to vendor the `.proto` files locally. Point `buf generate` at the remote
module and pick the plugin for your language, e.g. for Rust with a `buf.gen.yaml`:

```yaml
version: v2
plugins:
  - remote: buf.build/community/neoeinstein-prost
    out: gen/rust
  - remote: buf.build/community/neoeinstein-tonic
    out: gen/rust
```

```bash
buf generate buf.build/streamingfast/substreams-foundational-store
```

See the [Buf Generate docs](https://buf.build/docs/generate/overview/) for Go, TypeScript,
Python, and other language plugins.

---

## 3. Authentication

Calls to the store carry a StreamingFast **API key** in the gRPC `x-api-key` metadata:

```
x-api-key: <api-key>
```

An API key is **created automatically** when you create the store, so you don't have to make
one yourself beforehand. Manage your keys at
[https://thegraph.market/api-keys](https://thegraph.market/api-keys).

---

## 4. Write and delete entries

The remote‑feed ingest service is
[`sf.substreams.foundational_store.feed.v2.Feed`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.feed.v2#sf.substreams.foundational_store.feed.v2.Feed).
`Set` writes a batch of entries; `Delete` removes a batch of keys. Both are treated as
**final** and applied immediately (latest‑value semantics, no fork handling).

### Entry shape

See [`SinkEntries`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.model.v2#sf.substreams.foundational_store.model.v2.SinkEntries)
and [`Entry`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.model.v2#sf.substreams.foundational_store.model.v2.Entry)
on the Buf Schema Registry:

```protobuf
message SinkEntries {
  repeated Entry entries = 1;
  bool if_not_exist = 2;   // if true, skip keys that already exist
}

message Entry {
  Key key = 2;                  // key.bytes — raw bytes
  google.protobuf.Any value = 4; // any protobuf message, tagged with @type
}
```

### Calling `Feed.Set`

Send `Set` from the Buf Schema Registry explorer for
[`sf.substreams.foundational_store.feed.v2`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.feed.v2):

1. Open that page and select **Set**.
2. Set the target to `<deployment-id>.hs.streamingfast.io:443`.
3. Add metadata `x-api-key: <api-key>`.
4. Fill the `SinkEntries` request: one or more `Entry` values, each with `key.bytes` and a
   `google.protobuf.Any` `value`.

Notes:

- `key.bytes` is **base64** in JSON (gRPC bytes encoding).
- `value` is a `google.protobuf.Any`: the `@type` must be the fully‑qualified type URL of your value message, and the remaining fields are that message's fields.
- Use `"if_not_exist": true` at the `entries` level to avoid overwriting existing keys.

### Deleting keys — `Feed.Delete`

`Delete` removes a batch of keys. Missing keys are ignored. Like `Set`, the delete is
treated as **final** and applied immediately.

See [`DeleteRequest`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.feed.v2#sf.substreams.foundational_store.feed.v2.DeleteRequest)
on the Buf Schema Registry:

```protobuf
message DeleteRequest {
  repeated Key keys = 1;  // missing keys are ignored
}
```

Send `Delete` from the same Buf Schema Registry explorer for
[`sf.substreams.foundational_store.feed.v2`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.feed.v2):

1. Open that page and select **Delete**.
2. Set the target to `<deployment-id>.hs.streamingfast.io:443`.
3. Add metadata `x-api-key: <api-key>`.
4. Fill `keys` with one or more `key.bytes` values.

Notes:

- `key.bytes` is **base64** in JSON (gRPC bytes encoding).
- An empty `keys` list is a no-op; missing keys are ignored — the call still succeeds.
- This is a **hard delete**. A later `Get` / `GetFirst` returns `RESPONSE_CODE_NOT_FOUND`.
  It does not write a tombstone and does not return `RESPONSE_CODE_NOT_FOUND_FINALIZE`.

### Marking the store ready

A new store is **not ready**. Hold reads off while you populate it, then flip the flag with
`Feed.SetReady` — same Buf page, select **SetReady**, and send `{ "ready": true }`.

Until you call `SetReady` with `ready = true`:

- Direct `Get`/`GetFirst` return `block_reached = false` — even if the keys are already written.
- A Substreams that queries the store **hangs**. It does not error, skip, or time out; the
  runtime treats `block_reached = false` as "try again later" and waits until the store is
  marked ready, then continues.

Call `SetReady` only after the data consumers need is written. If a Substreams appears stuck
on the first block after you added a hosted‑store input, the store is almost certainly still
not ready.

---

## 5. Read entries

### Option A — query the store directly (`Store.Get`)

The query service is
[`sf.substreams.foundational_store.service.v2.Store`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.service.v2#sf.substreams.foundational_store.service.v2.Store).
Reads are performed **at a block** and return one result per requested key, in order. See
[`GetRequest`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.service.v2#sf.substreams.foundational_store.service.v2.GetRequest)
and [`GetResponse`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.service.v2#sf.substreams.foundational_store.service.v2.GetResponse):

```protobuf
message GetRequest {
  uint64 block_number = 1;        // query at this block
  bytes  block_hash   = 2;        // optional, for fork verification
  repeated Key keys   = 3;        // keys to look up
}

message GetResponse {
  bool block_reached = 1;         // false if the store hasn't reached block_number yet
  QueriedEntries entries = 2;     // one QueriedEntry per requested key
}
```

Call `Get` and `GetFirst` from the Buf Schema Registry explorer for
[`sf.substreams.foundational_store.service.v2`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.service.v2):

1. Open that page and select **Get** or **GetFirst**.
2. Set the target to `<deployment-id>.hs.streamingfast.io:443`.
3. Add metadata `x-api-key: <api-key>`.
4. Set `block_number` and the `keys` to look up (`key.bytes` is **base64** in JSON).

- **`Get`** returns exact key matches.
- **`GetFirst`** returns, for each requested key, the first key **≥** it (lexicographic order) — useful for range scans / "next key" lookups.

### Response codes

Each `QueriedEntry` carries a `code`:

| Code | Meaning |
|------|---------|
| `RESPONSE_CODE_FOUND` (1) | Key exists; `entry` holds the value. |
| `RESPONSE_CODE_NOT_FOUND` (2) | Key does not exist at the requested block. |
| `RESPONSE_CODE_NOT_FOUND_FINALIZE` (4) | Key was deleted after finality (historical reference). |
| `RESPONSE_CODE_UNSPECIFIED` (0) | Should not occur. |

`Feed.Delete` is a hard delete: subsequent reads of that key return `NOT_FOUND`, not
`NOT_FOUND_FINALIZE`.

Always check `block_reached` first: if it's `false`, the store hasn't ingested the block you
asked about yet (or it isn't marked ready), and a `NOT_FOUND` doesn't mean the key is truly absent.

### Option B — read from a Substreams module

This is the most common consumption path: your Substreams module declares the hosted store as
an input and queries it per block.

#### 1. Declare it in `substreams.yaml`

Add the store as a module input using `foundational-store: <deployment-id>@<version>`:

```yaml
modules:
  - name: map_query_test_store
    kind: map
    initialBlock: 1
    inputs:
      - source: sf.acme.type.v1.Block
      - foundational-store: <deployment-id>@v0.1.0
    output:
      type: proto:com.acme.example.v1.TestValue
```

> **Note:** the `@<version>` suffix (e.g. `@v0.1.0`) is **ignored by the system** — the store is
> resolved by its deployment ID alone — but it must be present to pass manifest validation.

Add the proto dependency in `buf.yaml` so the hosted store types are available:

```yaml
deps:
  - buf.build/streamingfast/substreams
  - buf.build/streamingfast/substreams-foundational-store
  - buf.build/googleapis/googleapis
```

#### 2. Query from a Substreams

The store is injected into your handler as a `FoundationalStore`. Call `get` with a slice of
keys; you get back a `QueriedEntries`.

```rust
use substreams::errors::Error;
use substreams::store::FoundationalStore;
use substreams::pb::sf::substreams::foundational_store::model::v2::ResponseCode;

mod pb;
use pb::com::acme::example::v1::TestValue;

#[substreams::handlers::map]
fn map_query_test_store(
    _block: pb::sf::acme::r#type::v1::Block,
    store: FoundationalStore,
) -> Result<TestValue, Error> {
    let key: &[u8] = b"test-key-1";
    let response = store.get(&[key]);

    let value = if let Some(entry) = response.entries.first() {
        let bytes = entry
            .entry
            .as_ref()
            .and_then(|e| e.value.as_ref().map(|v| v.value.clone()))
            .unwrap_or_default();

        if entry.code == ResponseCode::Found as i32 {
            String::from_utf8_lossy(&bytes).into_owned()
        } else {
            format!("code={}", entry.code)
        }
    } else {
        "NO_ENTRY".to_string()
    };

    Ok(TestValue { value })
}
```

The read is automatically performed at the block currently being processed, so results stay in
sync with the stream.

> **Not ready ⇒ hang.** If the store is not marked ready, `store.get(...)` does not return.
> The Substreams stalls at that block until someone calls `Feed.SetReady` with `ready = true`.
> This is intentional: the runtime treats `block_reached = false` as "try again later", not
> as a miss. Direct `Get`/`GetFirst` still return immediately with `block_reached = false`.

#### 3. Build and run

```bash
substreams build
substreams run substreams.yaml map_query_test_store \
  -e <substreams-endpoint>
```

(During local development against a dev stack you might use
`-e localhost:9088 --insecure --limit-processed-blocks=0`.)

---

## 6. End‑to‑end checklist

1. **Create** a Remote Feed Hosted Store in [The Graph Market](https://thegraph.market)
   (**Hosted Services** → **Hosted Store** → **Remote feed**) → note the **deployment ID**.
2. **Listen** for a response from the endpoint. The Store is now running.
3. **Authenticate** with an API key (`x-api-key`).
4. **Write** your entries with `Feed.Set` (and `Feed.Delete` to remove keys), then call
   `Feed.SetReady`. A Substreams that queries the store before that **hangs** until you
   mark it ready.
5. **Read** them either directly with `Store.Get` / `Store.GetFirst`, or by adding
   `foundational-store: <deployment-id>@<version>` to a Substreams module and calling
   `store.get(...)`.

---

## 7. Proto reference

The authoritative message and service definitions — `model/v2/model.proto`,
`feed/v2/feed.proto`, and `service/v2/service.proto` — are published at
[buf.build/streamingfast/substreams-foundational-store](https://buf.build/streamingfast/substreams-foundational-store).
Browse them there, or see the [Foundational Store Reference](../../references/foundational-store-reference.md)
for the architecture and data model behind them.

---

## Related resources

- [Hosted Services](hosted-services.md) — the managed Sink and Store offerings StreamingFast runs for you.
- [Substreams Feed Hosted Store Guide](hosted-store-substreams-feed.md) — let a Substreams package populate the store from the chain.
- [Foundational Stores](../composing-substreams/foundational-stores/foundational-stores.md) — chain-specific foundational stores populated by a Substreams package.
- [Consuming a Foundational Store](../../tutorials/consuming-foundational-store.md) — querying a foundational store from a Substreams module, imported as a package.
- [Hosting a Foundational Store](../../references/foundational-stores/hosting-foundational-stores.md) — running your own foundational store server instead of using a StreamingFast-managed one.
- [Foundational Store Reference](../../references/foundational-store-reference.md) — architecture and data model shared by both hosted and self-hosted stores.
