#!/usr/bin/env bash

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"

dry_run=""
force="false"

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

  echo "About to release version tagged $version ($mode)"
  sleep 3

  if [[ "$force" == "true" ]]; then
    echo "Pushing to ensure GitHub knowns about the latest commit(s)"
    git push
  fi

  args="--rm-dist"
  if [[ "$force" == "false" ]]; then
    args="--skip-publish --skip-validate $args"
  fi

  set -e
  git tag "$version"
  set +e

  goreleaser release $args
  if [[ $? -gt 0 || "$force" == "false" ]]; then
    git tag -d "$version"
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