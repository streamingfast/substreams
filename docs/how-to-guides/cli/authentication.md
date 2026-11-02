# Substreams Authentication Guide

This guide explains how to authenticate when running a Substreams package (`.spkg`) with a provider, specifically using The Graph Market.

## Overview

`substreams auth` opens a browser so you can pick an organization (if you belong to more than one) and an API key. The CLI retrieves the selected key over the API — you do not copy and paste it — and writes it to `.substreams.env` as `SUBSTREAMS_API_KEY`. The key is sent as-is on every request; no token is issued and nothing expires or needs refreshing.

## Prerequisites

- The [Substreams CLI](./installing-the-cli.md) installed.
- A browser, so you can sign in and pick a key.
- An account with [The Graph Market](https://thegraph.market), or be ready to create one during login.

## Step 1: Run login

```bash
substreams auth
```

The command prints an approval URL (and tries to open it in your browser). Sign in, choose an organization if asked, then select an API key.

On success it writes `.substreams.env` with `SUBSTREAMS_API_KEY`. Add that file to `.gitignore`.

## Step 2: Load the credentials

```bash
. ./.substreams.env
```

Other `substreams` commands also read `.substreams.env` automatically when it is present next to the manifest (or in the current directory).

## Paste an API key instead

If you already have a Graph Market `server_` API key:

```bash
substreams auth --paste
```

Paste the key when prompted; it is written to `.substreams.env` like above. You can also set it yourself:

```bash
export SUBSTREAMS_API_KEY="<YOUR-API-KEY>"
```

{% hint style="warning" %}
**Already have `SUBSTREAMS_API_TOKEN` set?** If you don't know why you use an API token (JWT) instead of an API key, and only set it because you followed previous versions of this guide, migrate to the API key: see [Migrating from API Token to API Key](../../references/migrating-to-api-key.md). A JWT is short-lived and is not refreshed by the CLI, so it stops working once it expires.

JWTs are meant for advanced use cases only, see the [Authentication Reference](../../references/authentication.md). When both `SUBSTREAMS_API_KEY` and `SUBSTREAMS_API_TOKEN` are set, the API key is used.
{% endhint %}

## Step 3: Verify authentication

Run a test Substreams against Ethereum Mainnet:

```bash
substreams gui ethereum-common@v0.3.1 all_events --start-block=15000000
```

If the stream starts without an authentication error, your credentials are in place.

## Learn More

- [Authentication Reference](../../references/authentication.md): API key vs JWT, when to use each, issuing and handling tokens.
- [Migrating from API Token to API Key](../../references/migrating-to-api-key.md)

## Need Help?

If you encounter any issues or have questions, the StreamingFast team is available on [Discord](https://discord.gg/jZwqxJAvRs) to assist you.
