# Using the CLI

Here are the commands you can invoke with the `substreams` command-line tool.

{% hint style="info" %}
Don't forget to [install it](../developer-guide/installation-requirements.md#install-the-substreams-cli-tool).
{% endhint %}

### **`run`**

The `run` command allows you connect to a Substreams endpoint and start processing data.

```
substreams run -e api-dev.streamingfast.io:443 \
   --stop-block +1 \
   ./substreams.yaml \
   module_name
```

Let's break down everything happening above.

* `substreams` is our executable, `run` our command
* `-e api-dev.streamingfast.io:443` is the endpoint of the provider running our Substreams
* `--stop-block +1` only requests a single block (stop block will be manifest's `initialBlock` + 1)
* `substreams.yaml` is the path where we have defined our [Substreams Manifest](https://github.com/streamingfast/substreams-docs/blob/master/docs/guides/docs/reference/manifests.html). This can be an `.spkg` or a `substreams.yaml` file.
* `module_name` this is the module we want to run, refering to the `name` [defined in the manifest](manifests.md#modules-.name).

Passing a different `-s` (or `--start-block`) will run any prior modules at high speed, in order to provide you with output at the requested start block as fast as possible, while keeping snapshots along the way, in case you want to process it again.

Here is the example of an output of the `block_to_erc_20_transfer` starting at `14440000` block for only `1` block. The `<continued ...>` was added to abbreviate the JSON output as there were a lot of ERC20 transfers.

```
2022-05-11T12:38:56.232-0400 INFO (substreams) connecting...
2022-05-11T12:38:56.259-0400 INFO (substreams) connected
----------- Block #14 440 000 (irreversible) ---------------
block_to_erc_20_transfer: message "eth.erc20.v1.Transfers": {
  "transfers": [
    {
      "from": "0x48acf41d10a063f9a6b718b9aad2e2ff5b319ca2",
      "to": "0x109403ab5c5896711699dd3de01c1d520f79801a",
      "amount": "7752235492486228145381410794440202021481973102607839495265831900745419456512",
      "balanceChangeFrom": [
        {
          "oldBalance": "13569385457497991651199724805705614201555076328004753598373935625927319879680",
          "newBalance": "14021698306081258039573048965895801341606912205604912051653066813458230542336"
        }
      ],
      "balanceChangeTo": [
        {
          "oldBalance": "9498569820248594155839807363993929941088553429603327518861754938149123915776",
          "newBalance": "9950882668831860544213131524184117081140389307203485972140886125680034578432"
        }
      ]
    },
    <continued ...>
  ]
}
```

### `pack`

The `pack` command builds a shippable, importable package from a `substreams.yaml` manifest file.

Use:

```bash
$ substreams pack ./substreams.yaml
...
Successfully wrote "your-package-v0.1.0.spkg".
```

### `inspect`

Use to inspect the contents of a Substreams package (`yaml` or `spkg`):

```
$ substreams inspect ./substreams.yaml | less
proto_files {
...
modules {
  modules {
    name: "my_module_name"
...
```
