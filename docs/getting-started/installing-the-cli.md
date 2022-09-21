---
description: StreamingFast Substreams CLI installation
---

# Installing the CLI

### CLI Installation

The Substreams command line interface (CLI) is the main means of interacting with and using the product.

The CLI allows you to connect to Substreams endpoints, stream data in real time, and package custom Substreams modules.

#### Homebrew Installation

```
brew install streamingfast/tap/substreams
```

#### Pre-compiled Binary Installation

```bash
# Use correct binary for your platform
LINK=$(curl -s https://api.github.com/repos/streamingfast/substreams/releases/latest | awk '/download.url.*linux/ {print $2}' | sed 's/"//g')
curl -L  $LINK  | tar zxf -
```

{% hint style="warning" %}
Check [https://github.com/streamingfast/substreams/releases](https://github.com/streamingfast/substreams/releases) and use the latest release available
{% endhint %}

#### Substreams Source Installation

```bash
git clone git@github.com:streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```

### CLI Install Validation

Run Substreams passing it the version flag to ensure the CLI is working as expected.

```bash
substreams --version
substreams version 0.0.20 (Commit dae1548, Built 2022-08-23T18:35:46Z)
```

_Note,_ [_Gitpod_](../developer-guide/installation-requirements.md) _can also be used for Substreams rather than local installations._
