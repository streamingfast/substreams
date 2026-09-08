---
description: Client guide for a StreamingFast-managed Hosted Store populated by a Substreams package
---

# Substreams Feed Hosted Store — Client Guide

A **Substreams Feed Hosted Store** is a managed, block‑aware key/value store that
StreamingFast runs for you. A Substreams package reads the chain and writes
key/value entries into the store; your other modules — or any gRPC client — read
those entries back at a given block height.

This is the **Substreams feed** flavor of a Hosted Store. For off‑chain writes
over gRPC instead, see the [Remote Feed Hosted Store Guide](hosted-store-remote-feed.md).

This guide shows you how to:

1. Understand what a Substreams‑fed store is and when to use it
2. Produce `SinkEntries` from a Substreams package
3. Create the store and let StreamingFast populate it
4. Authenticate
5. Read entries back, either from a Substreams module or directly (`Store.Get`)
6. Look up the proto reference

---

## 1. Concepts

A hosted store is a **key → value** map with a few blockchain‑specific properties:

- **Keys** are raw `bytes` — they can be a string, a hash, an address, or any composite key you choose.
- **Values** are a `google.protobuf.Any` — i.e. any protobuf message you define, tagged with its `@type`.
- **Block‑aware reads** — every read targets a specific block number. The response tells you whether the store has ingested up to that block yet (`block_reached = true`). If `block_reached` is `false`, treat the result as not-yet-final rather than as "key not found".
- **Fork‑aware writes** — because a Substreams package is the source, reorgs are handled for you. (Remote‑feed writes are treated as already final; that is not this path.)

StreamingFast runs the Substreams sink against the chain and lands the output in
the store. There is no SQL database. You do not call `Feed.Set`, `Feed.Delete`, or
`Feed.SetReady`.

While the store is still catching up, a Substreams that queries it **hangs** at
that block until `block_reached` becomes true. Direct `Get`/`GetFirst` return
immediately with `block_reached = false`.

---

## 2. Produce entries from a package

The output module of the package **must** emit
`proto:sf.substreams.foundational_store.model.v2.SinkEntries`. That is the only
shape the hosted store will ingest.

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

### Producer module

```rust
use prost::Message;
use prost_types::Any;
use substreams::errors::Error;
use substreams::pb::sf::substreams::foundational_store::model::v2::{Entry, Key, SinkEntries};

mod pb;
use pb::com::acme::example::v1::TokenMetadata;

#[substreams::handlers::map]
fn map_to_store(events: pb::acme::Events) -> Result<SinkEntries, Error> {
    let mut entries = Vec::new();

    for event in events.items {
        let value = TokenMetadata {
            address: event.address.clone(),
            name: event.name,
            symbol: event.symbol,
        };
        let mut buf = Vec::new();
        Message::encode(&value, &mut buf)?;

        entries.push(Entry {
            key: Some(Key {
                bytes: event.address,
            }),
            value: Some(Any {
                type_url: "type.googleapis.com/com.acme.example.v1.TokenMetadata".to_string(),
                value: buf,
            }),
        });
    }

    Ok(SinkEntries {
        entries,
        if_not_exist: false,
    })
}
```

Set `if_not_exist: true` when a key should be written only once (for example an
account‑initialization mapping). Leave it `false` when later blocks should
overwrite the value.

### Manifest

```yaml
modules:
  - name: map_to_store
    kind: map
    inputs:
      - map: map_events
    output:
      type: proto:sf.substreams.foundational_store.model.v2.SinkEntries
```

The package must be reachable over public HTTPS (a GitHub Release, object
storage, or the [Substreams Registry](https://substreams.dev)) so the hosted
runner can fetch it. Local paths are not accepted.

Worked examples of producer packages:

- [Ethereum — ERC20 Token Metadata](../composing-substreams/foundational-stores/ethereum/erc20-token-metadata.md)
- [Solana — SPL Initialized Account](../composing-substreams/foundational-stores/solana/spl-initialized-account.md)

---

## 3. Create a Substreams Feed Hosted Store

Stores are provisioned through [**The Graph Market**](https://thegraph.market), under
**Hosted Services**:

- [https://thegraph.market/sinks/new](https://thegraph.market/sinks/new) — create a new service.
  1. Choose **Hosted Store**.
  2. Set the feed mode to **Substreams feed**. Do **not** pick **Remote feed**
     unless you will push values yourself over gRPC.
  3. Fill in the fields below.
- [https://thegraph.market/sinks](https://thegraph.market/sinks) — list your services and copy
  the store's deployment ID.

| Field | Description |
|---|---|
| **Name** | A label for the store. |
| **Package** | Public URL of the `.spkg`, or a Substreams.dev id (e.g. `spl-initialized-account@v0.2.0`). |
| **Type URL** | Fully‑qualified protobuf type of the values the module stores (e.g. `sf.substreams.solana.spl.v1.AccountOwner`). The wizard suggests types from the package. |
| **Network** | Chain id used to resolve the Substreams endpoint (e.g. `solana`, `mainnet`). |
| **Start block** | Block to start processing from. Use `0` for genesis. |
| **Stop block** | Block to stop at. Use `0` to follow the chain head. |
| **Output module** | Module that emits `SinkEntries` (e.g. `map_to_store`). |
| **Filters / Parameters** | Optional execution filters and module parameters. |

When you create one you get back a **deployment ID** (e.g. `depxado8480c1026b3edb7f`).
That ID is used in two places:

- as the **hostname** of the store's gRPC endpoint, and
- as the **store reference** in a Substreams manifest.

The store then catches up from the start block. Watch status, lag, and logs on
the service detail page the same way you would for a [hosted sink](../sinks/hosted-sinks/manage-your-sink.md).

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

## 4. Authentication

Calls to the store carry a StreamingFast **API key** in the gRPC `x-api-key` metadata:

```
x-api-key: <api-key>
```

An API key is **created automatically** when you create the store, so you don't have to make
one yourself beforehand. Manage your keys at
[https://thegraph.market/api-keys](https://thegraph.market/api-keys).

The examples below use `$SF_API_KEY` for that value.

---

## 5. Read entries

Reading is the same for both feed modes. The usual path is a Substreams module
that declares the store as an input. You can also query it directly over gRPC.

### Option A — read from a Substreams module

Add the store as a module input using `foundational-store: <deployment-id>@<version>`:

```yaml
modules:
  - name: map_with_store
    kind: map
    inputs:
      - source: sf.acme.type.v1.Block
      - foundational-store: <deployment-id>@v0.1.0
    output:
      type: proto:com.acme.example.v1.Enriched
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

The store is injected into your handler as a `FoundationalStore`. Call `get` with a slice of
keys; you get back a `QueriedEntries`. The read is performed at the block currently being
processed, so results stay in sync with the stream.

```rust
use substreams::errors::Error;
use substreams::store::FoundationalStore;
use substreams::pb::sf::substreams::foundational_store::model::v2::ResponseCode;

#[substreams::handlers::map]
fn map_with_store(
    _block: pb::sf::acme::r#type::v1::Block,
    store: FoundationalStore,
) -> Result<pb::com::acme::example::v1::Enriched, Error> {
    let key: &[u8] = b"example-key";
    let response = store.get(&[key]);

    let found = response.entries.iter().any(|e| e.code == ResponseCode::Found as i32);
    Ok(pb::com::acme::example::v1::Enriched { found })
}
```

> **Catching up ⇒ hang.** If the store has not yet ingested the block being processed,
> `store.get(...)` does not return. The Substreams stalls until the hosted store
> catches up, then continues. This is intentional: the runtime treats
> `block_reached = false` as "try again later", not as a miss.

### Option B — query the store directly (`Store.Get`)

The query service is
[`sf.substreams.foundational_store.service.v2.Store`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.service.v2#sf.substreams.foundational_store.service.v2.Store).
Reads are performed **at a block** and return one result per requested key, in order.

Call `Get` and `GetFirst` from the Buf Schema Registry explorer for
[`sf.substreams.foundational_store.service.v2`](https://buf.build/streamingfast/substreams-foundational-store/docs/main:sf.substreams.foundational_store.service.v2):

1. Open that page and select **Get** or **GetFirst**.
2. Set the target to `<deployment-id>.hs.streamingfast.io:443`.
3. Add metadata `x-api-key: <api-key>`.
4. Set `block_number` and the `keys` to look up (`key.bytes` is **base64** in JSON).

- **`Get`** returns exact key matches.
- **`GetFirst`** returns, for each requested key, the first key **≥** it (lexicographic order).

Always check `block_reached` first: if it's `false`, the store hasn't ingested that
block yet, and a `NOT_FOUND` doesn't mean the key is truly absent.

Each `QueriedEntry` carries a `code`:

| Code | Meaning |
|------|---------|
| `RESPONSE_CODE_FOUND` (1) | Key exists; `entry` holds the value. |
| `RESPONSE_CODE_NOT_FOUND` (2) | Key does not exist at the requested block. |
| `RESPONSE_CODE_NOT_FOUND_FINALIZE` (4) | Key was deleted after finality (historical reference). |
| `RESPONSE_CODE_UNSPECIFIED` (0) | Should not occur. |

---

## 6. End‑to‑end checklist

1. **Build** a package whose output module emits `SinkEntries`.
2. **Publish** it to a public URL or the Substreams Registry.
3. **Create** a Substreams Feed Hosted Store in [The Graph Market](https://thegraph.market)
   (**Hosted Services** → **Hosted Store** → **Substreams feed**) → note the **deployment ID**.
4. **Wait** until the store is catching up or live. A consumer Substreams that queries it
   before that block **hangs** until the store reaches it.
5. **Read** entries by adding `foundational-store: <deployment-id>@<version>` to a
   module, or with `Store.Get` / `Store.GetFirst` over gRPC.

---

## 7. Proto reference

The authoritative message and service definitions — `model/v2/model.proto` and
`service/v2/service.proto` — are published at
[buf.build/streamingfast/substreams-foundational-store](https://buf.build/streamingfast/substreams-foundational-store).
Browse them there, or see the [Foundational Store Reference](../../references/foundational-store-reference.md)
for the architecture and data model behind them.

---

## Related resources

- [Hosted Services](hosted-services.md) — the managed Sink and Store offerings StreamingFast runs for you.
- [Remote Feed Hosted Store Guide](hosted-store-remote-feed.md) — push values into a hosted store over gRPC instead of from a package.
- [Foundational Stores](../composing-substreams/foundational-stores/foundational-stores.md) — chain-specific packages that populate a store from the chain.
- [Consuming a Foundational Store](../../tutorials/consuming-foundational-store.md) — querying a foundational store from a Substreams module, imported as a package.
- [Hosting a Foundational Store](../../references/foundational-stores/hosting-foundational-stores.md) — running your own foundational store server instead of using a StreamingFast-managed one.
- [Foundational Store Reference](../../references/foundational-store-reference.md) — architecture and data model shared by both hosted and self-hosted stores.
