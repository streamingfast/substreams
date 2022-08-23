#!/usr/bin/env bash

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd ../../.. && pwd )"

option=""

main() {
  pushd "$ROOT" &> /dev/null

  while getopts "ho:" opt; do
    case $opt in
      h) usage && exit 0;;
      o) option="$OPTARG";;
      \?) usage_error "Invalid option: -$OPTARG";;
    esac
  done
  shift $((OPTIND-1))

  set -e

  substreams pack manifest/testdata/spkg1/spkg1.yaml
  mv spkg1-v0.0.0.spkg manifest/testdata/spkg1/
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
  echo "usage: build.sh <all>"
  echo ""
  echo "Generates the .spkg files from their manifest."
  echo ""
  echo "Options"
  echo "    -h          Display help about this script"
}

main "$@"