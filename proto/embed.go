package sfproto

import (
	_ "embed"
)

var OptionsPath = "sf/substreams/options.proto"

//go:embed sf/substreams/options.proto
var OptionsSource []byte

// Schema annotations (`(schema.table)`, `(schema.field)`). Like options.proto, protoparse needs the source
// on disk to honour the extensions, and a manifest that imports it has no copy.
var SchemaPath = "sf/substreams/sink/v1/schema.proto"

//go:embed sf/substreams/sink/v1/schema.proto
var SchemaSource []byte

// SQLSchemaPath is the previous location of the schema annotations, kept as an `import public` of SchemaPath so manifests
// that still import it keep resolving.
var SQLSchemaPath = "sf/substreams/sink/sql/schema/v1/schema.proto"

//go:embed sf/substreams/sink/sql/schema/v1/schema.proto
var SQLSchemaSource []byte
