package pipeline

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/streamingfast/dstore"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/pipeline/exec"
	store2 "github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// s3RequestCounter counts the requests going through to an S3 endpoint, by kind.
type s3RequestCounter struct {
	mu     sync.Mutex
	counts map[string]int
	sent   atomic.Int64
}

func (c *s3RequestCounter) count(request *http.Request) {
	kind := request.Method
	if request.Header.Get("x-amz-copy-source") != "" {
		kind = "COPY"
	}
	c.mu.Lock()
	c.counts[kind]++
	c.mu.Unlock()
}

func (c *s3RequestCounter) snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int, len(c.counts))
	for kind, count := range c.counts {
		out[kind] = count
	}
	return out
}

// TestKVLoadMetadataWrites saves one full KV store to an S3 endpoint, loads it through
// setupSubrequestStores as tier2 does, and checks that no load writes the file's metadata back,
// since the file was saved with its size: on S3 dstore implements a metadata write as a copy of
// the whole object onto itself. It logs the requests the loads took. Run it against any S3
// server, for example:
//
//	docker run -d --name bench-s3 -p 18333:8333 chrislusf/seaweedfs server -s3 -dir=/data
//	curl -X PUT http://localhost:18333/bench
//	SUBSTREAMS_TEST_S3_ENDPOINT=http://localhost:18333 go test ./pipeline -run TestKVLoadMetadataWrites -v
//
// SUBSTREAMS_TEST_S3_KV_MB (default 64) sets the store size, SUBSTREAMS_TEST_S3_LOADS (default 20)
// how many times it is loaded, SUBSTREAMS_TEST_S3_PAUSE_MS (default 0) the pause between loads.
func TestKVLoadMetadataWrites(t *testing.T) {
	endpoint := os.Getenv("SUBSTREAMS_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("SUBSTREAMS_TEST_S3_ENDPOINT not set")
	}
	sizeMB := envInt(t, "SUBSTREAMS_TEST_S3_KV_MB", 64)
	loads := envInt(t, "SUBSTREAMS_TEST_S3_LOADS", 20)
	pause := time.Duration(envInt(t, "SUBSTREAMS_TEST_S3_PAUSE_MS", 0)) * time.Millisecond

	target, err := url.Parse(endpoint)
	require.NoError(t, err)
	counter := &s3RequestCounter{counts: map[string]int{}}
	proxy := httputil.NewSingleHostReverseProxy(target)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.count(r)
		proxy.ServeHTTP(w, r)
	}))
	defer server.Close()

	storeURL := fmt.Sprintf("s3://%s/bench/run-%d?region=none&insecure=true&access_key_id=bench&secret_access_key=bench", server.Listener.Addr(), time.Now().UnixNano())
	objStore, err := dstore.NewStore(storeURL, "", "zstd", true)
	require.NoError(t, err)

	confMap := make(store2.ConfigMap)
	for _, name := range []string{"mod1", "mod2"} {
		conf, err := store2.NewConfig(name, 0, name, pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", objStore, nil, 0, "", "")
		require.NoError(t, err)
		confMap[name] = conf
	}

	// one full KV of about sizeMB, random so that compression keeps its size
	fullKV := confMap["mod1"].NewFullKV(zap.NewNop())
	value := make([]byte, 64*1024)
	for i := 0; i < sizeMB*16; i++ {
		_, _ = rand.Read(value)
		fullKV.Set(0, fmt.Sprintf("key-%08d", i), string(value))
	}
	require.NoError(t, fullKV.Flush())
	_, writer, err := fullKV.Save(1000)
	require.NoError(t, err)
	require.NoError(t, writer.Write(context.Background()))
	fullKV.Close()

	storeKind := &pbsubstreams.Module_KindStore_{KindStore: &pbsubstreams.Module_KindStore{}}
	p := Pipeline{
		stores: &Stores{configs: confMap},
		executionStages: exec.ExecutionStages{
			exec.StageLayers{exec.LayerModules{&pbsubstreams.Module{Name: "mod1", Kind: storeKind}}},
			exec.StageLayers{exec.LayerModules{&pbsubstreams.Module{Name: "mod2", Kind: storeKind}}},
		},
	}

	before := counter.snapshot()
	start := time.Now()
	for i := 0; i < loads; i++ {
		if i > 0 {
			time.Sleep(pause)
		}
		storeMap, err := p.setupSubrequestStores(withTestRequest(t, "mod2", 1000))
		require.NoError(t, err)
		_, isFull := storeMap["mod1"].(*store2.FullKV)
		require.True(t, isFull, "mod1 must be loaded as a full KV")
		for _, s := range storeMap {
			if closer, ok := s.(io.Closer); ok {
				closer.Close()
			}
		}
	}
	elapsed := time.Since(start)

	// metadata writes are detached, wait until no request has gone through for 2s
	for last := counter.snapshot(); ; {
		time.Sleep(2 * time.Second)
		now := counter.snapshot()
		if fmt.Sprint(now) == fmt.Sprint(last) {
			break
		}
		last = now
	}
	after := counter.snapshot()

	attrs, err := objStore.SubStore("mod1/states")
	require.NoError(t, err)
	var objectSize int64
	require.NoError(t, attrs.WalkAttributes(context.Background(), "", func(entry dstore.ObjectEntry) error {
		objectSize += entry.Size
		return nil
	}))

	t.Logf("store file: %.1f MiB, loads: %d, load time: %s", float64(objectSize)/(1<<20), loads, elapsed.Round(time.Millisecond))
	for _, kind := range []string{"HEAD", "GET", "PUT", "COPY"} {
		t.Logf("%-4s requests during the loads: %d", kind, after[kind]-before[kind])
	}
	t.Logf("each COPY is a CopyObject of the %.1f MiB file onto itself", float64(objectSize)/(1<<20))
	require.Zero(t, after["COPY"]-before["COPY"], "loading a file saved with its size must not write its metadata")
}

func envInt(t *testing.T, name string, fallback int) int {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	require.NoError(t, err, name)
	return value
}
