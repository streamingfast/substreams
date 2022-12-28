---
description: StreamingFast Substreams CLI installation
---

# Installing the Substreams CLI

### Substreams CLI installation

The Substreams CLI lets you connect to Substreams endpoints, stream data in real time, and package custom Substreams modules.

{% hint style="info" %}
_**Important**: Check the_ [_official Github repository_](https://github.com/streamingfast/substreams/releases) _to get the **latest Substreams CLI release that is available**._&#x20;
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

### Substreams CLI install validation

Run Substreams passing it the version flag to ensure the Substreams CLI is working as expected.

```bash
substreams --version
substreams version dev
```

{% hint style="info" %}
**Note**: It's possible to use [Gitpod](../developers-guide/installation-requirements.md) for Substreams instead of a local installation.
{% endhint %}
