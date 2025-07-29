In this guide, you'll learn how to publish a Substreams package to the [Substreams Registry](https://substreams.dev).

## Prerequisites

- You must have the Substreams CLI installed.
- You must have a Substreams package (`.spkg`) that you want to publish.

## Step 1: Run the `substreams publish` Command

1. In a command-line terminal, run `substreams publish <YOUR-PACKAGE>.spkg`.

2. If you do not have a token set in your computer, navigate to `https://substreams.dev/me`.

<figure><img src="../../.gitbook/assets/tutorials/publish-package/1_get-token.png" alt="" width="100%"></figure>

## Step 2: Get a Token in the Substreams Registry

1. In the Substreams Registry, log in with your GitHub account.

2. Create a new token and copy it in a safe location.

<figure><img src="../../.gitbook/assets/tutorials/publish-package/2_new_token.png" alt="" width="100%"></figure>

## Step 3: Authenticate in the Substreams CLI

1. Back in the Substreams CLI, paste the previously generated token.

<figure><img src="../../.gitbook/assets/tutorials/publish-package/3_paste_token.png" alt="" width="100%"></figure>

2. Lastly, confirm that you want to publish the package.

<figure><img src="../../.gitbook/assets/tutorials/publish-package/4_confirm.png" alt="" width="100%"></figure>

That's it! You have succesfully published a package in the Substreams registry.
