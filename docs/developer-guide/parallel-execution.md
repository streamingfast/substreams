---
description: StreamingFast Substreams parallel Execution
---

# Parallel Execution

In simple terms, parallel execution is the ability to pre-compute the execution of Substreams modules. In more detail, parallel execution is the process of a Substreams module's code executing multiple segments of blockchain data simultaneously. Substreams modules are executed in parallel, rapidly producing data for consumption in end-user applications.

Parallel Execution is what's responsible for making Substreams extremely fast. StreamingFast was able to use 120 concurrent workers to completely process and store all of the ERC20/ERC721/ERC1155 token transfers on the Ethereum Mainnet in nearly 45 minutes!

The server will define an execution schedule and take the module's dependencies into consideration. The server's execution schedule is a list of pairs of (`module, range`), where range contains `N` blocks. This is a configurable value set to 25K blocks, on the server.

The single map_transfer module will fulfill a request from 0 - 75,000. The server's execution plan returns the results of `[(map_transfer, 0 -> 24,999), (map_transfer, 25,000 -> 74,999), (map_transfer, 50,000 -> 74,999)]`.

The three pairs will be simultaneously executed by the server handling caching of the output of the store. For stores, an additional step will combine the store keys across multiple segments producing a unified and linear view of the store's state.

The Ethereum Mainnet consists of roughly 16,000,000 blocks translating to 640 segments of 25K blocks. The server currently accepts a limited amount of concurrency. In theory, 640 concurrent workers could be spanned, in practice, it depends on the capabilities of the service provider StreamingFast sets the concurrency to 15 for the production endpoint producing fair usage of resources for the free service.

Parallel execution occurs when a requested module's start block is further back in the blockchain's history than the requested start block. For example, if a module starts at block 12,000,000 and a user requests data at block 15,000,000, parallel execution is used. This applies to both the development and production modes of Substreams operation; parallel execution is performed for the full range of blocks when Substreams is in production mode.

Parallel execution addresses the problem of the slow single linear execution of a module. Instead of running a module in a linear fashion, one block after the other without leveraging full computing power, we execute N workers over a different segment of the chain. It means we are able to push data back to the user N times faster than if we had 1 worker.
