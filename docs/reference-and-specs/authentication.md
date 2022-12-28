---
description: StreamingFast Substreams authentication reference
---

# Authentication

### Substreams authentication

A StreamingFast authentication token is required for connecting to the Substreams server.

An authentication token is a [JSON Web Token ](https://jwt.io/)(JWT) that will grant entry into the StreamingFast infrastructure.

### Obtain your API key

The API key is required for obtaining an authentication token. Register for an authentication key at [`app.streamingfast.io`](https://app.streamingfast.io).&#x20;

The StreamingFast team is available on [Discord](https://discord.gg/jZwqxJAvRs) to help you get an API key.

### Request your authentication token

An authentication token must be requested after successfully obtaining the API key.

Request the authentication token using your StreamingFast API key through curl:

```bash
curl -s https://auth.streamingfast.io/v1/auth/issue --data-binary '{"api_key":"your-secret-key"}'
```

### Set your environment variable

The token should be set as an ENV variable through the terminal using the command:&#x20;

```bash
export SUBSTREAMS_API_TOKEN="your_token"
```

{% hint style="success" %}
**Tip**: Surround the token in quotes.&#x20;
{% endhint %}

The `substreams run` command will check the `SUBSTREAMS_API_TOKEN` environment variable for the StreamingFast authentication token by default.

{% hint style="info" %}
**Note**_:_ **Authentication Token Env Flag**

By default, the `substreams run` command uses an environment variable for the authentication token. It's possible to specify a custom environment variable name using the `--substreams-api-token-envvar` flag.
{% endhint %}

### Environment variable script

Run the `bash` function from the command line to obtain a token. The function will be placed in the `.bashrc` file, located in your computer's home directory.&#x20;

```bash
# Ask us on Discord for a key
export STREAMINGFAST_KEY=server_YOUR_KEY_HERE  
function sftoken {
    export FIREHOSE_API_TOKEN=$(curl https://auth.streamingfast.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
    export SUBSTREAMS_API_TOKEN=$FIREHOSE_API_TOKEN
    echo Token set on FIREHOSE_API_TOKEN and SUBSTREAMS_API_TOKEN
}
```

Load the key into the `SUBSTREAMS_API_TOKEN` environment variable using:

```bash
sftoken
```
