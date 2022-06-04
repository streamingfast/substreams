# Overview

In the upcoming guide, we will attempt to build a Substream that tracks ERC721 holder count for a given contract.&#x20;

{% hint style="info" %}
You can find the accompanying repository here: [https://github.com/streamingfast/substreams-template](https://github.com/streamingfast/substreams-template)
{% endhint %}

### Gitpod Quick Start

If you want to just get up and running and not go through the detailed steps, you can follow this Quickstart guide. Use these steps to conveniently open your repository in a Gitpod.

1. First, [copy this repository](https://github.com/streamingfast/substreams-template/generate).
2. Grab a StreamingFast key from [https://app.dfuse.io/](https://app.dfuse.io/)
3. Create a [Gitpod](https://gitpod.io/) account
4. Configure a `STREAMINGFAST_KEY` variable in your Gitpod account settings
5. Open your repository as a [Gitpod workspace](https://gitpod.io/workspaces)

### Developer Guide

We have broken down the guide into a few steps:

1. [Installation](installation-requirements.md): Install all the dependencies and setup your environment to create your first Substreams
2. [Creating Your Manifest](creating-your-manifest.md): Setup your first `substreams.yaml` which gives you a high-level overview of the file
3. [Creating Protobuf Schemas](creating-protobuf-schemas.md): Write your first Protobuf schema that will be used in your handlers
4. [Writing Module Handlers](writing-module-handlers.md): Write your first module handler
5. [Running your Substreams](running-substreams.md): Run your newly written Substreams
