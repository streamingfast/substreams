---
description: StreamingFast Substreams packages reference
---

# Packages

## Substreams packages overview

A Substreams package is a **single file** **containing all necessary dependencies** including:&#x20;

* Protobuf definitions as FileDescriptors
* Compiled WASM code
* Module DAG definition&#x20;

Substreams packages are protobuf-serialized files. The standard extension for Substreams packages is **`.spkg`**.

{% hint style="success" %}
**Tip**: Packages expedite the use of Substreams and allow developers to **begin streaming immediately**_._
{% endhint %}

**Buf images**

[Substreams packages](../../proto/sf/substreams/v1/package.proto) conform to [Buf images](https://docs.buf.build/reference/images) and the standard protobuf FileDescriptorSet. Substreams packages can be used across multiple code generation tools as a source for schema definitions.

### Creating packages

Packages are created by using the `substreams`[`pack`](https://substreams.streamingfast.io/reference-and-specs/command-line-interface#pack) command, passing the Substreams manifest file.

```bash
substreams pack ./substreams.yaml
```

### Package dependencies

Developers can use modules and protobuf definitions from other Substreams packages when `imports` is defined in the manifest.

{% hint style="warning" %}
**Important**: To avoid potential naming collisions select unique `.proto` filenames and namespaces specifying fully qualified paths.
{% endhint %}

Local protobuf filenames take precedence over the imported package's proto files.&#x20;

### Additional information

Learn more about packages and explore examples in the [Substreams Playground](https://github.com/streamingfast/substreams-playground).
