# order Substreams modules

This package was initialized via `substreams init`, using the `sol-minimal` template.

## Usage

```bash
substreams build
substreams auth
substreams gui       			  # Get streaming!
```

Optionally, you can publish your Substreams to the [Substreams Registry](https://substreams.dev).

```bash
substreams registry login         # Login to substreams.dev
substreams registry publish       # Publish your Substreams to substreams.dev
```

## Modules

### `map_my_data`

This module will do a simple computation of the number of **transactions**
and the number of **instructions** in each block.
