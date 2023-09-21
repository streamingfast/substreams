## Custom Sink Using JavaScript

If none of the previous sinks work for you, don't worry! You can create your own custom sink by using the [Substreams JavaScript library](https://github.com/substreams-js/substreams-js).
This library enables you to run a Substreams, just like you would through the CLI, but programatically.

The library works both on the client-side and the server-side.

### Installing the Library

In your JavaScript project, use your preferred JavaScript package manager to install the required dependencies:

1. The Substreams Core library:

```bash
npm install @substreams/core
```

2. The Substreams Manifest library:

```bash
npm install @substreams/manifest
```

3. The Protobuf library, which will be used to decode the Substreams response:

```bash
npm install @bufbuild/connect-web
```

### Using the Library

In order to use the library, you will need:

- A Substreams endpoint.
- An authentication token (visit https://app.streamingfast.io to get one).
- A Substreams package (`spkg`).

Consider that you want to consume the `map_block_meta` module of the [Ethereum Explorer package](https://github.com/streamingfast/substreams-explorers/tree/main/ethereum-explorer), which is hosted on Google Cloud (`https://storage.googleapis.com/substreams-registry/spkg/ethereum-explorer-v0.1.1.spkg`).

1. First, let's define a few helper variables:

```javascript
const TOKEN = "YOUR_TOKEN" // Your authentication token
const SPKG = "https://storage.googleapis.com/substreams-registry/spkg/ethereum-explorer-v0.1.1.spkg" // URL of the SPKG
const MODULE = "map_block_meta" // Name of the Substreams Module to run
```

2. Use the `fetchSubstream` method from the library to download the Substreams. Then, the `createRegistry` function creates the Protobuf definitions from the package:

```javascript
const fetchPackage = async () => {
    return await fetchSubstream(SPKG)
}

const main = async () => {
    // Fetch Substreams
    const pkg = await fetchPackage()
    // Create Protobuf registry
    const registry = createRegistry(pkg);
}
```

3. Use the `createConnectTransport` to define the networking details of the connection (Substreams endpoint and authentication token):

```javascript
const main = async () => {
    const pkg = await fetchPackage()
    const registry = createRegistry(pkg);

    const transport = createConnectTransport({
        // Substreams endpoint
        baseUrl: "https://api.streamingfast.io",
        // Authentication token
        interceptors: [createAuthInterceptor(TOKEN)],
        useBinaryFormat: true,
        jsonOptions: {
            // Protobuf Registry
            typeRegistry: registry,
        },
    });
}
```

4. The `createRequest` function encapsulates the information of the execution (package, module, start block and stop block):

```javascript
const main = async () => {
    const pkg = await fetchPackage()
    const registry = createRegistry(pkg);

    const transport = createConnectTransport({
        baseUrl: "https://api.streamingfast.io",
        interceptors: [createAuthInterceptor(TOKEN)],
        useBinaryFormat: true,
        jsonOptions: {
            typeRegistry: registry,
        },
    });

    // Execution details
    const request = createRequest({
        substreamPackage: pkg,
        outputModule: MODULE,
        productionMode: true,
        startBlockNum: 100000,
        stopBlockNum: '+10',
    });
}
```

5. Finally, you can use the `streamBlocks` function to iterate over the stream of blocks returned by the Substreams endpoint:

```javascript
const main = async () => {
    const pkg = await fetchPackage()
    const registry = createRegistry(pkg);

    const transport = createConnectTransport({
        baseUrl: "https://api.streamingfast.io",
        interceptors: [createAuthInterceptor(TOKEN)],
        useBinaryFormat: true,
        jsonOptions: {
            typeRegistry: registry,
        },
    });

    const request = createRequest({
        substreamPackage: pkg,
        outputModule: MODULE,
        productionMode: true,
        startBlockNum: 100000,
        stopBlockNum: '+10',
    });

    // Iterate over blocks
    for await (const response of streamBlocks(transport, request)) {
        const output = unpackMapOutput(response.response, registry);

        if (output !== undefined && !isEmptyMessage(output)) {
            const outputAsJson = output.toJson({typeRegistry: registry});
            console.log(outputAsJson)
        }
    }
}
```

Now, you can send the data anywhere and create your own custom sink! If you have created a sink and you think it can be reused by other developers, [let us know on Discord](https://discord.gg/jZwqxJAvRs)!

The previous code is availalble [on GitHub](https://gist.github.com/enoldev/b9f32e045f47675bd5c20f92246aed84).
