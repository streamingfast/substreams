---
description: StreamingFast Substreams parallel execution
---

# Parallel execution

Parallel execution is the process of a Substreams module's code executing multiple segments of blockchain data simultaneously in a forward or backward direction. Substreams modules can be executed in parallel, rapidly producing data for consumption in end-user applications. Parallel execution enables Substreams' highly efficient blockchain data processing capabilities.

Parallel execution occurs when a requested module's start block is further back in the blockchain's history than the requested start block. For example, if a module starts at block 12,000,000 and a user requests data at block 15,000,000, parallel execution is used. This applies to both the development and production modes of Substreams operation.

Parallel execution addresses the problem of the slow single linear execution of a module. Instead of running a module in a linear fashion, one block after the other without leveraging full computing power, N number of workers are executed over a different segment of the chain. It means data can be pushed back to the user N times faster than cases using a single worker.

The server will define an execution schedule and take the module's dependencies into consideration. The server's execution schedule is a list of pairs of (`module, range`), where range contains `N` blocks. This is a configurable value set to 25K blocks, on the server.

The single map_transfer module will fulfill a request from 0 - 75,000. The server's execution plan returns the results of `[(map_transfer, 0 -> 24,999), (map_transfer, 25,000 -> 74,999), (map_transfer, 50,000 -> 74,999)]`.

The three pairs will be simultaneously executed by the server handling caching of the output of the store. For stores, an additional step will combine the store keys across multiple segments producing a unified and linear view of the store's state.

Assuming a chain has 16,000,000 blocks, which translates to 640 segments of 25K blocks. The server currently has a limited amount of concurrency. In theory, 640 concurrent workers could be spawned. In practice, the number of concurrent workers depends on the capabilities of the service provider. For the production endpoint, StreamingFast sets the concurrency to 15 to ensure fair usage of resources for the free service.

## Production versus development mode for parallel execution

The amount of parallel execution for the two modes is illustrated in the diagram. Production mode results in more parallel processing than development mode for the requested range. In contrast, development mode consists of more linear processing. Another important note is, foward parallel execution only occurs in production mode.

<figure><img src="https://github.com/streamingfast/substreams/raw/develop/docs/assets/substreams_processing.png" alt=""><figcaption><p>Substreams production versus development mode for parallel execution diagram</p></figcaption></figure>

## Backward and forward parallel execution steps

The two steps involved during parallel execution are **backward execution and forward execution**.

Backward parallel execution consists of executing in parallel block ranges, from the module's initial block, up to the start block of the request. If the start block of the request matches the module's initial block no backwards execution is performed.

Forward parallel execution consists of executing in parallel block ranges from the start block of the request up to last known final block, also called an irreversible block, or the stop block of the request depending on which is smaller. Forward parallel execution significantly improves the performance of Substreams.

Backward parallel execution will occur in both development and production modes.

Forward parallel execution only occurs in production mode.