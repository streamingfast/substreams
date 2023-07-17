MODULE ?= map_noop
ENDPOINT ?= mainnet.eth.streamingfast.io:443
START_BLOCK ?= 16021772
STOP_BLOCK ?= +1

.PHONY: build
build:
	cargo build --target wasm32-unknown-unknown --release
	cp target/wasm32-unknown-unknown/release/substreams.wasm .

.PHONY: run_noop
run_noop: build
	substreams run -e $(ENDPOINT) substreams.yaml map_noop -s $(START_BLOCK) -t $(STOP_BLOCK)

.PHONY: run_block
run_block: build
	substreams run -e $(ENDPOINT) substreams.yaml map_block -s $(START_BLOCK) -t $(STOP_BLOCK)

.PHONY: pack
pack: build
	substreams pack substreams.yaml
