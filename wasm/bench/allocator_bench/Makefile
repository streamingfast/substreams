ALLOCATORS := default talc rlsf lol mini
OUTPUT_DIR := ./wasm_variants

.PHONY: all clean build-all

all: build-all

$(OUTPUT_DIR):
	mkdir -p $(OUTPUT_DIR)

# Build with default allocator (dlmalloc)
build-default: $(OUTPUT_DIR)
	./build-docker.sh default

# Build with Talc allocator
build-talc: $(OUTPUT_DIR)
	./build-docker.sh talc

# Build with rlsf allocator
build-rlsf: $(OUTPUT_DIR)
	./build-docker.sh rlsf

# Build with lol_alloc allocator
build-lol: $(OUTPUT_DIR)
	./build-docker.sh lol

# Build with mini-alloc allocator
build-mini: $(OUTPUT_DIR)
	./build-docker.sh mini

build-all: build-default build-talc build-rlsf build-lol build-mini
	@echo "All allocator variants built in $(OUTPUT_DIR)/"
	@ls -lh $(OUTPUT_DIR)/*.wasm

clean:
	cargo clean
	rm -rf $(OUTPUT_DIR)
