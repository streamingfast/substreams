package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// openRecordingStore records the objects opened through it and through its sub stores.
type openRecordingStore struct {
	dstore.Store
	opened *sync.Map
}

func (s *openRecordingStore) OpenObject(ctx context.Context, name string) (io.ReadCloser, error) {
	s.opened.Store(name, true)
	return s.Store.OpenObject(ctx, name)
}

func (s *openRecordingStore) SubStore(subFolder string) (dstore.Store, error) {
	sub, err := s.Store.SubStore(subFolder)
	if err != nil {
		return nil, err
	}
	return &openRecordingStore{Store: sub, opened: s.opened}, nil
}

// newLocalTestConfig returns a store config over zstd-compressed files on local disk, the
// way tier1 writes states, and the record of the objects opened through it.
func newLocalTestConfig(t *testing.T) (*Config, *sync.Map) {
	t.Helper()
	base, err := dstore.NewStore("file://"+t.TempDir(), "zst", "zstd", true)
	require.NoError(t, err)
	opened := &sync.Map{}
	cfg, err := NewConfig("test", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, "string", &openRecordingStore{Store: base, opened: opened}, nil, 0, "", "")
	require.NoError(t, err)
	return cfg, opened
}

func writePartial(t *testing.T, cfg *Config, start, end uint64, fill func(p *PartialKV)) *FileInfo {
	t.Helper()
	p := cfg.NewPartialKV(start, zap.NewNop())
	if fill != nil {
		fill(p)
		require.NoError(t, p.Flush())
	}
	file, writer, err := p.Save(end)
	require.NoError(t, err)
	require.NoError(t, writer.Write(context.Background()))
	return file
}

func TestEmptyPartialKVs(t *testing.T) {
	ctx := context.Background()
	cfg, opened := newLocalTestConfig(t)

	empty := writePartial(t, cfg, 0, 10, nil)
	withKey := writePartial(t, cfg, 10, 20, func(p *PartialKV) { p.Set(0, "k", "v") })
	withDeletedPrefix := writePartial(t, cfg, 20, 30, func(p *PartialKV) { p.DeletePrefix(0, "k") })
	large := writePartial(t, cfg, 30, 40, func(p *PartialKV) {
		for i := range 20 {
			sum := sha256.Sum256([]byte(fmt.Sprint(i)))
			p.Set(0, hex.EncodeToString(sum[:]), hex.EncodeToString(sum[:]))
		}
	})
	missing := NewPartialFileInfo("test", 40, 50)

	emptySize, err := cfg.FileSize(ctx, empty)
	require.NoError(t, err)
	require.LessOrEqual(t, emptySize, int64(maxEmptyPartialSize), "an empty partial must be small enough to be checked")
	largeSize, err := cfg.FileSize(ctx, large)
	require.NoError(t, err)
	require.Greater(t, largeSize, int64(maxEmptyPartialSize))

	got, err := cfg.EmptyPartialKVs(ctx, []*FileInfo{empty, withKey, withDeletedPrefix, large, missing})
	require.NoError(t, err)

	assert.Equal(t, map[string]bool{empty.Filename: true}, got)
	_, largeOpened := opened.Load(large.Filename)
	assert.False(t, largeOpened, "a partial above the size limit is never read")
}

func TestEmptyPartialKVsAcrossListingPrefixes(t *testing.T) {
	ctx := context.Background()
	cfg, _ := newLocalTestConfig(t)

	// end blocks 990 to 1020 are listed under two prefixes, 0000000 and 0000001
	var files []*FileInfo
	for start := uint64(980); start < 1020; start += 10 {
		files = append(files, writePartial(t, cfg, start, start+10, nil))
	}
	notAsked := writePartial(t, cfg, 1020, 1030, nil)

	got, err := cfg.EmptyPartialKVs(ctx, files)
	require.NoError(t, err)

	want := make(map[string]bool)
	for _, f := range files {
		want[f.Filename] = true
	}
	assert.Equal(t, want, got)
	assert.NotContains(t, got, notAsked.Filename)
}

func TestEmptyPartialKVsNoFiles(t *testing.T) {
	cfg, _ := newLocalTestConfig(t)
	got, err := cfg.EmptyPartialKVs(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListingPrefixes(t *testing.T) {
	tests := []struct {
		first, last string
		want        []string
	}{
		{"0000000010", "0000000010", []string{"0000000010"}},
		{"0000000010", "0000000090", []string{"000000001", "000000002", "000000003", "000000004", "000000005", "000000006", "000000007", "000000008", "000000009"}},
		{"0000000990", "0000001020", []string{"0000000", "0000001"}},
		{"0003051000", "0004050000", []string{"0003", "0004"}},
		{"999", "1000", []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.first+"-"+tt.last, func(t *testing.T) {
			assert.Equal(t, tt.want, listingPrefixes(tt.first, tt.last))
		})
	}
}

func TestPartialFileName(t *testing.T) {
	name, ok := partialFileName("0000000010-0000000000.partial")
	assert.True(t, ok)
	assert.Equal(t, "0000000010-0000000000.partial", name)

	name, ok = partialFileName("0000000010-0000000000.partial.zst")
	assert.True(t, ok)
	assert.Equal(t, "0000000010-0000000000.partial", name, "a listing may keep the store extension")

	_, ok = partialFileName("0000000010-0000000000.kv")
	assert.False(t, ok)
}

func TestCopyFullKV(t *testing.T) {
	ctx := context.Background()
	cfg, _ := newLocalTestConfig(t)

	full := cfg.NewFullKV(zap.NewNop())
	full.Set(0, "a", "1")
	full.Set(0, "b", "2")
	require.NoError(t, full.Flush())
	_, writer, err := full.Save(10)
	require.NoError(t, err)
	require.NoError(t, writer.Write(ctx))
	full.Close()

	require.NoError(t, cfg.CopyFullKV(ctx, 10, 20))

	copied := cfg.NewFullKV(zap.NewNop())
	defer copied.Close()
	require.NoError(t, copied.Load(ctx, NewCompleteFileInfo("test", 0, 20)))
	got := map[string]string{}
	require.NoError(t, copied.Iter(func(key string, value []byte) error {
		got[key] = string(value)
		return nil
	}))
	assert.Equal(t, map[string]string{"a": "1", "b": "2"}, got)
}

func TestDeletePartialKVs(t *testing.T) {
	ctx := context.Background()
	cfg, _ := newLocalTestConfig(t)
	first := writePartial(t, cfg, 0, 10, nil)
	second := writePartial(t, cfg, 10, 20, nil)

	cfg.DeletePartialKVs(ctx, []*FileInfo{first, second, NewPartialFileInfo("test", 20, 30)})

	for _, f := range []*FileInfo{first, second} {
		exists, err := cfg.ExistsPartialKV(ctx, f.Range.StartBlock, f.Range.ExclusiveEndBlock)
		require.NoError(t, err)
		assert.False(t, exists, f.Filename)
	}
}
