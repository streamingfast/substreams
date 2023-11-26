---
description: Learn how to authenticate with a Substreams Provider
---

Running a Substreams involves sending your package (`.spkg`) to a a Substreams provider for execution. Usually, Substreams provider will require you to authenticate to avoid abuses on the service.

## Authentication with StreamingFast

### Obtain your API key

First, obtain an API key by visiting our Portal:

* [https://app.streamingfast.io](https://app.streamingfast.io)

The StreamingFast team is also available on [Discord](https://discord.gg/jZwqxJAvRs) to help you get an API key.

### Request your authentication token

Use your API Key to obtain a shorter-lived authentication token using `curl`:

```bash
curl -s https://auth.streamingfast.io/v1/auth/issue --data-binary '{"api_key": "your-secret-key"}'
```

### Set your environment variable

Set the token as an `ENV` variable through the terminal by using:

```bash
export SUBSTREAMS_API_TOKEN="your_token"
```

The `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command checks the `SUBSTREAMS_API_TOKEN` environment variable for the token by default. You can change that with the `--substreams-api-token-envvar` flag.

### All-in-one bash function

Place this function in your terminal profile (`.bashrc` or `.zshrc`), for a quick all-in-one token fetcher:

```bash
export STREAMINGFAST_KEY=server_YOUR_KEY_HERE
function sftoken {
  export SUBSTREAMS_API_TOKEN=$(curl https://auth.streamingfast.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
  echo "Token set on in SUBSTREAMS_API_TOKEN"
}
```

Then obtain a new key and set it in your environment by running:

```bash
$ sftoken
```

[^1]: A [JSON Web Token](https://jwt.io/), or JWT

[^2]: The `api_key` specified here is one starting with `server_`, `web_` or `mobile_`, obtained through the StreamingFast Portal.

[^3]: Install `jq` from [https://stedolan.github.io/jq/](https://stedolan.github.io/jq/)


## Authentication with Pinax

TO DO