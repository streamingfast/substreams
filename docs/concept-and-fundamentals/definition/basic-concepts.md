---
description: StreamingFast Substreams basic concepts
---

# Basic Concepts

### Problem

Web3 development has been highly centered around saving data to blockchain ledgers. Everything from buying Bitcoin or an NFT to trading cryptocurrencies is rooted in committing transaction data to blockchain ledgers.

Searching through the linear transaction data in the ledgers hasn’t historically seen the same level of development effort. Finding and aggregating blockchain data can be difficult, time-consuming, costly, and computationally intensive. Before Substreams was created, blazing fast, easy and efficient searchability of blockchain data was simply not possible.&#x20;

### Solution

A revolutionary approach to data extraction from blockchain nodes, called Firehose, provides massive levels of previously unseen data availability to Substreams. Requests can be made for single blocks at any point in the blockchain ledger. The data inside each block is fully searchable down to the transaction event level. Substreams processes many blocks at once, in parallel, enabling developers to instantly isolate and locate any data in full blockchain ledgers without the need for linear processing.

The Rust programming language is used by the developer to define data of interest available in the blockchain. Substreams can route data to a myriad of stores including file systems, relational databases, and even straight into an application’s user interface.
