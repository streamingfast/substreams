package orchestrator

import (
	"context"

	"github.com/streamingfast/substreams/storage/store"
)

type NoopMapSquasher struct {
	name string
}

func (n NoopMapSquasher) moduleName() string                                             { return n.name }
func (n NoopMapSquasher) launch(ctx context.Context)                                     {}
func (n NoopMapSquasher) waitForCompletion(ctx context.Context) error                    { return nil }
func (n NoopMapSquasher) squash(ctx context.Context, partialFiles store.FileInfos) error { return nil }
