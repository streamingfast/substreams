# Change log

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]


## [v0.0.5-beta]

First chain-agnostic release. THIS IS BETA SOFTWARE. USE AT YOUR OWN RISK.
WE PROVIDE NO BACKWARDS COMPATIBILITY GUARANTEES FOR THIS RELEASE.

See https://github.com/streamingfast/substreams for usage docs.

* Removed `local` command. See README.md for instructions on how to run locally now.  Build `sfeth` from source for now.
* Changed the `remote` command to `run`.
* Changed `run` command's  `--substreams-api-key-envvar` flag to ``--substreams-api-token-envvar`, and its default value is changed from `SUBSTREAMS_API_KEY` to `SUBSTREAMS_API_TOKEN`. See README.md to learn how to obtain such tokens.
