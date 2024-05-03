# Substreams in Go with tinygo

First test is to receive a Clock in Go, and ship it to the Substreams engine, and have it run over there.
- Craft a first `map_mod` that prints something to logs.

First test is to unpack a raw Ethereum Block, from within `tinygo`.

## Build with

```bash
tinygo build -o wasm.wasm -target wasi -scheduler none .
```

## Debug with the server

In `firehose-core`, make sure `go.mod` has:

```
replace github.com/streamingfast/substreams => $HOME/path/to/your/substreams
```

Build with:
```bash 
cd firehose-core
go install -v ./cmd/firecore
```

In `~/sf/firehose-ethereum/devel/eth-local`, run:

```
DEBUG=true firecore start firehose,substreams-tier1,substreams-tier2 --common-live-blocks-addr="" --common-merged-blocks-store-url=./localblocks -c "" --block-type sf.ethereum.type.v2.Block --firehose-grpc-listen-addr=":9000"
```

Run the test with:

```bash
substreams gui ./substreams.yaml --plaintext -e 127.0.0.1:10016 -t +10 map_test
# or 
substreams run ./substreams.yaml --plaintext -e 127.0.0.1:10016 -t +10 map_test
```
