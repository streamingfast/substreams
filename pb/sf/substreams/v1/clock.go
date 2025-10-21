package pbsubstreams

import "github.com/streamingfast/bstream"

func (c *Clock) AsBlockRef() bstream.BlockRef {
	return bstream.NewBlockRef(c.Id, c.Number)
}
