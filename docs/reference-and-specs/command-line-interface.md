---
description: StreamingFast Substreams command line interface (CLI)
---

# Substreams CLI reference

The Substreams command line interface (CLI) is the user interface and central access point for working with Substreams.

The Substreams CLI exposes many commands to developers enabling a range of features. Each command is explained in further detail.

{% hint style="info" %}
**Note**: any time a package is specified any of the following can be used, local `substreams.yaml` file, local `.spkg` or a remote `.spkg` URL.
{% endhint %}

### **`run`**

The `run` command connects to a Substreams endpoint and begins processing data.

```
substreams run -e mainnet.eth.streamingfast.io:443 \
   -t +1 \
   ./substreams.yaml \
   module_name
```

#### Run Command breakdown

* `-e mainnet.eth.streamingfast.io:443` is the endpoint of the provider running our Substreams
* `-t +1` (or `--stop-block`) only requests a single block (stop block will be manifest's `initialBlock` + 1)
* `substreams.yaml` is the path where we have defined our [Substreams Manifest](https://github.com/streamingfast/substreams-docs/blob/master/docs/guides/docs/reference/manifests.html). This can be an `.spkg` or a `substreams.yaml` file.
* `module_name` is the module we want to run, referring to the `name` [defined in the manifest](manifests.md#modules-.name).

Passing a different `-s` (or `--start-block`) will run any prior modules at high speed, to provide you with output at the requested start block as fast as possible, while keeping snapshots along the way, in case you want to process it again.

Example output of `gravatar_updates` starting at block 6200807.

```
$ substreams run -e mainnet.eth.streamingfast.io:443 \
    https://github.com/Jannis/gravity-substream/releases/download/v0.0.1/gravity-v0.1.0.spkg \
    gravatar_updates -o json
{
  "updates": [
    {
      "id": "39",
      "owner": "0xaadcc13071fdf9c73cfbb8d97639ea68aa6fd1d2",
      "displayName": "alex | OpenSea",
      "imageUrl": "https://ucarecdn.com/13a67247-cb89-417a-92d2-50a7d7aa481c/-/crop/382x382/0,0/-/preview/"
    }
  ]
}
...
```

Notice the `-o` (or `--output`), that can alter the output format. The options are:

* `ui`, a nicely formatted, UI-driven interface, with progress information, and execution logs.
* `json`, an indented stream of data, with no progress information nor logs, but just data output for blocks following the start block.
* `jsonl`, same as `json` but with each output on a single line.

### `pack`

The `pack` command builds a shippable, importable package from a `substreams.yaml` manifest file.

```bash
$ substreams pack ./substreams.yaml
...
Successfully wrote "your-package-v0.1.0.spkg".
```

### `info`

The `info` command prints out the contents of a package for inspection. It works on both local and remote `yaml` or `spkg` files.

```bash
$ substreams info ./substreams.yaml
Package name: solana_spl_transfers
Version: v0.5.2
Doc: Solana SPL Token Transfers stream

  This streams out SPL token transfers to the nearest human being.

Modules:
----
Name: spl_transfers
Initial block: 130000000
Kind: map
Output Type: proto:solana.spl.v1.TokenTransfers
Hash: 2b59e4e840f814f4154a688c2935da9c3b61dc61

Name: transfer_store
Initial block: 130000000
Kind: store
Value Type: proto:solana.spl.v1.TokenTransfers
Update Policy: UPDATE_POLICY_SET
Hash: 11fd70768029bebce3741b051c15191d099d2436
```

### `graph`

The `graph` command prints out a visual graph of the package in the _mermaid-js_ format.

{% hint style="info" %}
_Note: see_ [_https://mermaid.live/_](https://mermaid.live/) _for a live mermaid-js editor._
{% endhint %}

````bash
$ substreams graph ./substreams.yaml                         [±master ●●]
Mermaid graph:

```mermaid
graph TD;
  spl_transfers[map: spl_transfers]
  sf.solana.type.v1.Block[source: sf.solana.type.v1.Block] --> spl_transfers
  transfer_store[store: transfer_store]
  spl_transfers --> transfer_store
```
````

The code will a graphic similar to

{% embed url="https://mermaid.ink/svg/pako:eNp1kMsKg0AMRX9Fsq5Ct1PootgvaHeOSHBilc6LeRRE_PeOUhe2dBOSm5NLkglaIwgYPBzaPruXJ66zzFvZBIfad-R8pdCyvVSvUFd4I1FjEUZLxetYXKRpn5U30bXE_vXrLM_Pe7vFbSsaH4yjao3sS61_dlu99tDCwAEUOYWDSJdNi8Ih9KSIA0upoA6jDBy4nhMarcBAVzGkcWAdSk8HwBjMbdQtsOAibVA5YHqU-lDzG43ick8" %}
Open the link and change ".ink/svg/" to ".live/edit#" in the URL, to go back to edit mode.
{% endembed %}

### `inspect`

This command goes deep into the file structure of a package (`yaml` or `spkg`). The `inspect` command is used mostly for debugging, _or for the curious ;)_

```
$ substreams inspect ./substreams.yaml | less
proto_files {
...
modules {
  modules {
    name: "my_module_name"
...
```

### Help

The commands and a brief explanation are also provided in the Substreams CLI application. To view the help reference at any time, simply execute the `substreams` command in a terminal and pass an `-h` flag.

{% code title="Substreams help" overflow="wrap" %}
```
Usage:
  substreams [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  decode
  graph       Generate mermaid-js graph document
  help        Help about any command
  info        Display package modules and docs
  inspect     Display low-level package structure
  pack        Build an .spkg out of a .yaml manifest
  protogen    Generate Rust bindings from a package
  run         Stream modules from a given package on a remote endpoint
  tools       Developer tools related to substreams

Flags:
  -h, --help      help for substreams
  -v, --version   version for substreams

Use "substreams [command] --help" for more information about a command.
```
{% endcode %}
