package tests_e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2"
	pubsubpb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"github.com/cenkalti/backoff/v4"
	"github.com/streamingfast/logging"
	"github.com/streamingfast/substreams/client"
	"github.com/streamingfast/substreams/sink"
	pubsubsink "github.com/streamingfast/substreams/sink/pubsub"
	"github.com/streamingfast/substreams/tools/devenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var pubsubTestLog, pubsubTestTracer = logging.PackageLogger("tests_e2e_pubsub", "github.com/streamingfast/substreams/tests_e2e/pubsub")

func TestPubSubSink(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	const burst = 200
	container, err := newDummyBlockchainContainer(ctx, tmpDir, latestDummyBlockchainImage, "", burst)
	require.NoError(t, err)
	defer devenv.TerminateDummyBlockchain(ctx, container)
	waitMergerCaughtUp(t, ctx, tmpDir, burst)

	app2, t2Endpoint := startTier2App(t, ctx, tmpDir, zlog)
	app, endpoint := startTier1App(t, ctx, tmpDir, container, t2Endpoint, zlog)
	defer shutdownStack(app, app2)

	srv := pstest.NewServer()
	t.Cleanup(func() { _ = srv.Close() })
	t.Setenv("PUBSUB_EMULATOR_HOST", srv.Addr)

	admin := newPubSubClient(t, ctx)

	t.Run("one message per block", func(t *testing.T) {
		const topicID = "blocks"
		createPubSubTopic(t, ctx, admin, topicID)
		runPubSubSink(t, ctx, endpoint, topicID, 0)

		got := pullPubSub(t, ctx, admin, topicID, 5)
		assert.Equal(t, []uint64{100, 101, 102, 103, 104}, clockNumbers(t, got, false))
		for _, msg := range got {
			assert.Equal(t, pubsubsink.TypeBlock, msg.attrs[pubsubsink.AttributeType])
			assert.Equal(t, "map_events", msg.key)
			assert.Equal(t, "map_events", msg.module)
		}
	})

	t.Run("batch", func(t *testing.T) {
		const topicID = "batches"
		createPubSubTopic(t, ctx, admin, topicID)
		runPubSubSink(t, ctx, endpoint, topicID, 5)

		got := pullPubSub(t, ctx, admin, topicID, 1)
		require.Len(t, got, 1)
		assert.Equal(t, pubsubsink.TypeBatch, got[0].attrs[pubsubsink.AttributeType])
		assert.Equal(t, "map_events", got[0].key)
		assert.Equal(t, []uint64{100, 101, 102, 103, 104}, clockNumbers(t, got, true))
	})
}

func newPubSubClient(t *testing.T, ctx context.Context) *pubsub.Client {
	t.Helper()
	client, err := pubsub.NewClient(ctx, "test-project")
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func createPubSubTopic(t *testing.T, ctx context.Context, client *pubsub.Client, topicID string) {
	t.Helper()
	const project = "test-project"
	topicName := fmt.Sprintf("projects/%s/topics/%s", project, topicID)
	_, err := client.TopicAdminClient.CreateTopic(ctx, &pubsubpb.Topic{Name: topicName})
	require.NoError(t, err)
	_, err = client.SubscriptionAdminClient.CreateSubscription(ctx, &pubsubpb.Subscription{
		Name:                  fmt.Sprintf("projects/%s/subscriptions/%s", project, topicID),
		Topic:                 topicName,
		EnableMessageOrdering: true,
	})
	require.NoError(t, err)
}

func runPubSubSink(t *testing.T, ctx context.Context, endpoint, topicID string, batchMaxBlocks int) {
	t.Helper()
	pkg, module, hash, err := sink.ReadManifestAndModule("./dummy/e2e-v0.1.0.spkg", "", nil, "map_events", sink.IgnoreOutputModuleType, false, nil, pubsubTestLog)
	require.NoError(t, err)

	bo := backoff.NewExponentialBackOff()
	bo.MaxElapsedTime = 0
	sinkerConfig := &sink.SinkerConfig{
		Pkg:              pkg,
		OutputModule:     module,
		OutputModuleHash: hash,
		ClientConfig: client.NewSubstreamsClientConfig(client.SubstreamsClientConfigOptions{
			Endpoint:  endpoint,
			AuthType:  client.None,
			PlainText: true,
			Agent:     "test-pubsub",
		}),
		Mode:            sink.SubstreamsModeDevelopment,
		StartBlock:      100,
		StopBlock:       105,
		MaxRetries:      2,
		BackOff:         bo,
		LivenessChecker: sink.NewCursorBasedLivenessChecker(),
		Logger:          pubsubTestLog,
		Tracer:          pubsubTestTracer,
	}

	pubsubSink, err := pubsubsink.NewSink(ctx, pubsubsink.SinkConfig{
		Project:        "test-project",
		Topic:          topicID,
		StateFile:      filepath.Join(t.TempDir(), "state.cursor"),
		OnFailure:      pubsubsink.OnFailureExit,
		SinkerConfig:   sinkerConfig,
		BatchMaxBlocks: batchMaxBlocks,
		BatchMaxWait:   time.Hour,
		Retry: pubsubsink.RetryConfig{
			Timeout:     15 * time.Second,
			MaxRetries:  2,
			MaxInterval: time.Second,
		},
		Logger: pubsubTestLog,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = pubsubSink.Close() })

	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	require.NoError(t, pubsubSink.Run(runCtx))
}

type pubSubMessage struct {
	data   []byte
	attrs  map[string]string
	key    string
	module string
}

func pullPubSub(t *testing.T, ctx context.Context, client *pubsub.Client, topicID string, n int) []pubSubMessage {
	t.Helper()
	sub := client.Subscriber(topicID)
	recvCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var mu sync.Mutex
	var got []pubSubMessage
	err := sub.Receive(recvCtx, func(_ context.Context, msg *pubsub.Message) {
		var body struct {
			Manifest struct {
				ModuleName string `json:"moduleName"`
			} `json:"manifest"`
		}
		_ = json.Unmarshal(msg.Data, &body)
		item := pubSubMessage{
			data:   append([]byte(nil), msg.Data...),
			attrs:  maps.Clone(msg.Attributes),
			key:    msg.OrderingKey,
			module: body.Manifest.ModuleName,
		}
		msg.Ack()
		mu.Lock()
		got = append(got, item)
		done := len(got) >= n
		mu.Unlock()
		if done {
			cancel()
		}
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		require.NoError(t, err)
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, n)
	return append([]pubSubMessage(nil), got...)
}

func clockNumbers(t *testing.T, msgs []pubSubMessage, batched bool) []uint64 {
	t.Helper()
	var numbers []uint64
	for _, msg := range msgs {
		var body struct {
			Clock struct {
				Number uint64 `json:"number"`
			} `json:"clock"`
			Blocks []struct {
				Clock struct {
					Number uint64 `json:"number"`
				} `json:"clock"`
			} `json:"blocks"`
		}
		require.NoError(t, json.Unmarshal(msg.data, &body))
		if batched {
			for _, block := range body.Blocks {
				numbers = append(numbers, block.Clock.Number)
			}
			continue
		}
		numbers = append(numbers, body.Clock.Number)
	}
	return numbers
}
