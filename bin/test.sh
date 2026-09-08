#!/usr/bin/env bash

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"

export SUBSTREAMS_INTEGRATION_TESTS=true

main() {
  pushd "$ROOT" &> /dev/null

  while getopts "h" opt; do
    case $opt in
      h) usage && exit 0;;
      \?) usage_error "Invalid option: -$OPTARG";;
    esac
  done
  shift $((OPTIND-1))

  set -e

  go test ./... "$@"

  # tests_e2e is its own Go module, so `go test ./...` above never reaches it. It stays
  # out of this script until it passes on Linux: the node writes root-owned files into
  # the bind-mounted t.TempDir, which fails every container test's cleanup on a CI
  # runner. See the fix/tests-e2e-on-linux branch.
  #
  # pushd tests_e2e &> /dev/null
  # go test ./... "$@"
  # popd &> /dev/null
}

usage_error() {
  message="$1"
  exit_code="$2"

  echo "ERROR: $message"
  echo ""
  usage
  exit ${exit_code:-1}
}

usage() {
  echo "usage: test.sh"
  echo ""
  echo "Runs the Go tests in all sub-packages of this repository. Normal 'go test'"
  echo "does run test just under the current Go package, sub-package are not traversed."
  echo ""
  echo "Options"
  echo "    -h          Display help about this script"
}

main "$@"
