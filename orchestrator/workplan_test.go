package orchestrator

import (
	"github.com/streamingfast/substreams/block"
	"strings"
)

var parseRange = block.ParseRange
var parseRanges = block.ParseRanges

func parseSnapshotSpec(in string) *Snapshots {
	out := &Snapshots{}
	if in == "" {
		return out
	}
	for _, el := range strings.Split(in, ",") {
		el = strings.Trim(el, " ")
		partial := strings.Contains(el, "p")
		partRange := block.ParseRange(strings.Trim(el, "p"))
		if partial {
			out.Partials = append(out.Partials, partRange)
		} else {
			out.Completes = append(out.Completes, partRange)
		}
	}
	out.Sort()
	return out
}
