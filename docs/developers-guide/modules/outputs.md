---
description: StreamingFast Substreams module outputs
---

# Output

## Output overview

Substreams `map` modules support a single `output`. The `output` must be a protobuf populated by data acquired inside the `map` module. If the module intends to provide a basic `output` type of a single value, such as a `String` or `bool`, a protobuf is still required. The single value needs to be wrapped in a protobuf for use as the `output` value from a `map` module.

{% hint style="info" %}
**Note:** `store` modules **cannot** define an `output`.
{% endhint %}

An `output` object has a `type` attribute defining the `type` of the `output` for the `map` module. The `output` definition is located in the Substreams manifest, within the module definition.

```yaml
output:
  type: proto:eth.erc721.v1.Transfers
```
