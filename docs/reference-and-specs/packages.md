---
description: StreamingFast Substreams packages reference
---

# Packages

A Substreams _package_ is a **single file** containing all dependencies, `Protobuf` definitions (as FileDescriptors), compiled WASM code, and the modules tree specifications.&#x20;

{% hint style="success" %}
_Tip: Packages allow developers to begin streaming immediately!_
{% endhint %}

Substreams packages are protobuf-serialized files. Their standard extension is **`.spkg`**.

{% hint style="info" %}
The [Substreams packages](../../proto/sf/substreams/v1/package.proto) conform to [Buf images](https://docs.buf.build/reference/images) as well as the standard `protobuf` _FileDescriptorSet_. This means Substreams packages can be used with multiple code generation tools as a source for schema definitions.
{% endhint %}

### Creating Packages

Packages are created using the `substreams pack` command, passing the Substreams manifest file.

```
substreams pack ./substreams.yaml
```

### Dependencies

When `imports` is defined in a new `substreams.yaml`, it can load modules and protobuf definitions from _other_ Substreams packages.

{% hint style="warning" %}
_**Important: local protobuf filenames take precedence over the imported package's proto files in this situation**._&#x20;
{% endhint %}

To avoid conflicts it's important to use unique `.proto` filenames. It's also important to use namespaces with fully qualified paths. These efforts help avoid potential naming collisions.

See the [Substreams Playground](https://github.com/streamingfast/substreams-playground) for package examples.
