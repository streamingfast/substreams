---
description: StreamingFast Substreams CLI installation documentation
---

# Installing the Substreams CLI

## Install the `substreams` CLI

Used for connecting to endpoints, streaming data in real time, and packaging custom modules.

{% hint style="success" %}
**Tip**_:_ [Check the official Github repository](https://github.com/streamingfast/substreams/releases) to get the **latest** [**`substreams` CLI**](../reference-and-specs/command-line-interface.md) **release available**.
{% endhint %}

### Homebrew installation

```
brew install streamingfast/tap/substreams
```

### Pre-compiled binary installation

```bash
# Use correct binary for your platform
LINK=$(curl -s https://api.github.com/repos/streamingfast/substreams/releases/latest | awk '/download.url.*linux/ {print $2}' | sed 's/"//g')
curl -L  $LINK  | tar zxf -
```

### Installation from source

```bash
git clone https://github.com/streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```

{% hint style="warning" %}
**Important**: Add $HOME/go/bin to the system path if it's not already present.
{% endhint %}

## Validation of installation

Run the [`substreams` CLI](../reference-and-specs/command-line-interface.md) passing the `--version` flag to check the success of the installation.

```bash
substreams --version
```

A successful installation will print the version that you have installed.

```bash
substreams version dev
```

{% hint style="info" %}
**Note**: You can [also use Gitpod](../developers-guide/installation-requirements.md) instead of a local installation.
{% endhint %}
