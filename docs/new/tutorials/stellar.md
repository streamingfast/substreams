Tutorial on Stellar
=================

In this guide, you'll learn how to initialize a Stellar-based Substreams project within the Dev Container.

{% hint style="info" %} 
 If you prefer to begin locally within your terminal rather than through the Dev Container (VS Code required), refer to the [Substreams CLI installation guide](../references/cli/installing-the-cli.md).
{% endhint %}

## Step 1: Initialize Your Stellar Substreams Project

1. Open the [Dev Container](https://github.com/streamingfast/substreams-starter) and follow the on-screen steps to initialize your project.

2. Running `substreams init` will give you the option to choose between two Stellar project options. Select the one that best fits your requirements:
    - **stellar-minimal**: Creates a simple Substreams that extracts raw Stellar block data and generates corresponding Rust code. This path will start you with the full raw block, you can navigate to the `substreams.yaml` (the manifest) to modify the input.
    - **stellar-transactions-operations**: Creates a Substreams that extracts and decodes Stellar trasactions or operations using the cached [Stellar Foundational Module](https://substreams.dev/packages/stellar-foundational/v0.3.0). If you choose the index transactions, you will be able to filter by **source account(s)**. If you choose to index operations, you will be able to filter by **operation name**.

{% hint style="info" %} 
 The first streamable block for Stellar on Substreams is currently 55,411,000.
{% endhint %}

{% hint style="info" %} 
 The `stellar-transactions-operations` foundational module **only decodes and indexes SOME operations**. However, you can [modify the code](https://github.com/streamingfast/substreams-foundational-modules/blob/develop/stellar-common/src/operations.rs#L16) to include the decoding of other operations if needed.
 
 Please, find below the operations supported:
 
```rust
&Op::CreateAccount(_) => "create_account",
&Op::AccountMerge(_) => "account_merge",
&Op::Payment(_) => "payment",
&Op::CreateClaimableBalance(_) => "create_claimable_balance",
&Op::ClaimClaimableBalance(_) => "claim_claimable_balance",
&Op::Clawback(_) => "clawback",
&Op::ClawbackClaimableBalance(_) => "clawback_claimable_balance",
&Op::AllowTrust(_) => "allow_trust",
&Op::SetTrustLineFlags(_) => "set_trust_line_flags",
&Op::LiquidityPoolDeposit(_) => "liquidity_pool_deposit",
&Op::LiquidityPoolWithdraw(_) => "liquidity_pool_withdraw",
&Op::ManageBuyOffer(_) => "manage_buy_offer",
&Op::ManageSellOffer(_) => "manage_sell_offer",
&Op::CreatePassiveSellOffer(_) => "create_passive_sell_offer",
&Op::PathPaymentStrictSend(_) => "path_payment_strict_send",
&Op::PathPaymentStrictReceive(_) => "path_payment_strict_receive",
```
{% endhint %}

## Step 2: Visualize the Data

1. Run `substreams auth` to create your [account](https://thegraph.market/) and generate an authentication token (JWT), then pass this token back as input.

2. Now you can freely use the `substreams gui` to visualize and iterate on your extracted data.

## Step 2.5: (Optionally) Transform the Data 

Within the generated directories, modify your Substreams modules to include additional filters, aggregations, and transformations, then update the manifest accordingly. To learn more about this, visit the [How-to-Guides](../how-to-guides/develop-your-own-substreams/develop-your-own-substreams.md)

## Step 3: Load the Data

To make your Substreams queryable (as opposed to [direct streaming](../how-to-guides/sinks/stream/stream.md)), you can automatically generate a SQL sink.

### SQL

1. Run `substreams codegen sql` and choose from either ClickHouse or Postgres to initialize the sink, producing the necessary files. 
2. Run `substreams build` build the [Substreams:SQL](../how-to-guides/sinks/sql/sql-sink.md) sink. 
3. Run `substreams-sink-sql` to sink the data into your selected SQL DB.

{% hint style="info" %}
**Note**: Run `help` to better navigate the development environment and check the health of containers. 
{% endhint %}

## Additional Resources

You may find these additional resources helpful for developing your first Stellar application.

### Dev Container Reference

The [Dev Container Reference](../references/devcontainer-ref.md) helps you navigate the container and its common errors. 

### CLI Reference

The [CLI reference](../references/cli/command-line-interface.md) lets you explore all the tools available in the Substreams CLI.

### Substreams Components Reference

The [Components Reference](../references/substreams-components/) dives deeper into navigating the `substreams.yaml`.
