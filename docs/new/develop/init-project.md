
Getting started with Substreams is very easy! Depending on the blockchain that you want to use, the best way to get started might change:

{% tabs %}
{% tab title="Solana" %}
If you want to extract data from Solana, you can take a look at the [Tutorials](../tutorials/solana/solana.md) section, which covers the development of several useful Substreams (SPL tokens, NFT trades, DEX trades...). The [Substreams for Solana Developers](../common/intro-solana.md) section is really useful if this is your first time using Substreams.

The [Substreams Explorer](../tutorials/solana/explore-solana/explore-solana.md) teaches you to most basic extractions you can perform on Solana. More tooling will be developed for Solana soon.
{% endtab %}

{% tab title="EVM" %}
If you have a specific smart contract that you want to extract data from, the `substreams init` initializes a Substreams project that extract the events from the smart contract.

## Tracking a Smart Contract

{% embed url="https://www.youtube.com/watch?v=vWYuOczDiAA" %}
Initialize a Substreams project
{% endembed %}

## Tracking a Factory Smart Contract (Dynamic Datasource)

The `substreams init` command also offers you the possibility to easily track a factory smart contract (in Substreams terminology, a _dynamic datasource_). The following video covers how easy it is to get started with factory contracts on Substreams.

{% embed url="https://www.youtube.com/watch?v=Vn11ovfSpNU" %}
Initialize a Substreams project
{% endembed %}

{% endtab %}
{% endtabs %}