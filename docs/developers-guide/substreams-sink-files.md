---
description: StreamingFast Substreams sink files
---

# `substreams-sink-files` overview

Overview (bundling N blocks together, line by line to file, entities are extracted and formatted by the Substreams itself any line by line text format supported, work in progress for binary format like Avro, Parquet, etc.)

## Prepare your Substreams

How to respect Sink's expected output's type with examples for JSON (maybe CSV))

## Clone and install `substreams-sink-files`

1. Visit the official `substreams-sink-files` Github repository and clone the project to acquire the required tools and code.

2. Launch a shell session and naviate into the `devel` directory. Start the `substreams-sink-files` installation process by using the `./substreams-sink-files` command.

3. Check the installation to make sure the isntallation is working properly by using the `substreams-sink-files -v` command. A message is printed to the shell session displaying the version of the application that was installed.

```bash
substreams-sink-files version dev
```

## Run and configure `substreams-sink-files`

(launching, flags, output, inspect, results)
Discussion about where Substreams cursor is saved and importance of persisting this state (save as a .yaml file)

## Getting help

To get help with `substreams-sink-files` at any time, use the `substreams-sink-files -h` command. This will print helpful information about the application to the shell session.

```bash
Substreams Sink to Files (JSONL, CSV, etc.)

Usage:
  substreams-sink-files [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  run         Runs extractor code

Flags:
      --delay-before-start duration   [OPERATOR] Amount of time to wait before starting any internal processes, can be used to perform to maintenance on the pod before actually letting it starts
  -h, --help                          help for substreams-sink-files
      --metrics-listen-addr string    [OPERATOR] If non-empty, the process will listen on this address for Prometheus metrics request(s) (default "localhost:9102")
      --pprof-listen-addr string      [OPERATOR] If non-empty, the process will listen on this address for pprof analysis (see https://golang.org/pkg/net/http/pprof/) (default "localhost:6060")
  -v, --version                       version for substreams-sink-files

Use "substreams-sink-files [command] --help" for more information about a command.
```

## Conclusion

---

QUESTIONS:

What are the primary goals of this tutorial? Let's figure out how to write a paragraph to help the reader/dev understand the full implications of this content. It will help guide us through the writing/creation process.

How much data should be extracted to be written to the sink?
What blockchain data fields do we want to use for the example? Do we want to use more than what's available in the Block objects?

What repo/project/code do we want to use as a starting point for the code for this new tutorial?

The steps should be something like:

- Set up initial Substreams project (clone existing? which one?)
- Create module handlers for data extraction (what data do we extract?)
- Test new Substreams project
- Download/acquire SF tool for sinking to files (how does this work exactly?)
- Create protobuf for sink tool? (is this required for files sinks?)
- Run and test sink tool (need commands, etc., they aren't provided anywhere)

NOTES:

Just like Substeams Sink Databases, something that explains in greater detail how somehow can have a Substreams that dump to file. JSONL and CSV will be the first target. Quick tutorial like content, from your Substreams, do this, run that, etc. This is a more advanced tutorial, so we give quick overview of the commands with quick explanation. There need to have some content about how the limitation of this sink which write bundles only when last block of a bundle is final.
