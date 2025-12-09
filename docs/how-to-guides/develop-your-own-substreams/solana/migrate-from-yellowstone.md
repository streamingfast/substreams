# Migrate From Yellowstone to Substreams

## Introduction

Both Substreams and Yellowstone allow you consume Solana data in a fast and reliable way through gRPC connection. However, there are some unique capabilities of Substreams that make it shine:

### Improvements over Yellowstone

- Substreams is a programmable stack, while Yellowstone is only a gRPC interface with the Geyser plugin.
- Substreams gives you access to the full Solana `Block`, and you can use Rust to filter and output the schema that you need.
- Substreams runs on top of a parallelization engine, which speeds up the indexing times.
- Substreams allows you to filter data and create your own output schema (you choose what data model gets ouputted from the Substreams).
- Substreams is a composable stack, which means that you can reuse other Substreams modules built by other people (take a look at the [Substreams Registry](https://substreams.dev)).
- Substreams has native integrations with many _sinks_ (people where you want to consume the data), such as Postgres or Subgraphs. You can also use libraries like Go, Rust or JavaScript.

### Pricing

Yellowstone is usually charged based on credit units. A single response from Yellowstone will cost X credit units.

In Substreams, you are charged depending on the amount of data (TBs) that you consume from the endpoint. Therefore, you will be charge exactly for what the Solana blockchain is producing.

To reduce the cost even more, we have caches of data that will help you consume less data (blocks without voting transactions cache or transactions filtered by program ID cache, for example).

## Examples

In Substreams, you can build your own Substreams modules to filter and output the data that you need.

By default, you can consume the most basic information in Solana (full Blocks, transactions and account changes). In the following examples, you will see different example in differents formats: using the Substreams CLI, consuming the data in JavaScript, or consuming the data in Go.

### Installation

{% tabs %}
{% tab title="CLI" %}
1. Install the Substreams CLI in your computer.
2. Verify that the installation is correct by running:

```bash
substreams --version
```
{% endtab %}

{% tab title="JavaScript (Node)" %}
1. Clone the [Yellostone Examples GitHub repository](https://github.com/enoldev/yellowstone-to-substreams-examples).
2. In the repository, move to the `javascript` folder.
3. Run `npm install` to install all the necessary dependencies.
{% endtab %}
{% endtabs %}

### Get the Full Solana Block

The [https://spkg.io/streamingfast/solana_common-v0.3.3.spkg](https://substreams.dev/packages/solana-common/v0.3.3) Substreams package contains several modules to get the most basic Solana data, such as blocks or transactions. The `blocks_without_votes` module retrieves Solana blocks, removing all the _voting_ transactions.

{% tabs %}
{% tab title="CLI" %}
Run the following command in your terminal:

```bash
substreams gui https://spkg.io/streamingfast/solana_common-v0.3.3.spkg blocks_without_votes --start-block=320100000
```
- `substreams gui` allows you to run a Substreams module and debug its content (move across the content, search, etc).
- `https://spkg.io/streamingfast/solana_common-v0.3.3.spkg` is the Substreams package that extracts the most basic information on Solana.
- `blocks_without_votes` is the module that extracts full Blocks (removing voting transactions).
- `--start-block=320100000` specifies where you want to start consuming data.

You will enter the Substreams GUI view, which will allow you to start the stream and move across blocks.

**IMPORTANT:**
- To start the streaming of data, press the `Enter` key.
- To move across tabs (`Request`, `Output`...), press the `Tab` key.
- To move across blocks, press the `o` and `p` keys.
- To exist the GUI screen, press the `q` key.
{% endtab %}

{% tab title="JavaScript (Node)" %}
```bash
node index.js https://mainnet.sol.streamingfast.io:443 https://spkg.io/streamingfast/solana_common-v0.3.3.spkg blocks_without_votes 320876956
```
- `https://mainnet.sol.streamingfast.io:443 https://spkg.io/streamingfast/`: StreamingFast endpoint for Solana mainnet.
- `https://spkg.io/streamingfast/solana_common-v0.3.3.spkg`: URL of the `solana-common` package in the Substreams Registry.
- `blocks_without_votes`: name of the module you want to execute. This will retrieve Blocks without voting transactions.
- `320876956`: start block of the application.
{% endtab %}
{% endtabs %}

### Get Transactions Filtered by Program ID (Pump.Fun)

The `solana-common` package also allows you to filter transaction by program ID and/or accounts by using the `transactions_by_programid_without_votes` module.

{% tabs %}
{% tab title="CLI" %}
Run the following command in your terminal:

```bash
substreams gui https://spkg.io/streamingfast/solana_common-v0.3.3.spkg transactions_by_programid_without_votes -p "transactions_by_programid_without_votes=program:6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P" --start-block=320100000
```
- `https://spkg.io/streamingfast/solana_common-v0.3.3.spkg` package contains several modules that extract the most basic Solana data.
- `transactions_by_programid_without_votes` is the module that extracts filtered transactions.
- `-p "transactions_by_programid_without_votes=program:6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P"` specifies the parameters passed to the Substreams module. The `transactions_by_programid_without_votes` expects one or several filters to be provided.
In this example, you filter transactions that contain data from the `6EF8rrecthR5Dkzon8Nwu78hRvfCKubJ14M5uBEwF6P` program (Pump Fun program).
{% endtab %}

{% tab title="JavaScript (Node)" %}
```bash
node index.js https://mainnet.sol.streamingfast.io:443 https://spkg.io/streamingfast/solana_common-v0.3.3.spkg blocks_without_votes 320876956
```
- `https://mainnet.sol.streamingfast.io:443 https://spkg.io/streamingfast/`: StreamingFast endpoint for Solana mainnet.
- `https://spkg.io/streamingfast/solana_common-v0.3.3.spkg`: URL of the `solana-common` package in the Substreams Registry.
- `blocks_without_votes`: name of the module you want to execute. This will retrieve Blocks without voting transactions.
- `320876956`: start block of the application.
{% endtab %}
{% endtabs %}

### Get Account Changes History

You can also get the history of an account (with some limitations) using the [solana-accounts-foundational module](https://substreams.dev/packages/solana-accounts-foundational/v0.1.1).

{% tabs %}
{% tab title="CLI" %}
Run the following command in your terminal:

```bash
substreams gui https://spkg.io/streamingfast/solana_accounts_foundational-v0.1.1.spkg filtered_accounts -p "filtered_accounts=account:5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1" --start-block=327404502
```
- `https://spkg.io/streamingfast/solana_accounts_foundational-v0.1.1.spkg` package contains module to filter Solana account changes data.
- `filtered_accounts` is the module that extracts filtered accounts.
- `-p "filtered_accounts=account:5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1"` specifies the parameters passed to the Substreams module. The `filtered_accounts` module expects one or several filters to be provided.
In this example, you filter to only get data from the `5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1` account.
{% endtab %}

{% tab title="JavaScript (Node)" %}
```bash
node index.js https://accounts.mainnet.sol.streamingfast.io:443 https://spkg.io/streamingfast/solana_accounts_foundational-v0.1.1.spkg filtered_accounts 327404502 filtered_accounts=account:5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1
```
- `https://accounts.mainnet.sol.streamingfast.io:443`: StreamingFast endpoint for Account Changes
- `https://spkg.io/streamingfast/solana_accounts_foundational-v0.1.1.spkg`: URL of the `solana-accounts-foundational` module in the Substreams Registry.
- `filtered_accounts`: module of the package that allows you to filter one or several accounts.
- `327404502`: start block of the stream.
- `filtered_accounts=account:5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1`: parameters passed to the module. In this example, you filter on the `5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1` account.
{% endtab %}
{% endtabs %}

<!--Developing Substreams involves three main dependencies: the Rust programming language, the Buf CLI (Protobuf), and the Substreams CLI (a command-line utility tool to manage your Substreams). Follow [this guide](../../cli/installing-the-cli.md) to install all the necessary dependencies in your local computer OR use the [DevContainer](https://github.com/streamingfast/substreams-starter), which contains all the necessary dependencies ready to use. -->

