package pubsub

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"github.com/cenkalti/backoff/v4"
	"go.uber.org/zap"
)

// AttributeType is set on every message so a subscriber can tell a block
// payload from a batch or a reorg notification.
const AttributeType = "type"

const (
	// TypeBlock is the attribute used by a single-block payload kept from an
	// older pending file. Block messages are published as TypeBatch.
	TypeBlock = "block"
	// TypeBatch is one BatchPayload. The JSON matches the webhook sink's batch
	// body, including a message that holds one block.
	TypeBatch = "batch"
	// TypeUndo is one UndoPayload. The JSON matches the webhook sink.
	TypeUndo = "undo"
)

// Message is one publish. OrderingKey is the output module name for the
// webhook JSON. A module that emits Publish uses the substreams-sink-pubsub
// key instead: the zero-padded block number and the message index.
type Message struct {
	Data        []byte            `json:"data,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	OrderingKey string            `json:"ordering_key,omitempty"`
}

// Publisher publishes one message, retrying transient failures. A *DeliveryError
// means every attempt failed.
type Publisher interface {
	Publish(ctx context.Context, msg Message) error
}

// RetryConfig bounds one publish. MaxRetries is the number of retries after the
// first attempt; -1 retries until ctx is done. Timeout is the deadline of one
// attempt. MaxInterval caps the wait between attempts.
type RetryConfig struct {
	Timeout     time.Duration
	MaxRetries  int
	MaxInterval time.Duration
}

// DeliveryError is returned once every attempt to publish a payload has failed.
type DeliveryError struct {
	Project     string
	Topic       string
	BlockNumber uint64
	Attempts    int
	Err         error
}

func (e *DeliveryError) Error() string {
	return fmt.Sprintf("pubsub delivery of block %d to %s/%s failed after %d attempt(s): %v", e.BlockNumber, e.Project, e.Topic, e.Attempts, e.Err)
}

func (e *DeliveryError) Unwrap() error { return e.Err }

// orderedTopic is the piece of a Pub/Sub topic the retry loop needs. A failed
// publish pauses the ordering key until ResumePublish, so the next attempt
// (and the next block, in skip mode) would be rejected without it.
type orderedTopic interface {
	publish(ctx context.Context, msg *pubsub.Message) (string, error)
	ResumePublish(orderingKey string)
}

type gcpTopic struct{ publisher *pubsub.Publisher }

func (g gcpTopic) publish(ctx context.Context, msg *pubsub.Message) (string, error) {
	return g.publisher.Publish(ctx, msg).Get(ctx)
}

func (g gcpTopic) ResumePublish(orderingKey string) { g.publisher.ResumePublish(orderingKey) }

// configureTopic turns ordering on and applies the per-attempt timeout.
// Ordering has to be enabled before the first publish or the client rejects
// an ordering key.
func configureTopic(publisher *pubsub.Publisher, timeout time.Duration) {
	publisher.EnableMessageOrdering = true
	if timeout > 0 {
		publisher.PublishSettings.Timeout = timeout
	}
}

// GCPPublisher publishes to one topic with message ordering enabled.
type GCPPublisher struct {
	client    *pubsub.Client
	publisher *pubsub.Publisher
	retry     *retryPublisher
}

// NewGCPPublisher dials Pub/Sub for projectID. PUBSUB_EMULATOR_HOST, when set,
// selects the emulator; otherwise application default credentials are used.
func NewGCPPublisher(ctx context.Context, projectID, topicID string, cfg RetryConfig, logger *zap.Logger) (*GCPPublisher, error) {
	if cfg.MaxRetries < -1 {
		return nil, fmt.Errorf("max retries must be -1 (infinite) or greater")
	}
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("creating pubsub client: %w", err)
	}
	publisher := client.Publisher(topicID)
	configureTopic(publisher, cfg.Timeout)
	return &GCPPublisher{
		client:    client,
		publisher: publisher,
		retry:     newRetryPublisher(gcpTopic{publisher}, projectID, topicID, cfg, logger),
	}, nil
}

// Publish publishes msg, retrying within RetryConfig.
func (p *GCPPublisher) Publish(ctx context.Context, msg Message) error {
	return p.retry.Publish(ctx, msg)
}

// Close flushes the topic and closes the client.
func (p *GCPPublisher) Close() error {
	p.publisher.Stop()
	return p.client.Close()
}

type retryPublisher struct {
	topic       orderedTopic
	project     string
	topicID     string
	timeout     time.Duration
	maxRetries  int
	maxInterval time.Duration
	logger      *zap.Logger
}

func newRetryPublisher(topic orderedTopic, project, topicID string, cfg RetryConfig, logger *zap.Logger) *retryPublisher {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &retryPublisher{
		topic:       topic,
		project:     project,
		topicID:     topicID,
		timeout:     cfg.Timeout,
		maxRetries:  cfg.MaxRetries,
		maxInterval: cfg.MaxInterval,
		logger:      logger,
	}
}

func (p *retryPublisher) Publish(ctx context.Context, msg Message) error {
	attempts := 0
	operation := func() error {
		attempts++
		attemptCtx := ctx
		var cancel context.CancelFunc
		if p.timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, p.timeout)
			defer cancel()
		}

		_, err := p.topic.publish(attemptCtx, &pubsub.Message{
			Data:        msg.Data,
			Attributes:  msg.Attributes,
			OrderingKey: msg.OrderingKey,
		})
		if err == nil {
			return nil
		}
		// Resume even on the last attempt: skip mode publishes the next block
		// on the same key, and a paused key rejects it.
		if msg.OrderingKey != "" {
			p.topic.ResumePublish(msg.OrderingKey)
		}
		if ctx.Err() != nil {
			return backoff.Permanent(ctx.Err())
		}
		return err
	}

	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 0
	if p.maxInterval > 0 {
		b.MaxInterval = p.maxInterval
		if b.InitialInterval > b.MaxInterval {
			b.InitialInterval = b.MaxInterval
		}
		b.Reset()
	}

	var retryBackoff backoff.BackOff
	if p.maxRetries == -1 {
		retryBackoff = b
	} else {
		retryBackoff = backoff.WithMaxRetries(b, uint64(p.maxRetries))
	}
	retryBackoff = backoff.WithContext(retryBackoff, ctx)

	err := backoff.Retry(operation, retryBackoff)
	if err != nil {
		p.logger.Warn("pubsub publish failed after all retries",
			zap.Error(err),
			zap.Int("attempts", attempts),
			zap.String("project", p.project),
			zap.String("topic", p.topicID))
		return &DeliveryError{Project: p.project, Topic: p.topicID, Attempts: attempts, Err: err}
	}
	return nil
}
