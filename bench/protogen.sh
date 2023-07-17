#!/usr/bin/env bash

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

main() {
  cd "$ROOT" &> /dev/null

  while getopts "h" opt; do
    case $opt in
      h) usage && exit 0;;
      \?) usage_error "Invalid option: -$OPTARG";;
    esac
  done
  shift $((OPTIND-1))

  files=`find . -type f -name "protogen.json"`

  for item in ${files}; do
    input=`cat "$item" | jq -r .input`
    exclude_paths=`cat "$item" | jq -r '.excludePaths | join(",")' 2> /dev/null`
    directory=`dirname ${item}`

    pushd ${directory} &> /dev/null
      echo "Generating in ${directory}"
      if [[ ! -f "buf.gen.yaml" ]]; then
        echo "ERROR: buf.gen.yaml file not found in ${directory}"
        exit 1
      fi

      substreams protogen --exclude-paths "$exclude_paths"
      # buf generate "$input"

      # for delete_path in ${delete_paths}; do
      #   rm -rf "$delete_path" &> /dev/null
      # done
    popd &> /dev/null
  done
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
  echo "usage: protogen.sh <test>"
  echo ""
  echo "Options"
  echo "    -h          Display help about this script"
}

main "$@"