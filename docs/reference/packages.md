# Substreams Packages

* [Packages](packages.md#packages)
  * [Definition](packages.md#definition)
  * [Where to find them](packages.md#where-to-find-them)

### Definition

Packages are single files containing all dependencies, protobuf definitions (as FileDescriptors), compiled WASM code and modules tree specifications. They allow you to start streaming right away!

Their conventional extension is `.spkg`.

They are protobuf-serialized files that use this [model](../../proto/sf/substreams/v1/package.proto)

You will notice that the conform to both [https://buf.build](https://buf.build) [Images](https://docs.buf.build/reference/images) and standard Protobuf FileDescriptorSet, meaning they can be used with multiple code generation tools to scaffold.

### Creating packages

You can create a package by running:

```
substreams pack ./substreams.yaml
```

from a Substreams modules manifest.

### Dependencies

When `imports` is defined in a new `substreams.yaml`, it can load modules and protobuf definitions from other Substreams packages.

When doing so, **local protobuf filenames will take precedence over the imported package's proto files**. Make sure, therefore, that you use different `.proto` filenames then the ones you import, to avoid conflicts.

### Where to find them

There is currently no single point of reference for Substreams modules. See [https://github.com/streamingfast/substreams-playground](https://github.com/streamingfast/substreams-playground) for now.
