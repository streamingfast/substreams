# Substreams Sink ProtoJSON

* Current implementation still lives in [substreams-sink-files](https://github.com/streamingfast/substreams-sink-files)

# Example

* Extract USDT events to ProtoJSONL of a 200 blocks range, in 100-blocks-chunks
  ```
  substreams sink protojson substreams_ethereum_usdt@v0.1.0 map_events -o ./output --filter=.transfers[] -n 100 -s 20000000 -t +200
  ```
