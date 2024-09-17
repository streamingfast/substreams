In this guide, you'll learn how to initialize an Solana-based Substreams project. You’ll learn how to set up a simple project to extract raw data or filter Solana transactions based on Program IDs and Account IDs.

## Prerequisites

- Docker and VS Code installed and up-to-date.
- Visit the [Getting Started Guide](https://github.com/streamingfast/substreams-starter) to initialize your development environment.

## Step 1: Initialize Your Solana Substreams Project

1. Open your development environment and run the following command to initialize your project:
    
    ```bash
    substreams init
    ```

2. You will be given the option to choose between two Solana project options. Select the one that best fits your requirements:
    - **`sol-minimal`**: This option creates a simple Substreams project that extracts raw data directly from Solana blocks.
    - **`sol-transactions`**: This option creates a Substreams project that filters Solana transactions based on one or more Program IDs and/or Account IDs, using the cached [Solana Foundational Module](https://substreams.dev/streamingfast/solana-common/v0.3.0).

    Note: The filtered_transactions_without_votes module extracts transactions while excluding voting transactions, reducing data size and costs by 75%. To access voting transactions, use a full Solana block.
    
## Step 2: Visualize the Data

1. Create your account [here](https://thegraph.market/) to generate an authentification token (JWT) and pass it as input to: 

    ```bash
    substreams auth
    ```

2. Run the following command to visualize and itterate on your filtered data model:

    ```bash
    Substreams Gui
    ````

## Step 3: Customize your Project 

After initialization, you can:

- Modify your Substreams manifest to include additional filters or configurations.
- Implement custom processing logic in Rust based on the filtered data retrieved by the foundational module.

For a deeper dive into use cases and details, refer to the [Solana Tutorials](../../tutorials/solana).

## Additional Resources

You may find these additional resources helpful for developing your first Solana application.

### Development Container Reference

The [development container reference](../../references/devcontainer-ref) helps you navigate the complete container and its common errors. 

### Gui Reference

The [gui reference](../../references/gui) lets you explore the complete tool of the Pyth contract.

### Manifests Reference

The [manifests reference](../../references/manifests.md) helps you with editing the `substreams.yaml`.

