# Inputs

A `map` and `store` module can define one or multiple inputs. The possible inputs are `map`, `store`, and `source.`

#### `Source`

Input of type `source` protobuf objects that represent native objects. Currently, Substreams only support blockchain blocks, for Ethereum Substreams that would be `sf.ethereum.type.v1.Block.`

```yaml
  inputs:
    - source: sf.ethereum.type.v1.Block
```

#### `Map`

Input of type `map` is the the output of another `map` module. The type of the object would be type defined in the `output` attribute of the `map` module. **A map module cannot depend on itself.**

```yaml
  inputs:
    - map: my_map
```

#### `Store`

Input of type `store` is the the state of another store, there are two possible `mode` that you can define.

* `get`: in `get` mode you will be provided with a key/value store that is guaranteed to be synced up to the block being processed, readily queryable.
* `delta`: in `delta` mode you will be provided with all the changes that occurred in the `store` module that occurred at the given block.

If the  `mode` is not defined in the Manifest it will default to `get` mode.&#x20;

Stores that are passed as `inputs` are only read only and not writeable. **A store module cannot depend on, since it needs to have the ability to write to it.**&#x20;

```yaml
  inputs:
    - store: my_store
      mode: deltas
    - store: my_store # defaults to mode: get
```
