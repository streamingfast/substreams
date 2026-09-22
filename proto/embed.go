package sfproto

import (
	_ "embed"
)

var OptionsPath = "sf/substreams/options.proto"

//go:embed sf/substreams/options.proto
var OptionsSource []byte

// SQL sink schema annotations. Like options.proto, protoparse needs the source
// on disk to honour the extensions, and a manifest that imports it has no copy.
var SQLSchemaPath = "sf/substreams/sink/sql/schema/v1/schema.proto"

//go:embed sf/substreams/sink/sql/schema/v1/schema.proto
var SQLSchemaSource []byte
