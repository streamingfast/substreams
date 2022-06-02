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

Here is the example of an output of the `graviatar_updates` starting at block 6200807:

```
$ substreams run -e api-dev.streamingfast.io:443 \
                 gravity-v0.1.0.spkg gravatar_updates -o json
{
  "updates": [
    {
      "id": "39",
      "owner": "0xaadcc13071fdf9c73cfbb8d97639ea68aa6fd1d2",
      "displayName": "alex | OpenSea",
      "imageUrl": "https://ucarecdn.com/13a67247-cb89-417a-92d2-50a7d7aa481c/-/crop/382x382/0,0/-/preview/"
    }
  ]
}
...
```

Notice the `-o` (or `--output`), that can alter the output format. The options are:

* `ui`, a nicely formatted, UI-driven interface, with progress information, and execution logs.
* `json`, an indented stream of data, with no progress information nor logs, but just data output for blocks following the start block.
* `jsonl`, same as `json` but with each output on a single line
* `module-json`, same as `json` but wrapped in a json structure specifying the module name
* `module-jsonl`, sam eas `module-json`, but on a single line.

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
