# Running Your Substreams

We're now ready to run our example Substream!

Lets first build our Substreams

```
cargo build --target wasm32-unknown-unknown --release
```

To connect to the Substreams server you will need to get a StreamingFast authentication token. We will create a helpful `bash` function that you can call to get a token. Dump this function somewhere like `.bashrc`:

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

{% hint style="info" %}
**Obtaining API Token**

This token is used to access StreamingFast's infrastructure. You first need to get a StreamingFast key, you can [ask us in discord](https://discord.gg/jZwqxJAvRs).

You can use this oneline to get your token



`curl https://auth.dfuse.io/v1/auth/issue -s --data-binary '{"api_key":"'$STREAMINGFAST_KEY'"}' | jq -r .token`



Once you obtained a token, you should set it in an ENVIRONMENT variable

``

`export SUBSTREAMS_API_TOKEN="your_token"`
{% endhint %}

You can now run your Substreams

```
substreams run -e api-dev.streamingfast.io:443 \
   substreams.yaml \
   block_to_transfers \
   --start-block 12292922 \
   --stop-block +1
```

```bash
substreams run -p -e localhost:9000 substream.yaml block_to_transfers --start-block 12370550 --stop-block +1Let's break down everything happening above.
```

* `substreams` is our executable
* `-e api-dev.streamingfast.io:443` is the provider going to run our Substreams
* `substream.yaml` is the path where we have defined our Substreams Manifest
* `block_to_transfers` this is the module which we want to run, defined in the manifest
* `--start-block 12292922` start from block `12292922`
* `--stop-block +1` only request a single block (stop block will be manifest's start block + 1)

When you run the command you should get the following output:

```bash
 substreams run -e api-dev.streamingfast.io:443 \
   substreams.yaml \
   block_to_transfers \
   --start-block 12292922 \
   --stop-block +1
2022-05-30T10:52:27.256-0400 INFO (substreams) connecting...
2022-05-30T10:52:27.389-0400 INFO (substreams) connected

----------- IRREVERSIBLE BLOCK #12,292,922 (12292922) ---------------
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: log: NFT Contract bc4ca0eda7647a8ab7c2061c2e118a18a936f13d invoked
block_to_transfers: message "eth.erc721.v1.Transfers": {
  "transfers": [
    {
      "from": "AAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      "to": "q6cWGn+2nIjhbtn0Vc5it5HuTQM=",
      "trxHash": "z7GX9i7Fx/DnGhHsDEoOOUo6pB21OG6FUm+GyEs/J5Y=",
      "ordinal": "85"
    },
    {
      "from": "AAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      "to": "q6cWGn+2nIjhbtn0Vc5it5HuTQM=",
      "tokenId": "1",
      "trxHash": "z7GX9i7Fx/DnGhHsDEoOOUo6pB21OG6FUm+GyEs/J5Y=",
      "ordinal": "86"
    },
    {
      "from": "AAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      "to": "q6cWGn+2nIjhbtn0Vc5it5HuTQM=",
      "tokenId": "2",
      "trxHash": "z7GX9i7Fx/DnGhHsDEoOOUo6pB21OG6FUm+GyEs/J5Y=",
      "ordinal": "87"
    },
    ...
    {
      "from": "AAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      "to": "q6cWGn+2nIjhbtn0Vc5it5HuTQM=",
      "tokenId": "29",
      "trxHash": "z7GX9i7Fx/DnGhHsDEoOOUo6pB21OG6FUm+GyEs/J5Y=",
      "ordinal": "114"
    }
  ]
}
```

From the output above we can see we have 30 token transfers, we can confirm this by verifying on [etherscan](https://etherscan.io/tx/0xcfb197f62ec5c7f0e71a11ec0c4a0e394a3aa41db5386e85526f86c84b3f2796).
