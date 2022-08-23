#!/usr/bin/env bash

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"


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

  pushd types &> /dev/null
    go test ./... "$@"
  popd&> /dev/null

  go test ./... "$@"
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