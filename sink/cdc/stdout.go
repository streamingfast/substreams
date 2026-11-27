package cdc

import (
	"bufio"
	"context"
	"net/url"
	"os"
	"sync"
)

func init() {
	RegisterEmitter("stdout", func(_ context.Context, _ *url.URL, opts EmitterOptions) (Emitter, error) {
		out := opts.Stdout
		if out == nil {
			out = os.Stdout
		}
		return &stdoutEmitter{w: bufio.NewWriter(out)}, nil
	})
}

// stdoutEmitter writes one JSON line per event.
type stdoutEmitter struct {
	mu sync.Mutex
	w  *bufio.Writer
}

func (e *stdoutEmitter) EmitBatch(_ context.Context, batch *Batch) error {
	return e.write(batch)
}

func (e *stdoutEmitter) EmitCompensation(_ context.Context, batch *Batch) error {
	return e.write(batch)
}

func (e *stdoutEmitter) write(batch *Batch) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var line []byte
	for i := range batch.Events {
		var err error
		line, err = batch.Events[i].AppendJSONL(line[:0])
		if err != nil {
			return err
		}
		if _, err := e.w.Write(line); err != nil {
			return err
		}
	}
	return e.w.Flush()
}

func (e *stdoutEmitter) Flush(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.w.Flush()
}

func (e *stdoutEmitter) Close() error { return e.Flush(context.Background()) }
