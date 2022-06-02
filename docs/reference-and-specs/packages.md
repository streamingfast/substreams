# Packages

A Substreams _package_ is a **single file** containing all dependencies, protobuf definitions (as FileDescriptors), compiled WASM code and the modules tree specifications. They allow you to start streaming right away!

Their conventional extension is `.spkg`.

They are protobuf-serialized files that use this [model](../../proto/sf/substreams/v1/package.proto).

You will notice that they conform to both [https://buf.build](https://buf.build) [Images](https://docs.buf.build/reference/images) and standard Protobuf FileDescriptorSet, meaning they can be used with multiple code generation tools as a source of schema definitions.

### Creating packages

You can create a package by running:

```
substreams pack ./substreams.yaml
```

from a Substreams [manifest](manifests.md).

### Dependencies

When `imports` is defined in a new `substreams.yaml`, it can load modules and protobuf definitions from other Substreams packages.

When doing so, **local protobuf filenames will take precedence over the imported package's proto files**. Make sure, therefore, that you use different `.proto` filenames than the ones you import, to avoid conflicts. Namespacing using fully qualified paths is recommended.

### Where to find them

There is currently no single point of reference for Substreams modules. See [https://github.com/streamingfast/substreams-playground](https://github.com/streamingfast/substreams-playground) for now.
