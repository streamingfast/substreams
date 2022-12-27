---
description: StreamingFast Substreams CLI installation
---

# Installing the CLI

### CLI installation

The Substreams command line interface (CLI) is the main means of interacting with and using the product.

The CLI lets you connect to Substreams endpoints, stream data in real-time, and package custom Substreams modules.

{% hint style="info" %}
_**Important**: Check the_ [_official Github repository_](https://github.com/streamingfast/substreams/releases) _to get the **latest CLI release available**._&#x20;
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

#### Substreams source installation

```bash
git clone https://github.com/streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```

{% hint style="warning" %}
**Important**: Add $HOME/go/bin to the system path if it's not already present.
{% endhint %}

### CLI install validation

Run Substreams passing it the version flag to ensure the CLI is working as expected.

```bash
substreams --version
substreams version dev
```

{% hint style="info" %}
_Note:_ [_Gitpod_](../developer-guide/installation-requirements.md) _can also be used for Substreams rather than local installations._
{% endhint %}
