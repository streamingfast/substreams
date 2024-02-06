## Go

The [Substreams Go Sink library](https://github.com/streamingfast/substreams-sink) allows to you to programmatically stream a Substreams using the Go programming language. The library handles reconnections and provides best practices for error handling.

The GitHub repository contains [a couple of examples](https://github.com/streamingfast/substreams-sink/tree/develop/examples) that you can use as the starting point to build your custom sink logic.


### Getting Started with the Basic Example

Although you can use the `basic` example to get started, the Substreams team recommends using the `advanced` example in production environments.

In the Substreams Go Sink project, move to the `examples/basic` folder and open the `main.go` file.

#### Run the Program

In this example, the `https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.5.1/substreams-eth-block-meta-v0.5.1.spkg` package is used. You should adjust your Go program depending on the Protobuf output emitted by your Substreams.

In the following command, `go run .` is used to execute the `main.go` file. The `mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.5.1/substreams-eth-block-meta-v0.5.1.spkg db_out` part of the command are useful parameters passed to the Go program (separated by a whitespace).

In the parameters, you pass the Substreams endpoint, the package, and the module to execute.

```bash
go run . mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.5.1/substreams-eth-block-meta-v0.5.1.spkg db_out
```

#### Inspect the Code

The example contains code comments, which are very useful to understand and adjust the code to your logic needs. Let's inspect the most important parts of the code: 

```go
var expectedOutputModuleType = string(new(pbchanges.DatabaseChanges).ProtoReflect().Descriptor().FullName()) // 1.

// ...code omitted...

func main() {
    // ...code omitted...

    cli.Ensure(len(os.Args) == 3 || len(os.Args) == 4, "usage: %s <endpoint> <manifest> [<output_module>]", os.Args[0]) // 2.

	endpoint := os.Args[1] // 3.
	manifestPath := os.Args[2] // 4.

	outputModuleName := sink.InferOutputModuleFromPackage // 5.
	if len(os.Args) == 4 {
		outputModuleName = os.Args[3] // 6.
	}

	params := ([]string)(nil) // 7.

    // ...code omitted...
}
```
1. The output type of the Substreams module (i.e. the Protobuf output). In this example, the `db_out` emits `DatabaseChanges`. This will depend on the Substreams you are executing and the output type it emits.
2. Verify the number of parameters passed to the Go program. In this case, the endpoint and the manifest (package) are mandatory, while the module name is not.
3. Store the endpoint in a variable.
4. Store the manifest in a variable.
5. If the module name is NOT present in the parameters, this option tries to infer the module name from the manifest.
6. If the module is present in the parameters, then you store it in a variable.
7. Set the parameters for the Substreams module.
These are [parameters of the Substreams](../../develop/parameterized-modules.md) itself, do not confuse them with the parameters of the Go program.

Once you have all the information needed, you can create the actual Go structs from the string variables:

```go
// ...code omitted...

func main() {
    // ...code omitted...

    pkg, outputModule, outputHashModule, blockRange, err := sink.ReadManifestAndModuleAndBlockRange(
            manifestPath,
            "mainnet", // 1.
            params,
            outputModuleName,
            expectedOutputModuleType, // 2.
            false,
            ":", // 3.
            zlog,
        )
    
    // ...code omitted...
}
```
1. Equivalent to the `network` parameter of the CLI. Specify the network to use for params and initialBlocks, overriding the `network`` field in the Substreams package.
2. The output type emitted by the module.
3. The range block to execute. `:` means from the start of the chain up to the latest block.

The `ReadManifestAndModuleAndBlockRange` will generate several Go structs collected in their corresponding variables (`pkg`, `outputModule`...), which will be used to initialize the sink:

```go
// ...code omitted...

func main() {
    // ...code omitted...

    sinker, err := sink.New(
            sink.SubstreamsModeProduction, // 1.
            pkg,
            outputModule,
            outputHashModule,
            client.NewSubstreamsClientConfig(endpoint, os.Getenv("SUBSTREAMS_API_TOKEN"), false, false), // 2.
            zlog,
            tracer,
            sink.WithBlockRange(blockRange), // 3.
        )
    
    // ...code omitted...
}
```
1. Run the Substreams in _production_ mode.
2. Create a Substreams client, which includes the endpoint and the authentication token.
This will also work if you have a system environment variable, `SUBSTREAMS_API_TOKEN`, with a valid Substreams token.
3. Set the block range.

With the sink created, you can now run it:

```go
// ...code omitted...

func main() {
    // ...code omitted...

    sinker.OnTerminating(func(err error) { // 1.
		cli.NoError(err, "unexpected sinker error")

		zlog.Info("sink is terminating")
	})

	sinker.Run(context.Background(), sink.NewBlankCursor(), sink.NewSinkerHandlers(handleBlockScopedData, handleBlockUndoSignal)) // 2.
}
```
1. Handle termination of the sink.
2. Actually run the sink. There are two important parameters:
- **The cursor:** in this example, a new cursor is created with `sink.NewBlankCursor()`. In a production environment, you **must** persist the cursor. In the case of a network error, this will allow you to start from the latest block read by the sink.
- **The sink handlers:** two functions passed as parameters where you can include the logic for the sink.

The `BlockScopedData` handler is a callback to handle the data returned by the Substreams API. Include here the logic you need after reading a block.
The `BlockUndoSignal` handler is only called when the chain forks. The callback contains the last valid block, so you must rewind your changes back to the latest valid block. You **must** also persist the last valid cursor provided in the undo signal.
