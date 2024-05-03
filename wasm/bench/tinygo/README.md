# Substreams in Go with tinygo

First test is to receive a Clock in Go, and ship it to the Substreams engine, and have it run over there.
- Craft a first `map_mod` that prints something to logs.

First test is to unpack a raw Ethereum Block, from within `tinygo`.

## Build with

```bash
tinygo build -o wasm.wasm -target wasi .
tinygo build -o wasm.wasm -target wasm .
```

Errors:

```
024-04-19T17:47:01.645-0400 ERRO (substreams-tier1.tier1) panic while process block {"trace_id": "d307fe18f35a17942cdb95aeb24cdc2f", "block_num": 12360600, "error": "panic at block number:12360600  id:\"c9a86e6820988c09b28473ab86a691570523de7c409eac2c6104146602ddc33b\"  parent_id:\"f57d8e11c7c2b307bec372042cee0bc761f5cbcc772d9177c2b7b249bd6684f2\"  timestamp:{seconds:1620036704}  lib_num:12360400  payload_kind:ETH  payload_version:3
```

## Run the server

In `firehose-core`, make sure `go.mod` has:

```
replace github.com/streamingfast/substreams => /Users/abourget/sf/substreams
```

Build with:
```bash 
cd firehose-core
go install -v ./cmd/firecore
```

In `~/sf/firehose-ethereum/devel/eth-local`, run:

```
DEBUG=true SUBSTREAMS_WASM_RUNTIME=wazero firecore start firehose,substreams-tier1,substreams-tier2 --common-live-blocks-addr="" --common-merged-blocks-store-url=./localblocks -c "" --block-type sf.ethereum.type.v2.Block --firehose-grpc-listen-addr=":9000"
```

Build the WASM with:
```
tinygo build -o wasm.wasm -target wasi -scheduler none .
```


Run the test with:

```
substreams gui ./substreams.yaml --plaintext -e 127.0.0.1:10016 -t +1 map_test
```