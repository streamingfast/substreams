---
description: StreamingFast Substreams module outputs
---

# Outputs

### Module Data Outputs

A `map` module can define one output, which is the protobuf data type it announces it will produce.

A `store` modules cannot define an output

```yaml
output:
    type: proto:eth.erc721.v1.Transfers
```

An output object has an attribute `type` that defines the type of the output of the `map` module.
