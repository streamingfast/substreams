ENDPOINT ?= mainnet.eth.streamingfast.io:443
START_BLOCK ?= 16000000
STOP_BLOCK ?= +100

.PHONY: build
build:
	cargo build --target wasm32-unknown-unknown --release

.PHONY: stream
stream: build
	substreams run -e $(ENDPOINT) substreams.yaml map_events -s $(START_BLOCK) -t $(STOP_BLOCK)

.PHONY: protogen
protogen:
	substreams protogen ./substreams.yaml --exclude-paths="sf/substreams,google"

.PHONY: package
package: build
	substreams package substreams.yaml
