---
description: StreamingFast Substreams sink files
---

# Substreams sink files overview

Overview (bundling N blocks together, line by line to file, entities are extracted and formatted by the Substreams itself any line by line text format supported, work in progress for binary format like Avro, Parquet, etc.)

## Prepare your Substreams

How to respect Sink's expected output's type with examples for JSON (maybe CSV))

## Run and configure substreams-sink-files

(launching, flags, output, inspect, results)
Discussion about where Substreams cursor is saved and importance of persisting this state (save as a .yaml file)

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
