# Authentication

To connect to the Substreams server you will need to get a StreamingFast authentication token. The authentication token is essentially a [JWT](https://jwt.io/) that will give you access to the StreamingFast infrastructure.

The first step is to get a StreamingFast API key that will allow you to get a token. You can sign up at [https://app.dfuse.io/](https://app.dfuse.io/) to get a key or you can [ask us in discord](https://discord.gg/jZwqxJAvRs).

{% hint style="info" %}
**dfuse** was a prior brand of **StreamingFast**. This app will be updated soon.
{% endhint %}

Once you have your key, you can use this one-liner to get your token:

```bash
curl https://auth.dfuse.io/v1/auth/issue -s \
    --data-binary \
    '{"api_key":"'$SF_API_KEY'"}' | jq -r .token
```

Once you have obtained a token, you should set it as an ENV variable:

```
export SUBSTREAMS_API_TOKEN="your_token"
```

The `substreams run` command will check by default the `SUBSTREAMS_API_TOKEN` environment variable for your StreamingFast authentication token.

{% hint style="info" %}
**Authentication Token Env Flag**

You can change the default behavior of the `substreams run` command and specify your own ENV var name that has the Authentication token with the flag `--substreams-api-token-envvar`
{% endhint %}

We suggestion you setup the following `bash` function that you can call to get a token. Dump this function somewhere like `.bashrc`:

```bash
# Ask us on Discord for a key
export STREAMINGFAST_KEY=server_YOUR_KEY_HERE  
function sftoken {
    export FIREHOSE_API_TOKEN=$(curl https://auth.dfuse.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token)
	export SUBSTREAMS_API_TOKEN=$FIREHOSE_API_TOKEN
    echo Token set on FIREHOSE_API_TOKEN and SUBSTREAMS_API_TOKEN
}
```

Then in your shell, load a key into an environment variable with:

```bash
sftoken
```
