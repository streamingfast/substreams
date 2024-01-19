
Getting started with Substreams is very easy! If you have the Substreams CLI installed in your computer, you can use the `substreams init` command to initialize a Substreams project from a smart contract.

Given a smart contract address, the `substreams init` command will download the contract's ABI, inspect it and create a Substreams that extracts all the events.

{% embed url="https://www.youtube.com/watch?v=vWYuOczDiAA&" %}
Initialize a Substreams project
{% endembed %}

## Initialize the Project

1. Run `substreams init`:

```bash
substreams init
```

2. Provide a name for the project:

```bash
✗ Project name (lowercase, numbers, undescores):
```

3. Select what protocol you want to use (e.g. Ethereum):

```bash
? Select protocol: 
  ▸ Ethereum
    Other
```

4. Select what chain to use (Ethereum Mainnet, BNB, Polygon...):

```bash
? Select Ethereum chain: 
  ▸ Mainnet
    BNB
    Polygon
    Goerli
↓   Mumbai
```

5. Input the smart contrat address. If you do not provider an addressm the "Bored Ape Yacht Club" smart contract will be used:

```bash
✔ Contract address to track (leave empty to use "Bored Ape Yacht Club"):
```

After providing all the previous information, a new Substreams project will be generated. You can now start coding!