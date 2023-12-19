#!/bin/bash
# Copyright 2021 dfuse Platform Inc.
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

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd ../.. && pwd )"

# Protobuf definitions
PROTO=${1:-"$ROOT/../proto"}
PROTO_ETHEREUM=${2:-"$ROOT/proto"}

function main() {
  checks

  set -e

  cd "$ROOT/types/pb" &> /dev/null

  generate "sf/ethereum/transform/v1/transforms.proto"
  generate "sf/ethereum/trxstream/v1/trxstream.proto"
  generate "sf/ethereum/type/v2/type.proto"
  generate "sf/ethereum/substreams/v1/rpc.proto"

  echo "generate.sh - `date` - `whoami`" > ./last_generate.txt
  echo "streamingfast/proto revision: `GIT_DIR=$PROTO/.git git rev-parse HEAD`" >> ./last_generate.txt
  echo "streamingfast/firehose-ethereum/proto revision: `GIT_DIR=$ROOT/.git git log -n 1 --pretty=format:%h -- proto`" >> ./last_generate.txt
}

# usage:
# - generate <protoPath>
# - generate <protoBasePath/> [<file.proto> ...]
function generate() {
    base=""
    if [[ "$#" -gt 1 ]]; then
      base="$1"; shift
    fi

    for file in "$@"; do
      protoc -I$PROTO -I$PROTO_ETHEREUM \
        --go_out=. --go_opt=paths=source_relative \
        --go-grpc_out=. --go-grpc_opt=paths=source_relative,require_unimplemented_servers=false \
         $base$file
    done
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
    echo "  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26.0"
    echo "  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0"
    echo ""
    echo "If everything is working as expetcted, the command:"
    echo ""
    echo "  protoc-gen-go --version"
    echo ""
    echo "Should print 'protoc-gen-go v1.26.0' (if it just hangs, you don't have the correct version)"
    exit 1
  fi
}

main "$@"
