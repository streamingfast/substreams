---
description: StreamingFast Substreams module outputs
---

# Outputs

### Data Outputs

A `map` module can define one output. The output is the protobuf data type the module will produce.

_Note, a `store` module cannot define an output._

```yaml
output:
    type: proto:eth.erc721.v1.Transfers
```

An output object has an attribute `type` that defines the type of the output of the `map` module.
