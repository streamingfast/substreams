## Substreams Crates Release Instructions

### Instructions

> *Important* Don't forget to replace `${version}` by your real version like `0.1.3` in the commands below!

- Ensure build `cargo build` and tests `cargo test --target aarch64-apple-darwin` (adapt `--target` value to fit your machine's architecture)
- Ensure you are in a clean and pushed Git state
- Find & replace all occurrences of Regex `^version = "[^"]+"` in all `Cargo.toml` files to `version = "${version}"`
- Update the [docs/release-notes/change-log.md](../docs/release-notes/change-log.md) to update the `## Unreleased` header to become `## [${version}](https://github.com/streamingfast/substreams/releases/tag/v${version})`
- Ensure that Keybase is running and your are logged in
- Ensure that `cargo login` has been done in your terminal
- Commit everything with message `Preparing release of ${version}`.
- `./bin/release.sh v${version}` (Add `-f` before `v${version}` to doing the real non-dry mode)
- If everything went well, `git tag ${version}`
- Git push everything (`git push origin develop v${version}`)
