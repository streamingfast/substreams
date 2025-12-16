# Substreams Authentication Guide

This guide explains how to authenticate when running a Substreams package (`.spkg`) with a provider, specifically using The Graph Market.

## Overview

Substreams requires authentication to ensure secure and controlled access to providers. This guide focuses on obtaining and using a JWT token from The Graph Market to authenticate your Substreams execution.

## Prerequisites

- A Substreams package (`.spkg`) ready to deploy.
- An account with [The Graph Market](https://thegraph.market).

## Step 1: Obtain a JWT Token

To authenticate with The Graph Market, you need to generate a JWT token. Follow these steps:

1. **Log in to The Graph Market**:
   - Visit [https://thegraph.market](https://thegraph.market).
   - Log in to your existing account or create a new one if you don't have an account.

2. **Access the Dashboard**:
   - Click on `Dashboard` in the navigation menu or go directly to [https://thegraph.market/dashboard](https://thegraph.market/dashboard).

   ![Dashboard](../../.gitbook/assets/intro/thegraphmarket.png)

3. **Create a New API Key**:
   - In the dashboard, click on `Create New Key`.
   - Input a recognizable name for future reference.
   - This is not the _authentication token_, but a key to generate tokens.

4. **Generate an API Token**:
   - For security reasons, the API token is hidden. In the `API TOKEN` section, click the button besides the hidden token.
   - The system will generate a JWT token. **Copy** and **save** this token securely, as it will be required for authentication.

## Step 2: Set the JWT Token as an Environment Variable

To authenticate Substreams on your local machine, you need to set the JWT token as an environment variable.

### Unix-like Systems (macOS, Linux)

1. **Open a terminal** on your machine.

2. **Set the environment variable** using the following command:

   ```bash
   export SUBSTREAMS_API_TOKEN="<YOUR-JWT-TOKEN>"
   ```

   Replace `<YOUR-JWT-TOKEN>` with the JWT token you obtained earlier.

## Step 3: Verify Authentication

To ensure that your authentication is set up correctly, you can run a test Substreams. Here's how:

1. Run the following command in your terminal to get all the events on Ethereum Mainnet:

   ```bash
   substreams gui ethereum-common@v0.3.1 all_events --start-block=15000000
   ```

2. Verify that the Substreams runs without errors, confirming that your authentication is successful.

## Need Help?

If you encounter any issues or have questions, the StreamingFast team is available on [Discord](https://discord.gg/jZwqxJAvRs) to assist you.
