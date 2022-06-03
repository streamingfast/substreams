#!/usr/bin/env bash

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"

dry_run=""
force="false"
tag=""

main() {
  pushd "$ROOT" &> /dev/null

  while getopts "hnf" opt; do
    case $opt in
      h) usage && exit 0;;
      n) dry_run="true";;
      f) force="true";;
      \?) usage_error "Invalid option: -$OPTARG";;
    esac
  done
  shift $((OPTIND-1))

  verify_github_token
  verify_keybase

  if [[ "$dry_run" == "true" && "$force" == "true" ]]; then
    usage_error "Only one of -n (dry run) or -f (force) can be provided at a time"
  fi

  version="$1"; shift
  if [[ "$version" == "" ]]; then
    printf "What version do you want to release (current latest is `git describe --tags --abbrev=0`)? "
    read version
  fi

  if [[ ! "$version" =~ ^v ]]; then
    echo "Version $version is invalid, must start with a 'v'"
    exit 1
  fi

  mode="Dry Run, use -f flag to switch to publishing mode"
  if [[ "$force" == "true" ]]; then
    mode="Publishing"
  fi

  # We need to prepare a commit updating the crate's version, so we need change:
  # ^version\s*=\s*"[^"]+" => version = "<version>"
  # substreams-([^ ]+)\s*=\s*\{\s*version\s*=\s*"[^"]+" => substreams-$1 = { version = "<version>"

  echo "About to release version tagged $version ($mode)"
  sleep 3

  # Fails when externally called script error out
  set -e

  if [[ "$force" == "true" ]]; then
    echo "Pushing to ensure GitHub knowns about the latest commit(s)"
    git push
  fi

  args="--rm-dist"
  if [[ "$force" == "false" ]]; then
    args="--skip-publish --skip-validate $args"
  fi

  git tag "$version"
  tag="$version"

  if [[ "$force" == "false" ]]; then
    trap cleanup_tag EXIT
  fi

  goreleaser release $args

  args="${CARGO_PUBISH_ARGS:-}"
  if [[ "$force" == "false" ]]; then
    args="--dry-run $args"
  fi

  # We need to publish one crate at a time, one after the one
  cargo publish $args --target wasm32-unknown-unknown -p substreams-macro

  maybe_wait_publish
  cargo publish $args --target wasm32-unknown-unknown -p substreams
}

cleanup_tag() {
  if [[ "$tag" != "" ]]; then
    git tag -d "$tag"
  fi
}

verify_github_token() {
  if [[ ! -f "$HOME/.config/goreleaser/github_token" && "$GITHUB_TOKEN" = "" ]]; then
    echo "No GitHub token could be found in enviornment variable GITHUB_TOKEN"
    echo "nor at ~/.config/goreleaser/github_token."
    echo ""
    echo "You will need to create one on GitHub website and make it available through"
    echo "one of the accept way mentionned above."
    exit 1
  fi
}

verify_keybase() {
  if ! command keybase &> /dev/null; then
    echo "Keybase is required to sign the release (the checksum of all the artifacts"
    echo "to be precise)."
    echo ""
    echo "You will need to have it available ('brew install keybase' on Mac OS X) and"
    echo "configure it, just setting your Git username and a password should be enough."
    exit 1
  fi
}

maybe_wait_publish() {
  if [[ "$force" == "true" ]]; then
    # We must wait a bit in-between publish lettin enough time for the crates.io registry to
    # correctly records the newly published crate.
    #
    # Without this wait time, the second publish can hit `no matching package named ...` errors
    # because the dependent crate is not "seen" by the registry yet.
    #
    #   Verifying substreams v0.0.8-beta (/Users/maoueh/work/sf/substreams/rust/substreams)
    #   error: failed to verify package tarball
    #
    #   Caused by:
    #     no matching package named `substreams-macro` found
    #     location searched: registry `crates-io`
    #     required by package `substreams v0.0.8-beta (/Users/maoueh/work/sf/substreams/target/package/substreams-0.0.8-beta)`
    sleep 10
  fi
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
  echo "usage: release.sh [-h] [-f] [-n] [<version>]"
  echo ""
  echo "Perform the necessary commands to perform a release of the project."
  echo "The <version> is optional, if not provided, you'll be asked the question."
  echo ""
  echo "The release being performed against GitHub, you need a valid GitHub API token"
  echo "with the necessary rights to upload release and push to repositories. It needs to"
  echo "be provided in file ~/.config/goreleaser/github_token or through an environment"
  echo "variable GITHUB_TOKEN."
  echo ""
  echo "Keybase is required to sign the release (the checksum of all the artifacts"
  echo "to be precise)."
  echo ""
  echo "You will need to have it available ('brew install keybase' on Mac OS X) and"
  echo "configure it, just setting your Git username and a password should be enough."
  echo ""
  echo "Options"
  echo "    -f          Run in write mode publishing the release to GitHub"
  echo "    -n          Run in dry-run mode skipping validation and publishing"
  echo "    -h          Display help about this script"
}

main "$@"