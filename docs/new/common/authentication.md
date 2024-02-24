---
description: StreamingFast Substreams authentication reference
---

# Authentication

Running a Substreams involves sending your package (`.spkg`) to a a Substreams provider for execution. Usually, Substreams provider will require you to authenticate to avoid abuses on the service.

### Authentication with StreamingFast

Authentication can be done either directly from an API key or from a derived JWT.

#### Get your API key

First, obtain an API key by visiting our Portal:

* [https://app.streamingfast.io](https://app.streamingfast.io)

The StreamingFast team is also available on [Discord](https://discord.gg/jZwqxJAvRs) to help you.

#### Authenticate with your API key

The substreams server expects the `X-Api-Key` header to be set with your API key. Here's how you do it in the terminal:

Set the token as an `ENV` variable through the terminal by using:

```bash
export SUBSTREAMS_API_KEY="server_0123456789abcdef0123456789abcdef"
```

The `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) and [`gui`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#gui) commands check the `SUBSTREAMS_API_KEY` environment variable for the key by default. You can change that with the `--substreams-api-key-envvar` flag.

#### Authentication using a JWT (optional)

Streamingfast also provides a way to generate a token (JWT) from your API key and use it as authentication. The advantage of that method is that you can manage the JWT expiration, more suitable for using in web apps or tighter security standards.

#### Request your authentication token

Use your API Key to obtain an authentication token using `curl`:

```bash
# lifetime is the token duration in seconds
curl -s https://auth.streamingfast.io/v1/auth/issue --data-binary '{"api_key": "your-api-key", "lifetime": 3600}'
```

#### Use it in your requests

The substreams server expects the standard `Authorization: bearer your_api_key` header format for JWT-based authentication. Here's how you do it in the terminal:

```bash
export SUBSTREAMS_API_TOKEN="your_token"
```

The `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) and [`gui`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#gui) commands check the `SUBSTREAMS_API_TOKEN` environment variable for the token by default. You can change that with the `--substreams-api-token-envvar` flag.

#### All-in-one bash function

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
