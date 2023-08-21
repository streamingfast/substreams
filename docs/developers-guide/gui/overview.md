# Using the Substreams GUI

When running Substreams through the CLI, you can use two different commands: `substreams run` and `substreams gui`. The `substreams run` prints the output of the execution linearly for every block, such as the following:

// example

However, this is not useful approach when dealing with complex Substreams (i.e. with several modules and many blocks). The `substreams gui` command allows you to easily see the progress of a Substreams, switch modules or search within the output.

// example

In the following pages, you will learn more about the GUI capabilities.

## Launching the GUI

In order to showcase the different options of the GUI, the [Ethereum Block Meta Substreams](https://github.com/streamingfast/substreams-eth-block-meta/) will be used as an example.
By running the following command, you are executing the `kv_out` module, which retrieves outputs the data in a key-value format.

```bash
substreams gui -e mainnet.eth.streamingfast.io:443 https://github.com/streamingfast/substreams-eth-block-meta/releases/download/v0.5.1/substreams-eth-block-meta-v0.5.1.spkg kv_out --start-block 17712038 --stop-block +100
```

In your command-line terminal, you should see something like:

<img src="../../.gitbook/assets/gui/launching.gif" alt="" class="gitbook-drawing">

The `Progress` screen provides information about the Substreams execution, such as its status or the payload received. Once all the blocks have been consumed, the status is `Stream ended`.
There are two other main screens in the Substreams GUI: `Request` and `Output`. You can move to a different screen by using the `tab` key:

// video 2

You can restart the stream by pressing the `s` key.

// video 6

To quit the GUI, press the `q` key.

## The Output Screen

If you are in the `Progress` screen, press `tab` in your keyboard to move to the `Output` screen. In this screen, you can see the Protobuf output for every block. The image below shows the output for the block number `17712038` (the starting block).

// image 3

### Navigating Through Blocks

You can see the output for other blocks by using the `o` and `p` keys.
The `o` key takes to the following block, and the `p` takes your to the previous block

// video 4

If you want to jump to a specific block, you can press the `=` and specify the block number. Then, just press `enter`.

### Navigating Through Modules

// video 5




