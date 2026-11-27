// Package pubsub registers the `pubsub://<project>/<topic>` emitter: one Google Cloud Pub/Sub message per
// change event. Importing the package registers it.
//
// It reuses the publisher of the pubsub sink ([sinkpubsub.GCPPublisher]): retries with backoff, ordering key
// resume, `PUBSUB_EMULATOR_HOST` support.
//
// # Ordering
//
// Messages carry an ordering key, so Pub/Sub delivers messages of the same key to a subscription in the order
// they were published (the topic must be consumed from a subscription with message ordering enabled, and
// publishing to a single region keeps the guarantee). The key is picked with the `ordering` parameter:
//
//   - `table` (default): all events of a table share a key, a subscriber sees a table's changes, tombstones
//     included, in stream order. Throughput of one table is bounded by what one ordering key sustains
//     (1 MB/s on Pub/Sub).
//   - `key`: the key is the table plus the row primary key, so changes to one row are ordered and rows are
//     independent. Higher throughput, no order between rows.
//   - `none`: no ordering key, no order at all.
//
// A compensation tombstone (`op=d`, `revert_of` set) carries the table and key of the event it reverts, so in
// both `table` and `key` modes it lands on the same ordering key as the event it reverts and is delivered after it.
package pubsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/streamingfast/substreams/sink/cdc"
	"github.com/streamingfast/substreams/sink/cdc/emitters/params"
	sinkpubsub "github.com/streamingfast/substreams/sink/pubsub"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Ordering selects the ordering key of the messages.
type Ordering string

const (
	OrderingTable Ordering = "table"
	OrderingKey   Ordering = "key"
	OrderingNone  Ordering = "none"
)

// DefaultConcurrency is how many ordering keys are published to at once.
const DefaultConcurrency = 8

// publisherFactory builds the publisher, replaced in tests.
var publisherFactory = func(ctx context.Context, project, topic string, cfg sinkpubsub.RetryConfig, logger *zap.Logger) (sinkpubsub.Publisher, closer, error) {
	p, err := sinkpubsub.NewGCPPublisher(ctx, project, topic, cfg, logger)
	if err != nil {
		return nil, nil, err
	}
	return p, p, nil
}

type closer interface{ Close() error }

func init() {
	cdc.RegisterEmitter("pubsub", func(ctx context.Context, u *url.URL, opts cdc.EmitterOptions) (cdc.Emitter, error) {
		project, topic, err := sinkpubsub.ParseTopic(u.Host, strings.TrimPrefix(u.Path, "/"))
		if err != nil {
			return nil, fmt.Errorf("pubsub:// url must be pubsub://<project>/<topic>: %w", err)
		}

		q := params.New(u)
		ordering := Ordering(q.String("ordering", string(OrderingTable)))
		concurrency := q.Int("concurrency", DefaultConcurrency)
		retry := sinkpubsub.RetryConfig{
			Timeout:     q.Duration("timeout", 30*time.Second),
			MaxRetries:  q.Int("max-retries", -1),
			MaxInterval: q.Duration("max-interval", 30*time.Second),
		}
		if err := q.Err(); err != nil {
			return nil, err
		}
		if unused := q.Unused(); len(unused) > 0 {
			return nil, fmt.Errorf("unknown pubsub:// parameters %s, supported: ordering, concurrency, timeout, max-retries, max-interval", unused.Encode())
		}

		publisher, closer, err := publisherFactory(ctx, project, topic, retry, opts.Logger)
		if err != nil {
			return nil, err
		}
		e, err := New(publisher, closer, ordering, concurrency)
		if err != nil {
			if closer != nil {
				_ = closer.Close()
			}
			return nil, err
		}
		return e, nil
	})
}

// Emitter publishes one message per event.
type Emitter struct {
	publisher   sinkpubsub.Publisher
	closer      closer
	ordering    Ordering
	concurrency int
}

var _ cdc.Emitter = (*Emitter)(nil)

// New creates the emitter on top of publisher, closer (optional) is closed with the emitter.
func New(publisher sinkpubsub.Publisher, closer closer, ordering Ordering, concurrency int) (*Emitter, error) {
	if !slices.Contains([]Ordering{OrderingTable, OrderingKey, OrderingNone}, ordering) {
		return nil, fmt.Errorf("pubsub parameter ordering must be table, key or none, got %q", ordering)
	}
	if concurrency <= 0 {
		return nil, errors.New("pubsub parameter concurrency must be greater than 0")
	}
	return &Emitter{publisher: publisher, closer: closer, ordering: ordering, concurrency: concurrency}, nil
}

// OrderingKey returns the ordering key of ev for the configured ordering.
func (e *Emitter) OrderingKey(ev *cdc.Event) string {
	switch e.ordering {
	case OrderingTable:
		return ev.Source.Table
	case OrderingKey:
		// json.Marshal sorts map keys, so the key is stable.
		encoded, _ := json.Marshal(ev.Key)
		return ev.Source.Table + "/" + string(encoded)
	default:
		return ""
	}
}

func (e *Emitter) message(ev *cdc.Event) (sinkpubsub.Message, error) {
	data, err := ev.EncodeJSON()
	if err != nil {
		return sinkpubsub.Message{}, err
	}
	attrs := map[string]string{
		"op":        string(ev.Op),
		"table":     ev.Source.Table,
		"block_num": strconv.FormatUint(ev.Source.Block.Number, 10),
		"id":        ev.ID,
	}
	if ev.RevertOf != "" {
		attrs["revert_of"] = ev.RevertOf
	}
	return sinkpubsub.Message{Data: data, Attributes: attrs, OrderingKey: e.OrderingKey(ev)}, nil
}

func (e *Emitter) EmitBatch(ctx context.Context, batch *cdc.Batch) error {
	return e.publish(ctx, batch)
}

func (e *Emitter) EmitCompensation(ctx context.Context, batch *cdc.Batch) error {
	return e.publish(ctx, batch)
}

// publish sends the events of a batch: events of one ordering key are published one after the other, in
// order, different keys concurrently. It returns once every message is acknowledged by Pub/Sub.
func (e *Emitter) publish(ctx context.Context, batch *cdc.Batch) error {
	var keys []string
	groups := map[string][]sinkpubsub.Message{}
	for i := range batch.Events {
		msg, err := e.message(&batch.Events[i])
		if err != nil {
			return err
		}
		group := msg.OrderingKey
		if group == "" {
			// Unordered messages are all independent: one group each.
			group = "\x00" + strconv.Itoa(i)
		}
		if _, found := groups[group]; !found {
			keys = append(keys, group)
		}
		groups[group] = append(groups[group], msg)
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(e.concurrency)
	for _, key := range keys {
		msgs := groups[key]
		g.Go(func() error {
			for _, msg := range msgs {
				if err := e.publisher.Publish(ctx, msg); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return g.Wait()
}

// Flush is a no-op, publishing is synchronous.
func (e *Emitter) Flush(context.Context) error { return nil }

func (e *Emitter) Close() error {
	if e.closer == nil {
		return nil
	}
	return e.closer.Close()
}
