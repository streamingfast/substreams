# Launching the GUI

In order to showcase the different options of the GUI, the "Ethereum Block Meta" Substreams will be used as an example.
By running the following command, you are executing the `kv_out` module, which retrieves outputs the data in a key-value format.

```bash
substreams gui -e mainnet.eth.streamingfast.io:443 explorer1.spkg map_filter_transactions --start-block 17712038 --stop-block +100
```

In your command-line terminal, you see something like:

<video width="320" height="240" controls>
  <source src="../.gitbook/assets/videos/block1.mp4ss" type="video/mp4">
  Your browser does not support the video tag.
</video>

The `Progress` screen provides information about the Substreams execution, such as its status or the payload received. Once all the blocks have been consumed, the status is `Stream ended`.
There are two other main screens in the Substreams GUI: `Request` and `Output`. You can move to a different screen by using the `tab` key:

// video 2


Press `s` to restar the stream

// video 6

Press `q` to quit the GUI

## The Output Screen

If you are in the `Progress` screen, press `tab` in your keyboard to move to the `Output` screen. In this screen, you can see the Protobuf output of your Substreams for every block. The image below shows output for the block number `17712038` (the starting block) is shown.

// image 3

### Navigating Through Blocks

You can see the output for other blocks by using the `o` and `p` keys.

// video 4

If you want to jump to a specific block, you can press the `=` and specify the block number

### Navigating Through Modules

// video 5



