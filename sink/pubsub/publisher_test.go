package pubsub

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scriptedTopic fails its first fail publishes. fail < 0 fails every publish.
type scriptedTopic struct {
	mu        sync.Mutex
	fail      int
	publishes int
	resumes   int
	last      *pubsub.Message
}

func (s *scriptedTopic) publish(ctx context.Context, msg *pubsub.Message) (string, error) {
	s.mu.Lock()
	s.publishes++
	s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail < 0 || s.publishes <= s.fail {
		return "", errors.New("unavailable")
	}
	copied := *msg
	s.last = &copied
	return "id", nil
}

func (s *scriptedTopic) ResumePublish(string) {
	s.mu.Lock()
	s.resumes++
	s.mu.Unlock()
}

func (s *scriptedTopic) published() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publishes
}

func (s *scriptedTopic) resumed() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resumes
}

func testRetry(topic orderedTopic, retries int) *retryPublisher {
	return newRetryPublisher(topic, "my-gcp-project", "events", RetryConfig{
		Timeout:     time.Second,
		MaxRetries:  retries,
		MaxInterval: time.Millisecond,
	}, zlogTest)
}

func TestPublish_SuccessDoesNotResume(t *testing.T) {
	topic := &scriptedTopic{}
	err := testRetry(topic, 2).Publish(context.Background(), Message{
		Data:        []byte(`{}`),
		Attributes:  map[string]string{AttributeType: TypeBlock},
		OrderingKey: "map_events",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, topic.published())
	assert.Equal(t, 0, topic.resumed())
	require.NotNil(t, topic.last)
	assert.Equal(t, "map_events", topic.last.OrderingKey)
	assert.Equal(t, TypeBlock, topic.last.Attributes[AttributeType])
	assert.Equal(t, []byte(`{}`), topic.last.Data)
}

func TestPublish_ResumesOrderingKeyBetweenAttempts(t *testing.T) {
	topic := &scriptedTopic{fail: 1}
	err := testRetry(topic, 2).Publish(context.Background(), Message{OrderingKey: "map_events", Data: []byte(`{}`)})
	require.NoError(t, err)
	assert.Equal(t, 2, topic.published())
	assert.Equal(t, 1, topic.resumed(), "a failed attempt pauses the ordering key")
}

func TestPublish_ExhaustedAttemptsResumeAndReport(t *testing.T) {
	topic := &scriptedTopic{fail: -1}
	err := testRetry(topic, 1).Publish(context.Background(), Message{OrderingKey: "map_events"})

	var delivery *DeliveryError
	require.ErrorAs(t, err, &delivery)
	assert.Equal(t, 2, delivery.Attempts)
	assert.Equal(t, "my-gcp-project", delivery.Project)
	assert.Equal(t, "events", delivery.Topic)
	assert.Equal(t, 2, topic.published())
	assert.Equal(t, 2, topic.resumed(), "the key is resumed after the last attempt so the next block can publish")
}

func TestPublish_ZeroRetriesIsOneAttempt(t *testing.T) {
	topic := &scriptedTopic{fail: -1}
	err := testRetry(topic, 0).Publish(context.Background(), Message{OrderingKey: "map_events"})
	var delivery *DeliveryError
	require.ErrorAs(t, err, &delivery)
	assert.Equal(t, 1, delivery.Attempts)
	assert.Equal(t, 1, topic.resumed())
}

func TestPublish_CanceledContextStops(t *testing.T) {
	topic := &scriptedTopic{fail: -1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := testRetry(topic, -1).Publish(ctx, Message{OrderingKey: "map_events"})
	var delivery *DeliveryError
	require.ErrorAs(t, err, &delivery)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, topic.published())
}
