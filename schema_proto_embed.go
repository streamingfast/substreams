package substreams

import _ "embed"

// SQLSchemaProto is the annotations file a from-proto SQL sink's schema is declared with.
//
// It is embedded so `substreams tools extract-proto --sql` can write it next to the proto
// it annotates: the override is parsed with imports resolved from the working directory,
// so the file has to be on disk, and asking the operator to go and find the right version
// of it is most of the work the command exists to remove.
//
//go:embed proto/sf/substreams/sink/sql/schema/v1/schema.proto
var SQLSchemaProto string
