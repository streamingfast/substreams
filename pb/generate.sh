#!/bin/bash -u
# Copyright 2019 dfuse Platform Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"

# Protobuf definitions
PROTO=${1:-"$ROOT/proto"}

function main() {
  checks

  set -e
  pushd "$ROOT" >/dev/null

  generate_system pb/system/system.pb

  pushd "$ROOT/pb" > /dev/null

  generate "sf/substreams/v1/substreams.proto"
  generate "sf/substreams/v1/test/test.proto"
  generate "sf/substreams/v1/clock.proto"
  generate "sf/substreams/v1/modules.proto"
  generate "sf/substreams/v1/package.proto"

  popd >/dev/null
  popd >/dev/null

  echo "generate.sh - `date` - `whoami`" > $ROOT/pb/last_generate.txt
  echo "streamingfast/proto revision: `GIT_DIR=$ROOT/.git git rev-parse HEAD`" >> $ROOT/pb/last_generate.txt
}

# usage
# - generate <protoPath>
# - generate <protoBasePath/> [<file.proto> ...]
function generate() {
    base=""
    if [[ "$#" -gt 1 ]]; then
      base="$1"; shift
    fi

    for file in "$@"; do
      protoc -I$PROTO \
        --go_out=. --go_opt=paths=source_relative \
        --go-grpc_out=. --go-grpc_opt=paths=source_relative,require_unimplemented_servers=false \
         $base$file
    done
}

# - generate_rust <file.proto> <output>
function generate_rust() {
  file="$1"
  output="$2"

  protoc -I$PROTO \
    --prost_out="$output" \
    --prost_opt=bytes=false \
    "$file"
}

function generate_system() {
  directory="`dirname $1`"
  if [[ ! -d "$directory" ]]; then
    mkdir "$directory" > /dev/null
  fi

  protoc -I$PROTO \
    "$PROTO/sf/substreams/v1/clock.proto" \
    "$PROTO/sf/substreams/v1/modules.proto" \
    "$PROTO/sf/substreams/v1/package.proto" \
    "$PROTO/sf/substreams/v1/substreams.proto" \
    "$PROTO/sf/substreams/v1/test/test.proto" \
    "$PROTO/google/protobuf/any.proto" \
    "$PROTO/google/protobuf/descriptor.proto" \
    "$PROTO/google/protobuf/timestamp.proto" \
    --include_source_info \
    --descriptor_set_out "$1"
}

function checks() {
  # The old `protoc-gen-go` did not accept any flags. Just using `protoc-gen-go --version` in this
  # version waits forever. So we pipe some wrong input to make it exit fast. This in the new version
  # which supports `--version` correctly print the version anyway and discard the standard input
  # so it's good with both version.
  result=`printf "" | protoc-gen-go --version 2>&1 | grep -Eo v[0-9\.]+`
  if [[ "$result" == "" ]]; then
    echo "Your version of 'protoc-gen-go' (at `which protoc-gen-go`) is not recent enough."
    echo ""
    echo "To fix your problem, perform those commands:"
    echo ""
    echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.25.0"
    echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1.0"
    echo ""
    echo "If everything is working as expetcted, the command:"
    echo ""
    echo "  protoc-gen-go --version"
    echo ""
    echo "Should print 'protoc-gen-go v1.25.0' (if it just hangs, you don't have the correct version)"
    exit 1
  fi

  # The current version of `protoc-gen-prost` does not accept any flags. Just using `protoc-gen-go --version` in this
  # version waits forever. So we pipe some wrong input to make it exit fast. In the new version
  # which supports `--version` correctly, it prints the version anyway and discard the standard input
  # so it's good with both version.
  result=`printf "" | protoc-gen-prost --version 2>&1 | grep -Eo 0.1.[0-9]+`
  if [[ "$result" == "" ]]; then
    echo "Your version of 'protoc-gen-prost' (at `which protoc-gen-prost || echo N/A`) is not recent enough or not installed."
    echo ""
    echo "To fix your problem, perform those commands:"
    echo ""
    echo "  pushd /tmp"
    echo "    git clone -b integration git@github.com:streamingfast/protoc-gen-prost.git"
    echo "    cd protoc-gen-prost/protoc-gen-prost"
    echo "    cargo install --path ."
    echo "  popd"
    echo ""
    echo "If everything is working as expetcted, the command:"
    echo ""
    echo "  protoc-gen-prost --version"
    echo ""
    echo "Should print '0.1.3' (if it just hangs, you don't have the correct version)"
    exit 1
  fi
}

main "$@"
