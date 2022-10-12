---
description: StreamingFast Substreams module outputs
---

# Outputs

### Data Outputs

A `map` module can define one output. The output is the protobuf data type the module will produce.

{% hint style="info" %}
_**Note:**  `store` modules **cannot** define an output._
{% endhint %}

An output object has an attribute `type` that defines the type of the output for the `map` module. The output definition is found in the manifest for the Substreams implementation.

```yaml
output:
    type: proto:eth.erc721.v1.Transfers
```
