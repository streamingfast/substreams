---
description: StreamingFast Substreams packages reference
---

# Packages

## Substreams packages

A Substreams _package_ is a **single file** containing all dependencies, protobuf definitions (as FileDescriptors), compiled WASM code, and the module's DAG definition.&#x20;

Substreams packages are protobuf-serialized files. The standard extension for Substreams packages is **`.spkg`**.

{% hint style="success" %}
**Tip**: Packages simplify the use of Substreams and allow developers to _begin streaming immediately_!
{% endhint %}

{% hint style="info" %}
**Note**: [Substreams packages](../../proto/sf/substreams/v1/package.proto) conform to [Buf images](https://docs.buf.build/reference/images) and the standard `protobuf` FileDescriptorSe. Substreams packages can be used across multiple code generation tools as a source for schema definitions.
{% endhint %}

### Creating packages

Packages are created by using the `substreams pack` command, passing the Substreams manifest file.

```
substreams pack ./substreams.yaml
```

### Package dependencies

Developers can use modules and protobuf definitions from other Substreams packages when `imports` is defined in the manifest.&#x20;

{% hint style="warning" %}
**Important**: To avoid potential naming collisions select unique `.proto` filenames and namespaces specifying fully qualified paths.
{% endhint %}

{% hint style="info" %}
**Note**: Local protobuf filenames take precedence over the imported package's proto files.&#x20;
{% endhint %}

Additional package examples are available in the [Substreams Playground](https://github.com/streamingfast/substreams-playground).
