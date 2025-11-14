# Getting Started with Solana Account Changes

## Introduction

In this tutorial, you will learn how to consume Solana account change data using Substreams. We will walk you through the process of setting up your environment, configuring your first Substreams stream, and consuming account changes efficiently.

By the end of this tutorial, you will have a working Substreams feed that allows you to track real-time account changes on the Solana blockchain, as well as historical account change data.

{% hint style="info" %} 
Data for Solana account changes is available on a rolling three-month window.
{% endhint %}

For each Solana Account block, only the latest update per account is recorded, see the [Protobuf Reference](https://buf.build/streamingfast/firehose-solana/file/main:sf/solana/type/v1/account.proto). If an account is deleted, a payload with `deleted == True` is provided. Additionally, events of low importance we're omitted, such as those with the special owner “Vote11111111…” account or changes that do not affect the account data (ex: lamport changes).

{% hint style="success" %} The Account Changes Substreams natively handles a change of ownership upon delete. {% endhint %}

## Prerequisites

Before you begin, ensure that you have the following:

1. [Substreams CLI](../../how-to-guides/cli/installing-the-cli.md) installed.
2. A [Substreams key](../../references/cli/authentication.md) for access to the Solana Account Change data.
3. Basic knowledge of [how to use](../../references/cli/command-line-interface.md) the command line interface (CLI).

## Step 1: Set Up a Connection to Solana Account Change Substreams

Now that you have Substreams CLI installed, we can set up a connection to the Solana Account Change Substreams feed.

Using the [Solana Accounts Foundational Module](https://substreams.dev/packages/solana-accounts-foundational/latest), you can choose to stream data directly or use the GUI for a more visual experience. The following `gui` example filters for Honey Token account data.

```bash
 substreams gui  solana-accounts-foundational filtered_accounts -t +10 -p filtered_accounts="owner:TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA || account:4vMsoUT2BWatFweudnQM1xedRLfJgJ7hswhcpz4xgBTy"
```
This command will benchmark the account change stream within your terminal.

```bash
substreams run solana-accounts-foundational filtered_accounts -s -1 -o clock
```

The Foundational Module has support for filtering on specific accounts and/or owners. You can adjust the query based on your needs.

This tutorial will continue to guide you through filtering, sinking the data, and setting up reconnection policies.

## Step 2: Sink the Substreams

Consume the account stream [directly in your applicaion](../../how-to-guides/sinks/stream/stream.md) using a callback or make it queryable by using the [Substreams:SQL sink](../../how-to-guides/sinks/sql/sql-sink.md).

## Step 3: Setting up a Reconnection Policy

 [Cursor Management](../../references/reliability-guarantees.md) ensures seamless continuity and retraceability by allowing you to resume from the last consumed block if the connection is interrupted, preventing data loss and maintaining a persistent stream. 
 
 The user's primary responsibility when creating or using a sink is to pass a BlockScopedDataHandler and a BlockUndoSignalHandler implementation(s) which has the following interface:

```go
import (
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

type BlockScopedDataHandler = func(ctx context.Context, cursor *Cursor, data *pbsubstreamsrpc.BlockScopedData) error
type BlockUndoSignalHandler = func(ctx context.Context, cursor *Cursor, undoSignal *pbsubstreamsrpc.BlockUndoSignal) error
```
