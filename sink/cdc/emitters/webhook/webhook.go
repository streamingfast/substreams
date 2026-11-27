// Package webhook registers the `webhook://` (http) and `webhooks://` (https) emitters: change events POSTed
// as JSON to an HTTP endpoint. Importing the package registers them.
//
// It reuses the retry, backoff, auth and signing mechanics of the webhook sink ([webhook.Client]).
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/streamingfast/substreams/sink/cdc"
	"github.com/streamingfast/substreams/sink/cdc/emitters/params"
	sinkwebhook "github.com/streamingfast/substreams/sink/webhook"
	"go.uber.org/zap"
)

const (
	DefaultMaxEvents = 500
	DefaultMaxBytes  = 4 << 20
)

func init() {
	cdc.RegisterEmitter("webhook", factory("http"))
	cdc.RegisterEmitter("webhooks", factory("https"))
}

func factory(scheme string) cdc.EmitterFactory {
	return func(_ context.Context, u *url.URL, opts cdc.EmitterOptions) (cdc.Emitter, error) {
		q := params.New(u)
		cfg := Config{
			Client: sinkwebhook.Config{
				Timeout:         q.Duration("timeout", 30*time.Second),
				MaxRetries:      q.Int("max-retries", -1),
				MaxInterval:     q.Duration("max-interval", 30*time.Second),
				AuthHeaderName:  q.String("auth-header", ""),
				AuthHeaderValue: q.String("auth", ""),
				SigningSecret:   q.String("signing-secret", ""),
				Headers:         q.WithPrefix("header."),
			},
			MaxEvents: q.Int("max-events", DefaultMaxEvents),
			MaxBytes:  q.Int("max-bytes", DefaultMaxBytes),
		}
		if err := q.Err(); err != nil {
			return nil, err
		}

		// What is not ours goes to the receiver.
		target := *u
		target.Scheme = scheme
		target.RawQuery = q.Unused().Encode()
		cfg.URL = target.String()
		return New(cfg, opts.Logger)
	}
}

// Config configures an [Emitter].
type Config struct {
	// URL is the full http(s) URL the events are POSTed to.
	URL    string
	Client sinkwebhook.Config
	// MaxEvents and MaxBytes cap one request, a larger batch is sent as several requests, in order.
	MaxEvents int
	MaxBytes  int
}

// Payload is the JSON body of one request.
type Payload struct {
	Block cdc.Block `json:"block"`
	// Cursor follows the batch, it is the same on every part of a batch.
	Cursor string `json:"cursor"`
	// Compensation is true when the events revert earlier ones after an undo signal.
	Compensation bool `json:"compensation,omitempty"`
	// Part and Parts are set (1-based) when the batch is split over several requests.
	Part   int               `json:"part,omitempty"`
	Parts  int               `json:"parts,omitempty"`
	Events []json.RawMessage `json:"events"`
}

// Emitter POSTs each batch, in order, and returns once the receiver answered 2xx.
type Emitter struct {
	cfg    Config
	client *sinkwebhook.Client
}

var _ cdc.Emitter = (*Emitter)(nil)

// New creates the emitter.
func New(cfg Config, logger *zap.Logger) (*Emitter, error) {
	if cfg.URL == "" {
		return nil, errors.New("webhook output needs a host, e.g. webhooks://example.com/hook")
	}
	if cfg.MaxEvents <= 0 || cfg.MaxBytes <= 0 {
		return nil, errors.New("webhook parameters max-events and max-bytes must be greater than 0")
	}
	if cfg.Client.MaxRetries < -1 {
		return nil, errors.New("webhook parameter max-retries must be -1 (infinite) or greater")
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Emitter{cfg: cfg, client: sinkwebhook.NewClient(cfg.Client, logger)}, nil
}

func (e *Emitter) EmitBatch(ctx context.Context, batch *cdc.Batch) error {
	return e.send(ctx, batch, false)
}

func (e *Emitter) EmitCompensation(ctx context.Context, batch *cdc.Batch) error {
	return e.send(ctx, batch, true)
}

func (e *Emitter) send(ctx context.Context, batch *cdc.Batch, compensation bool) error {
	encoded := make([]json.RawMessage, len(batch.Events))
	for i := range batch.Events {
		var err error
		if encoded[i], err = batch.Events[i].EncodeJSON(); err != nil {
			return err
		}
	}

	chunks := e.chunks(encoded)
	for i, chunk := range chunks {
		payload := Payload{Block: batch.Block, Cursor: batch.Cursor, Compensation: compensation, Events: chunk}
		if len(chunks) > 1 {
			payload.Part, payload.Parts = i+1, len(chunks)
		}

		var body bytes.Buffer
		enc := json.NewEncoder(&body)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(&payload); err != nil {
			return fmt.Errorf("encode webhook payload: %w", err)
		}
		if err := e.client.Call(ctx, e.cfg.URL, bytes.TrimRight(body.Bytes(), "\n"), batch.Block.Number); err != nil {
			return err
		}
	}
	return nil
}

// chunks splits events in consecutive groups of at most MaxEvents events and about MaxBytes bytes, a lone
// event larger than MaxBytes goes alone.
func (e *Emitter) chunks(events []json.RawMessage) [][]json.RawMessage {
	var out [][]json.RawMessage
	start, size := 0, 0
	for i, ev := range events {
		if i > start && (i-start >= e.cfg.MaxEvents || size+len(ev) > e.cfg.MaxBytes) {
			out = append(out, events[start:i])
			start, size = i, 0
		}
		size += len(ev)
	}
	if start < len(events) {
		out = append(out, events[start:])
	}
	return out
}

// Flush is a no-op, nothing is buffered across calls.
func (e *Emitter) Flush(context.Context) error { return nil }

func (e *Emitter) Close() error { return nil }
