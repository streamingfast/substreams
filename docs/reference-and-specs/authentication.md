---
description: StreamingFast Substreams authentication reference
---

# Authentication

A StreamingFast authentication token is required for connecting to the Substreams server.

An authentication token is a [JSON Web Token ](https://jwt.io/)(JWT) that will grant access to the StreamingFast infrastructure.

First, get a StreamingFast API key. The API key is required for obtaining a token.&#x20;

Sign up at [https://app.dfuse.io/](https://app.dfuse.io/) to get an authentication key. The StreamingFast team is generally available on [Discord](https://discord.gg/jZwqxJAvRs) and can assist with API keys there as well.

_Note, StreamingFast was previously known as **** dfuse._

After obtaining the API key the following command is used to request the authentication token.

```bash
curl https://auth.dfuse.io/v1/auth/issue -s \
    --data-binary \
    '{"api_key":"'$SF_API_KEY'"}' | jq -r .token
```

After obtaining the token it should be set as an ENV variable through the terminal using the following command. _Note, be sure to surround the token in quotes as seen in the code below._&#x20;

```
export SUBSTREAMS_API_TOKEN="your_token"
```

By default the `substreams run` command will check the `SUBSTREAMS_API_TOKEN` environment variable for the StreamingFast authentication token.

{% hint style="info" %}
**Authentication Token Env Flag**

You can change the default behavior of the `substreams run` command and specify your own ENV var name that has the Authentication token with the flag `--substreams-api-token-envvar`
{% endhint %}

The following `bash` function can be run from the command line to obtain a token. The following function can be placed in the `.bashrc` file, located in the computer's home directory.&#x20;

```bash
# Ask us on Discord for a key
export STREAMINGFAST_KEY=server_YOUR_KEY_HERE  
function sftoken {
    export FIREHOSE_API_TOKEN=$(curl https://auth.dfuse.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
	export SUBSTREAMS_API_TOKEN=$FIREHOSE_API_TOKEN
    echo Token set on FIREHOSE_API_TOKEN and SUBSTREAMS_API_TOKEN
}
```

Issue the following command to the terminal to load the key into the `SUBSTREAMS_API_TOKEN` environment variable.

```bash
sftoken
```
