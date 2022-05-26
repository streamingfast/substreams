# Overview

A Substream consists of a few files / folder

* `manifest.yaml` : A YAML file that defines the modules and their dependancy
* `protobuf`: Within your `manifest.yaml` you will define custom types to represent your models, those will be defined in `protobuf` definition files.&#x20;
* `rust handler` : A `src` folder that will contain your Substream rust code of the `module handlers` defined in your `manifest.yaml`

In the upcoming steps we will attempt to build a Substream that tracks ERC721 holder count for a given contract. We have broken down the guide into a few steps:

1. &#x20;[Requirements](requirements.md): We will install all the decencies and setup your environment to create our first Substream
2. [Creating Your Manifest](creating-your-manifest.md): We will setup our first `manifest.yaml` and give you a high level overview of the file
3. [Creating Protobuf Schemas](../../getting-started-guide/creating-protobuf-schemas.md): We will write our first `Protobuf` schema that we will use in our handlers
4. [Writing Module Handlers](../../getting-started-guide/creating-protobuf-schemas-1.md): We will write our first module handler
5. [Running your Substream](../../getting-started-guide/consuming.md): We will run our newly written Substream

