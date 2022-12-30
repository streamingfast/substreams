---
description: StreamingFast Substreams CLI installation documentation
---

# Installing the Substreams CLI

## `substreams` CLI installation

Connect to endpoints, stream data in real time, and package custom modules.

{% hint style="success" %}
**Tip**_:_ Check the [official Github repository](https://github.com/streamingfast/substreams/releases) to get the **latest** [**`substreams` CLI**](../reference-and-specs/command-line-interface.md) **release available**.&#x20;
{% endhint %}

#### Homebrew installation

```
brew install streamingfast/tap/substreams
```

#### Pre-compiled binary installation

```bash
# Use correct binary for your platform
LINK=$(curl -s https://api.github.com/repos/streamingfast/substreams/releases/latest | awk '/download.url.*linux/ {print $2}' | sed 's/"//g')
curl -L  $LINK  | tar zxf -
```

#### Installation from source

```bash
git clone https://github.com/streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```

{% hint style="warning" %}
**Important**: Add $HOME/go/bin to the system path if it's not already present.
{% endhint %}

### Validation of installation

Start Substreams by using the [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) passing the version flag to ensure everything is working as expected.

```bash
substreams --version
substreams version dev # The output displayed in the terminal
```

{% hint style="info" %}
**Note**: You can use [Gitpod](../developers-guide/installation-requirements.md) instead of a local installation.
{% endhint %}
