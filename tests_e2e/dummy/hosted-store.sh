#!/usr/bin/env bash
# Drives a Remote Feed Hosted Store from the command line, so map_query_remote_feed_store
# (in substreams.remote-feed.yaml) has something to read.
#
# The store's gRPC schema is pulled straight from the Buf Schema Registry and combined
# with ./proto, which is where test.output.StoreValue lives -- buf curl consults the
# --schema flags in order, and it needs both to serialize the google.protobuf.Any that
# carries the value.
#
#   export STORE_ID=dep...            # deployment id, also the endpoint hostname
#   export SUBSTREAMS_API_KEY=...     # created with the store
#
#   ./hosted-store.sh seed            # write dummy-key-1 and dummy-key-2
#   ./hosted-store.sh ready           # flip the store ready -- consumers hang until this
#   ./hosted-store.sh get 100         # read them back at block 100
#
# A store is not ready when created. A Substreams querying an unready store does not
# error, it *hangs*, so run `ready` once the keys are in.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

readonly BSR_SCHEMA="buf.build/streamingfast/substreams-foundational-store"
readonly TYPE_URL="type.googleapis.com/test.output.StoreValue"
readonly DEFAULT_KEYS=(dummy-key-1 dummy-key-2)

usage() {
  cat <<'USAGE'
Usage: hosted-store.sh <command> [args]

  seed [key ...]            Feed.Set -- write one StoreValue per key
                            (default: dummy-key-1 dummy-key-2)
  ready [true|false]        Feed.SetReady -- defaults to true
  get <block> [key ...]     Store.Get -- exact key lookup at <block>
  get-first <block> [key ...]
                            Store.GetFirst -- first key >= each requested one

Environment:
  STORE_ID              required, the store's deployment id
  SUBSTREAMS_API_KEY    required, sent as the x-api-key header
  STORE_ENDPOINT        optional, defaults to $STORE_ID.hs.streamingfast.io:443
USAGE
}

require_env() {
  : "${STORE_ID:?STORE_ID must be set to the store deployment id}"
  : "${SUBSTREAMS_API_KEY:?SUBSTREAMS_API_KEY must be set}"
}

endpoint() {
  echo "https://${STORE_ENDPOINT:-${STORE_ID}.hs.streamingfast.io:443}"
}

b64() {
  printf '%s' "$1" | base64 | tr -d '\n'
}

call() {
  local method="$1" body="$2"
  buf curl \
    --schema "$BSR_SCHEMA" \
    --schema ./proto \
    --protocol grpc \
    --header "x-api-key: ${SUBSTREAMS_API_KEY}" \
    --data "$body" \
    "$(endpoint)/${method}"
}

# Each key gets a distinguishable value, so a key/value mix-up is visible in the read back.
cmd_seed() {
  # Assigned before the array is read, so this stays safe under bash 3.2's `set -u`,
  # which errors on ${#empty[@]}.
  local keys=("${@:-}")
  [[ $# -eq 0 ]] && keys=("${DEFAULT_KEYS[@]}")

  local entries="" index=0
  for key in "${keys[@]}"; do
    index=$((index + 1))
    [[ -n "$entries" ]] && entries+=","
    entries+=$(printf '{"key":{"bytes":"%s"},"value":{"@type":"%s","blockNumber":"%d","blockHash":"seeded:%s","transactionCount":"%d"}}' \
      "$(b64 "$key")" "$TYPE_URL" "$index" "$key" "$index")
  done

  call sf.substreams.foundational_store.feed.v2.Feed/Set \
    "{\"entries\":{\"entries\":[${entries}],\"ifNotExist\":false}}"
}

cmd_ready() {
  local ready="${1:-true}"
  call sf.substreams.foundational_store.feed.v2.Feed/SetReady "{\"ready\":${ready}}"
}

cmd_get() {
  local method="$1" block="$2"
  shift 2
  local keys=("${@:-}")
  [[ $# -eq 0 ]] && keys=("${DEFAULT_KEYS[@]}")

  local list=""
  for key in "${keys[@]}"; do
    [[ -n "$list" ]] && list+=","
    list+=$(printf '{"bytes":"%s"}' "$(b64 "$key")")
  done

  call "sf.substreams.foundational_store.service.v2.Store/${method}" \
    "{\"blockNumber\":\"${block}\",\"keys\":[${list}]}"
}

case "${1:-}" in
  seed)
    require_env
    shift
    cmd_seed "$@"
    ;;
  ready)
    require_env
    shift
    cmd_ready "$@"
    ;;
  get)
    require_env
    shift
    [[ $# -ge 1 ]] || { usage >&2; exit 1; }
    cmd_get Get "$@"
    ;;
  get-first)
    require_env
    shift
    [[ $# -ge 1 ]] || { usage >&2; exit 1; }
    cmd_get GetFirst "$@"
    ;;
  ""|-h|--help|help)
    usage
    ;;
  *)
    echo "unknown command: $1" >&2
    usage >&2
    exit 1
    ;;
esac
