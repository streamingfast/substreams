## Substreams CLI & Lib Release

## Instructions

> *Important* Do not forget to replace `${version}` by your real version like `v0.0.21` in the commands below!

Ensure you have [sfreleaser](https://github.com/streamingfast/sfreleaser) CLI, install with using Go with `go install github.com/streamingfast/sfreleaser/cmd/sfreleaser@latest`.

### Preparing for a release

- Ensure tests past `go test ./...`
- Ensure you are in a clean and pushed Git state
- Update the [./docs/release-notes/change-log.md](./docs/release-notes/change-log.md) to update the `## Unreleased` header to become `## [${version}](https://github.com/streamingfast/substreams/releases/tag/v${version})`
- Commit everything with message `Preparing release of ${version}`.

## Generating a draft release

Run `sfreleaser release` it's going to ask you questions about the release, just answer them. If you are missing some dependencies, the tool will also give you instructions to install them

The `release` command publishes in `draft` mode by default, it will ask you to preview you it and also ask if you want to publish it right away.
