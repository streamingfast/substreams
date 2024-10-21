In this guide, you'll learn how to initialize a Vara-based Substreams project. You’ll learn how to set up a simple project to extract raw data or filter events from a smart contract.

## Prerequisites

- Docker and VS Code installed and up-to-date.
- Visit the [Getting Started Guide](https://github.com/streamingfast/substreams-starter) to initialize your development environment.

## Step 1: Initialize Your Vara Substreams Project

1. Open your development environment and run the following command to initialize your project:
    
    ```bash
    substreams init
    ```
    
2. You will be given the option to choose between two Vara project options. Select the one that best fits your requirements:
    - **vara-minimal**: Creates a simple Substreams that extracts raw data from the block and generates Rust code.
    - **starknet-extrinsics**: Creates a Substreams that extracts Vara extrinsics using the cached [Vara Foundational Module](https://substreams.dev/streamingfast/vara-common/v0.1.6).


## Step 2: Visualize the Data

1. Create your account [here](https://thegraph.market/) to generate an authentification token (JWT) and pass it as input to: 

    ```bash
    substreams auth
    ```

2. Run the following command to visualize and itterate on your filtered data model:

    ```bash
    substreams gui
    ````

## Step 3: Customize your Project 

After initialization, you can:

- Modify your Substreams manifest to include additional filters or configurations.
- Implement custom processing logic in Rust based on the filtered data retrieved by the foundational module.

## Additional Resources

You may find these additional resources helpful for developing your first EVM application.

### Development Container Reference

The [Development Container Reference](../references/devcontainer-ref.md) helps you navigate the complete container and its common errors. 

### GUI Reference

The [GUI reference](../references/gui.md) lets you explore all the tools available in the Substreams GUI.

### Manifests Reference

The [Manifests Reference](../references/manifests.md) helps you with editing the `substreams.yaml`.

