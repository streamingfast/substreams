---
description: StreamingFast Substreams authentication reference
---

# Authentication

## Substreams authentication overview

A StreamingFast authentication token is required for connecting to the Substreams server.

An authentication token is a [JSON Web Token ](https://jwt.io/)(JWT) responsible for granting entry into the StreamingFast infrastructure.

### Obtain your API key

The API key is required for requesting an authentication token. Obtain one by visiting our Portal:

* [https://app.streamingfast.io](https://app.streamingfast.io)

The StreamingFast team is also available on [Discord](https://discord.gg/jZwqxJAvRs) to help you get an API key.

### Request your authentication token

Request the a token with `curl`:

<pre class="language-bash" data-overflow="wrap"><code class="lang-bash">curl -s https://auth.streamingfast.io/v1/auth/issue --data-binary '{<a data-footnote-ref href="#user-content-fn-1">"api_key"</a>: "your-secret-key"}'
</code></pre>

### Set your environment variable

Set the token as an `ENV` variable through the terminal by using:&#x20;

```bash
export SUBSTREAMS_API_TOKEN="your_token"
```

The `substreams` [`run`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#run) command checks the `SUBSTREAMS_API_TOKEN` environment variable for the token by default. You can change that with the `--substreams-api-token-envvar` flag.

### All-in-one bash function

Place this function in your terminal profile (`.bashrc` or `.zshrc`), for a quick all-in-one token fetcher:&#x20;

<pre class="language-bash" data-overflow="wrap"><code class="lang-bash">export STREAMINGFAST_KEY=server_YOUR_KEY_HERE  
function sftoken {
    export SUBSTREAMS_API_TOKEN=$(curl https://auth.streamingfast.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | <a data-footnote-ref href="#user-content-fn-2">jq</a> -r .token)
    echo "Token set on in SUBSTREAMS_API_TOKEN"
}
</code></pre>

Then obtain a new key and set it in your environment by running:

```bash
$ sftoken
```

[^1]: The `api_key` specified here is one starting with `server_`, `web_` or `mobile_`, obtained through the StreamingFast Portal.

[^2]: Install `jq` from [https://stedolan.github.io/jq/](https://stedolan.github.io/jq/)
