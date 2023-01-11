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

  # We do not sign releases anymore because they are done in a Docker env now
  # so some adaptation is required
  #verify_keybase

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

  package_name="github.com/streamingfast/substreams"
  golang_cross_version="v1.19.4"

  # We have no customized sysroot, so nothing to link now
  #-v "`pwd`/sysroot:/sysroot" \

	docker run \
		--rm \
		-e CGO_ENABLED=1 \
    --env-file .env.release \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v "`pwd`:/go/src/${package_name}" \
		-w "/go/src/${package_name}" \
		"goreleaser/goreleaser-cross:${golang_cross_version}" \
		$args
}

cleanup_tag() {
  if [[ "$tag" != "" ]]; then
    git tag -d "$tag"
  fi
}

verify_github_token() {
  release_env_file="$ROOT/.env.release"

  if [[ "$GITHUB_TOKEN" != "" && ! -f "$release_env_file" ]]; then
    echo 'GITHUB_TOKEN=${GITHUB_TOKEN}' > "$release_env_file"
  fi

  if [ ! -f "$ROOT/.env.release" ] || ! grep -q "GITHUB_TOKEN=" "$release_env_file"; then
    echo "A '.env.release' file must be found at the root of the project and it must contain"
    echo "definition of 'GITHUB_TOKEN' variable. You need to create this file locally and the"
    echo "content should be:"
    echo ""
    echo "GITHUB_TOKEN=<your_github_token>"
    echo ""
    echo "You will need to create your own GitHub Token on GitHub website and make it available through"
    echo "the file mentioned above."

    if [[ -f "$ROOT/.env.release" ]]; then
      echo ""
      echo "Actual content of '$release_env_file' is:"
      echo ""
      cat "$release_env_file"
    fi

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