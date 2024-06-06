# Remote build

## Getting started

### Build the docker file locally

```bash
docker build -f Dockerfile.remotebuild -t remote-build .
```

### Run the docker file locally

```bash
docker run --rm --publish 9000:9000 remote-build
```

### Build request from client

```bash
cd examples
# then with any of your substreams that you have zipped up: zip -r substreams.zip ./substreams
go run main.go substreams.zip
```

The remote build needs some prerequisite files:

- `substreams.yaml` (this is the building block and the core of your substreams, you need to have this)
- `Cargo.toml` (this will contain all the dependencies that we will install and that are needed for your substreams)
- `proto/all_of_your_protos` (a directory containing all the protobuf files which you are using in your substreams)
- `src/` (folder containing the content of your substreams)
- `Makefile` (ideally you need to have the build and the package command)

Here is an example of a valid `Makefile`

```bash
.PHONY: protogen
protogen:
	substreams protogen ./substreams.yaml --exclude-paths="google,sf/substreams,substreams/sink/kv,database.proto"

.PHONY: build
build: protogen
	cargo build --target wasm32-unknown-unknown --release

.PHONE: package
package: build
	substreams pack -o substreams.spkg substreams.yaml
```
