---
description: StreamingFast Substreams authentication reference
---

# Authentication

Running a Substreams involves sending your package (`.spkg`) to a a Substreams provider for execution. Usually, the Substreams provider will require you to authenticate to avoid abuses on the service.

## Authentication with The Graph Market

You can directly authenticate with The Graph Market (https://thegraph.market) by creating a new API key and obtaining the corresponding JWT token. 

### Get the JWT Token

1. Navigate to the [https://thegraph.market](https://thegraph.market). Log in your account or create a new account if you don't have one.

2. Click on `Dashboard` or nagivate to [https://thegraph.market/dashboard](https://thegraph.market/dashboard).

<figure><img src="../../.gitbook/assets/intro/thegraphmarket.png" width="100%" /></figure>

3. Then, click on `Create New Key`.
You will be provided with a JWT token. Copy and save this token in a safe place.

### Use the JWT Token

Once you have your JWT token, you can you it to consume data in Substreams. By default, the Substreams execution will inspect the `SUBSTREAMS_API_TOKEN` environment variable in your computer looking for a valid JWT token.

Setting the local environment variable might vary depending on your operating system. For Unix-like operating systems (Mac and Linux):

1. Open a command-line terminal.

2. Run the following command.

```bash
export SUBSTREAMS_API_TOKEN="<YOUR-JWT-TOKEN>"
```

Just replace `<YOUR-JWT-TOKEN>` with the actual JWT token.

### Verify the Authentication Works

To verify that everything works correctly, you can run a pre-built Substreams. In the following example, you run the [ERC20 Balance Changes Substreams](https://substreams.dev/streamingfast/erc20-balance-changes/) against the Ethereum Mainnet endpoint (`mainnet.eth.streamingfast.io:443`) of StreamingFast.

```bash
substreams gui \
  https://spkg.io/streamingfast/erc20-balance-changes-v1.2.0.spkg \
  map_valid_balance_changes \
  -e mainnet.eth.streamingfast.io:443 \
  --start-block 1397553
  --stop-block +10
```

The StreamingFast team is also available on [Discord](https://discord.gg/jZwqxJAvRs) to help you.
