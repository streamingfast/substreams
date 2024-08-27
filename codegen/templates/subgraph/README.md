# Substreams-powered Subgraph 

## Modules

_Describe important modules here_


## Develop

```bash
npm install
npm run generate  # Generate protobuf bindings
npm run codegen   # Generate subgraph mapping types
npm run build
```

Configure `graph-node` in `.graph-node/config.toml`.

```bash
substreams auth
. ./.substreams.dev  # or insert the SUBSTREAMS_API_TOKEN into the `config.toml` file.
```

Once `graph-node` is ready`, run:

```bash
npm run create-local
npm run deploy-local
npm run remove-local
```

### Query a subgraph

In the devcontainer, you can access the port-forwarded `graph-node` instance at: http://localhost:8000/subgraphs/name/{name_of_your_subgraph}/
