---
description: Installation instructions for the `substreams` command-line interface.
---

# Installing the CLI

{% hint style="success" %}
Alternatively to installing the `substreams` locally, you can [use Gitpod to get started quickly](../developer-guide/installation-requirements.md).
{% endhint %}

### Installing the `substreams` command-line interface

The `substreams` CLI allows you to interact with Substreams endpoints, stream data in real-time, as well as package your own Substreams modules.

#### From brew (for Mac OS)

```
brew install streamingfast/tap/substreams
```

#### From pre-compiled binary

Download the binary

```bash
# Use correct binary for your platform
LINK=$(curl -s https://api.github.com/repos/streamingfast/substreams/releases/latest | awk '/download.url.*linux/ {print $2}' | sed 's/"//g')
curl -L  $LINK  | tar zxf -
```

{% hint style="info" %}
Check [https://github.com/streamingfast/substreams/releases](https://github.com/streamingfast/substreams/releases) and use the latest release available
{% endhint %}

#### From Source

```bash
git clone git@github.com:streamingfast/substreams
cd substreams
go install -v ./cmd/substreams
```

### Validation

Ensure that `substreams` CLI works as expected:

```bash
substreams --version
substreams version 0.0.12 (Commit 7b30088, Built 2022-06-03T18:32:00Z)
```
