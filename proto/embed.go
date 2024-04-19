package sfproto

import (
	_ "embed"
)


var OptionsPath = "sf/substreams/options.proto"

//go:embed sf/substreams/options.proto
var OptionsSource []byte
