package pubsub

import (
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTopic(t *testing.T) {
	project, topic, err := ParseTopic("my-gcp-project", "events")
	require.NoError(t, err)
	assert.Equal(t, "my-gcp-project", project)
	assert.Equal(t, "events", topic)

	project, topic, err = ParseTopic("", "projects/my-gcp-project/topics/events")
	require.NoError(t, err)
	assert.Equal(t, "my-gcp-project", project)
	assert.Equal(t, "events", topic)

	project, topic, err = ParseTopic("my-gcp-project", "projects/my-gcp-project/topics/events")
	require.NoError(t, err)
	assert.Equal(t, "events", topic)

	_, _, err = ParseTopic("other", "projects/my-gcp-project/topics/events")
	assert.Error(t, err)

	_, _, err = ParseTopic("", "events")
	assert.Error(t, err)

	_, _, err = ParseTopic("my-gcp-project", "")
	assert.Error(t, err)

	_, _, err = ParseTopic("my-gcp-project", "projects/events")
	assert.Error(t, err)
}

func TestConfigureTopic_EnablesOrdering(t *testing.T) {
	publisher := &pubsub.Publisher{}
	configureTopic(publisher, 30*time.Second)
	assert.True(t, publisher.EnableMessageOrdering)
	assert.Equal(t, 30*time.Second, publisher.PublishSettings.Timeout)

	untimed := &pubsub.Publisher{}
	configureTopic(untimed, 0)
	assert.True(t, untimed.EnableMessageOrdering)
	assert.Zero(t, untimed.PublishSettings.Timeout)
}
