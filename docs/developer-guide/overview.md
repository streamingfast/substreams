# Overview

A Substream consists of a few files / folder

* `manifest.yaml` : A YAML file that defines the modules and their dependancy
* `protobuf`: Within your `manifest.yaml` you will define custom types to represent your models, those will be defined in `protobuf` definition files.
* `rust handler` : A `src` folder that will contain your Substream rust code of the `module handlers` defined in your `manifest.yaml`

In the upcoming steps we will attempt to build a Substream that tracks ERC721 holder count for a given contract. We have broken down the guide into a few steps:

1. [Requirements](installation-requirements.md): Install all the dependencies and setup your environment to create your first Substream
2. [Creating Your Manifest](creating-your-manifest.md): Setup your first `manifest.yaml` which gives you a high-level overview of the file
3. [Creating Protobuf Schemas](creating-protobuf-schemas.md): Write your first `Protobuf` schema that will be used in your handlers
4. [Writing Module Handlers](writing-module-handlers.md): Write your first module handler
5. [Running your Substream](running-substreams.md): Run your newly written Substream
