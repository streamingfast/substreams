The [Substreams JavaScript library](https://github.com/substreams-js/substreams-js) enables you to run a Substreams, just like you would through the CLI, but using JavaScript.

The library works both on the client-side and on the server-side, but with some small differences. Clone the [Substreams Sink Examples](https://github.com/streamingfast/substreams-sink-examples) repository contains examples several programming language. Then, move to the `javascript` folder.

Depending on your needs, you can use the `node` directory (which contains an example using server-side NodeJS) or the `web` directory (which contains an example using client-side JavaScript).

## Install the dependencies

The `package.json` contains all the necessary dependencies to run the application.

The NodeJS example uses `@connectrpc/connect-node`, while the Web example uses `@connectrpc/connect-web`.

{% tabs %}
{% tab title="NodeJS" %}
```json
{
    "name": "substreams-js-node-example",
    "version": "1.0.0",
    "description": "",
    "main": "index.js",
    "dependencies": {
      "@substreams/core": "^0.1.19",
      "@substreams/manifest": "^0.0.9",
      "@connectrpc/connect-node": "1.3.0",
      "@connectrpc/connect": "1.3.0"
    },
    "type": "module"
  }
```
{% endtab %}

{% tab title="Web" %}
```json
{
  "name": "substreams-js-web-example",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "dependencies": {
    "@substreams/core": "^0.1.19",
    "@substreams/manifest": "^0.0.9",
    "@connectrpc/connect-web": "1.4.0",
    "@connectrpc/connect": "1.4.0"
  },
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "devDependencies": {
    "vite": "^5.1.6"
  }
}
```
{% endtab %}
{% endtabs %}

You can install the dependencies by running:

```bash
npm install
```

## Run the Application

{% tabs %}
{% tab title="NodeJS" %}
```bash
node index.js
```

You will start receiving data! 
{% endtab %}

{% tab title="Web" %}
The Web example uses [ViteJS](https://vitejs.dev/) to create a development server that runs the application:
```bash
npm run dev
```

Then, you can navigate to `https://localhost:5173`. You will start receiving data!
{% endtab %}
{% endtabs %}

## Explore the Application

When you consume a Substreams package, a long-live gRPC connection is established, therefore, disconnections will happen and should be taken as _normal_. The Substreams keeps track of latest block you consumed by sending a **cursor** to your application. You **must** persist the cursor, so that in the case of a disconnection, you can restart the application from the latest consumed block.

{% tabs %}
{% tab title="NodeJS" %}
The `index.js` file contains the `main()` function, which runs an infite loop and takes care of managing the disconnections.

```js
const TOKEN = process.env.SUBSTREAMS_API_TOKEN // Substreams token. By default it takes the SUBSTREAMS_API_TOKEN environment variable of your system
const ENDPOINT = "https://mainnet.eth.streamingfast.io" // Substreams endpont. In this case, Ethereum mainnet
const SPKG = "https://spkg.io/streamingfast/ethereum-explorer-v0.1.2.spkg" // Substreams package. In this case, taken from the substreams.dev registry
const MODULE = "map_block_meta"
const START_BLOCK = '100000'
const STOP_BLOCK = '+10000'

/*
    Entrypoint of the application.
    Because of the long-running connection, Substreams will disconnect from time to time.
    The application MUST handle disconnections and commit the provided cursor to avoid missing information.
*/
const main = async () => {
    const pkg = await fetchPackage() // Download spkg
    const registry = createRegistry(pkg);

    // Create gRPC connection
    const transport = createConnectTransport({
        baseUrl: ENDPOINT,
        interceptors: [createAuthInterceptor(TOKEN)],
        useBinaryFormat: true,
        jsonOptions: {
            typeRegistry: registry,
        },
    });
    
    // The infite loop handles disconnections. Every time an disconnection error is thrown, the loop will automatically reconnect
    // and start consuming from the latest commited cursor.
    while (true) {
        try {
            await stream(pkg, registry, transport);
        } catch (e) {
            if (!isErrorRetryable(e)) {
              console.log(`A fatal error occurred: ${e}`)
              throw e
            }
            console.log(`A retryable error occurred (${e}), retrying after backoff`)
            console.log(e)
            // Add backoff from a an easy to use library
        }
    }
}
```
{% endtab %}

{% tab title="Web" %}
The `main.js` file contains the `main()` function, which runs an infite loop and takes care of managing the disconnections.

```js
const TOKEN = "<SUBTREAMS-TOKEN>" // Substreams token. Put here your Substreams API token.
const ENDPOINT = "https://mainnet.eth.streamingfast.io" // Substreams endpont. In this case, Ethereum mainnet
const SPKG = "https://spkg.io/streamingfast/ethereum-explorer-v0.1.2.spkg" // Substreams package. In this case, taken from the substreams.dev registry
const MODULE = "map_block_meta"
const START_BLOCK = '100000'
const STOP_BLOCK = '+10000'


/*
  Entrypoint of the application.
  Because of the long-running connection, Substreams will disconnect from time to time.
  The application MUST handle disconnections and commit the provided cursor to avoid missing information.
*/
const main = async () => {
  const pkg = await fetchPackage(); // Download spkg
  const registry = createRegistry(pkg);

  const transport = createConnectTransport({
      baseUrl: ENDPOINT,
      interceptors: [createAuthInterceptor(TOKEN)],
      useBinaryFormat: true,
      jsonOptions: {
          typeRegistry: registry,
      },
  });
  
  // The infite loop handles disconnections. Every time an disconnection error is thrown, the loop will automatically reconnect
  // and start consuming from the latest commited cursor.
  while (true) {
      try {
          await stream(pkg, registry, transport);
      } catch (e) {
          if (!isErrorRetryable(e)) {
            console.log(`A fatal error occurred: ${e}`)
            throw e
          }
          console.log(`A retryable error occurred (${e}), retrying after backoff`)
          console.log(e)
      }
  }
}
```
{% endtab %}
{% endtabs %}

The `stream()` function establishes the actual streaming connection by calling the `streamBlocks` function. The response of the function is a `StatefulResponse` object, which contains a progress message (containing useful information about the Substreams execution. The `handleProgressMessage()` function handles this message) and a response message (containing the message sent from the server. The `handleResponseMessage()` function decodes this message).

```js
const stream = async (pkg, registry, transport) => {
  const request = createRequest({
      substreamPackage: pkg,
      outputModule: MODULE,
      productionMode: true,
      startBlockNum: START_BLOCK,
      stopBlockNum: STOP_BLOCK,
      startCursor: getCursor() ?? undefined
  });
  
  // Stream the blocks
  for await (const statefulResponse of streamBlocks(transport, request)) {
       /*
            Decode the response and handle the message.
            There different types of response messages that you can receive. You can read more about the response message in the docs:
            https://substreams.streamingfast.io/documentation/consume/reliability-guarantees#the-response-format
        */
        await handleResponseMessage(statefulResponse.response, registry);

        /*
            Handle the progress message.
            Regardless of the response message, the progress message is always sent, and gives you useful information about the execution of the Substreams.
        */
        handleProgressMessage(statefulResponse.progress, registry);
  }
}
```

There are different kind of response messages that the server can send. The most common are ones `blockScopedData` and `blockUndoSignal`:
- `blockScopedData`: sent by the server whenever a new block is discovered in the blockchain. Contains all the block information that you can decode.
- `blockUndoSignal`: sent every time there is a fork in the blockchain. Because you have probably read incorrect blocks in the `blockScopedData` message, you must rewind back to the latest valid block.

```js
export const handleResponseMessage = async (response, registry) => {
    switch(response.message.case) {
        case "blockScopedData":
            handleBlockScopedDataMessage(response.message.value, registry);
            break;

        case "blockUndoSignal":
            handleBlockUndoSignalMessage(response.message.value);
            break;
    }
}
```
