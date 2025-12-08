The [Substreams Go Sink library](https://github.com/streamingfast/substreams-sink) allows to you to programmatically stream a Substreams using the Go programming language. The library handles reconnections and provides best practices for error handling.

The [Substreams Sink Examples GitHub repository](https://github.com/streamingfast/substreams-sink-examples) contains an example that you can use as the starting point to build your custom sink logic. After cloning the repository, move to the `go` directory.

## Run the Program

This example is built in the form of a CLI by using the `cobra` library. You can run the program by running the following command structure:

```bash
go run . sink <ENDPOINT> <SPKG> <MODULE_NAME>
```

In the following command, `go run .` is used to execute the `main.go` file. The `mainnet.eth.streamingfast.io:443 https://spkg.io/streamingfast/substreams-eth-block-meta-v0.4.3.spkg db_out` part of the command are useful parameters passed to the Go program (separated by a whitespace).

In the parameters, you pass the Substreams endpoint, the package, and the module to execute.

```bash
go run . sink mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.5.1/substreams-eth-block-meta-v0.5.1.spkg db_out
```

#### Inspect the Code

The example contains code comments, which are very useful to understand and adjust the code to your logic needs. Let's inspect the most important parts of the code: 

```go
var expectedOutputModuleType = string(new(pbchanges.DatabaseChanges).ProtoReflect().Descriptor().FullName()) // 1.

// ...code omitted...

func main() {
	logging.InstantiateLoggers()

	Run(
		"sinker",
		"Simple Go sinker sinking data to your terminal",

		Command(sinkRunE,
			"sink <endpoint> <manifest> [<output_module>]",
			"Run the sinker code",
			RangeArgs(2, 3),
			Flags(func(flags *pflag.FlagSet) {
				sink.AddFlagsToSet(flags)
			}),
		),

		OnCommandErrorLogAndExit(zlog),
	)
}
```
Create a new sink object from the parameters passed to the program:

```go
func sinkRunE(cmd *cobra.Command, args []string) error {
	endpoint := args[0]
	manifestPath := args[1]

	// Find the output module in the manifest sink.moduleName configuration. If you have no
	// such configuration, you can change the value below and set the module name explicitly.
	outputModuleName := sink.InferOutputModuleFromPackage
	if len(args) == 3 {
		outputModuleName = args[2]
	}

	sinker, err := sink.NewFromViper(
		cmd,
		// Should be the Protobuf full name of the map's module output, we use
		// `substreams-database-changes` imported type. Adjust to your needs.
		//
		// If your Protobuf is defined in your Substreams manifest, you can use `substream protogen`
		// while being in the same folder that contain `buf.gen.yaml` file in the example folder.
		expectedOutputModuleType,
		endpoint,
		manifestPath,
		outputModuleName,
		// This is the block range, in our case defined as Substreams module's start block and up forever
		":",
		zlog,
		tracer,
	)
	cli.NoError(err, "unable to create sinker: %s", err)

	sinker.OnTerminating(func(err error) {
		cli.NoError(err, "unexpected sinker error")

		zlog.Info("sink is terminating")
	})

	// You **must** save the cursor somewhere, saving it to memory while
	// make it last until the process is killed, in which on re-start, the
	// sinker will resume from start block again. You can simply read from
	// a file the string value of the cursor and use `sink.NewCursor(value)`
	// to load it.

	// Blocking call, will return on sinker termination
	sinker.Run(context.Background(), sink.NewBlankCursor(), sink.NewSinkerHandlers(handleBlockScopedData, handleBlockUndoSignal))
	return nil
}
```

It is necessary to handle two kind of Substreams response messages: 
- `blockScopedData`: sent by the server whenever a new block is discovered in the blockchain. Contains all the block information that you can decode.
- `blockUndoSignal`: sent every time there is a fork in the blockchain. Because you have probably read incorrect blocks in the `blockScopedData` message, you must rewind back to the latest valid block.

When you run the sinker, you pass two different functions to handle these messages:

```go
sinker.Run(context.Background(), sink.NewBlankCursor(), sink.NewSinkerHandlers(handleBlockScopedData, handleBlockUndoSignal))
```