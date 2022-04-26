package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
)

var cacheFileRegex *regexp.Regexp

func init() {
	cacheFileRegex = regexp.MustCompile(`cache-([\d]+)-([\d]+)\.cache`)
}

type CachePerformanceTracker struct {
	manager *Cache
}

func (t *CachePerformanceTracker) startTracking(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				t.log()
				return
			case <-time.After(5 * time.Second):
				t.log()
			}
		}
	}()
}

func (t *CachePerformanceTracker) log() {
	hits := t.manager.totalHits
	misses := t.manager.totalMisses
	if t.manager.currentCache != nil {
		hits += t.manager.currentCache.hits
		misses += t.manager.currentCache.misses
	}

	hitRate := float64(hits) / float64(hits+misses)

	zlog.Info("cache_performance",
		zap.Int("hits", hits),
		zap.Int("misses", misses),
		zap.Float64("hit_rate", hitRate),
	)
}

type CacheKey string

type Cache struct {
	store dstore.Store

	currentCache      *cache
	currentFilename   string
	currentStartBlock uint64
	currentEndBlock   uint64

	totalHits   int
	totalMisses int

	mu sync.RWMutex
}

func NewCache(ctx context.Context, store dstore.Store, startBlock int64) *Cache {
	cf := &Cache{
		store: store,
	}

	if startBlock > 0 {
		cf.currentStartBlock = uint64(startBlock)
	}

	perfTracker := &CachePerformanceTracker{manager: cf}
	perfTracker.startTracking(ctx)

	return cf
}

func (c *Cache) initialize(ctx context.Context, startBlock, endBlock uint64) *cache {
	var filename string
	var found bool

	prefixSearchBlock := startBlock
	if c.currentStartBlock > 0 {
		prefixSearchBlock = c.currentStartBlock
	}

	///search by prefix first...
	c.store.Walk(ctx, cacheFileStartBlockPrefix(prefixSearchBlock), ".tmp", func(fn string) (err error) {
		filename = fn
		found = true
		return nil
	})

	///if not found walk the store, look for closest start block
	if !found {
		closestDiff := uint64(math.MaxUint64)
		c.store.Walk(ctx, cacheFilePrefix(), ".tmp", func(fn string) (err error) {
			fileStart, fileEnd := mustParseCacheFileName(fn)
			if (startBlock < fileEnd) && (startBlock > fileStart) && (startBlock-fileStart) < closestDiff {
				filename = fn
				found = true
			}

			return nil
		})
	}

	if found {
		_, endBlock = mustParseCacheFileName(filename)
		c.currentFilename = filename
	} else {
		fileStartBlock := startBlock
		if c.currentStartBlock > 0 {
			fileStartBlock = c.currentStartBlock
		}
		filename = cacheFileName(fileStartBlock, endBlock)
		c.currentFilename = filename
	}

	c.currentEndBlock = endBlock
	c.load(ctx)

	return c.currentCache
}

func (c *Cache) UpdateCache(ctx context.Context, startBlock, endBlock uint64) {
	var initialize bool

	if c.currentCache == nil {
		initialize = true
	} else if c.currentEndBlock <= math.MaxUint64 && startBlock >= c.currentEndBlock {
		saveStartBlock := startBlock
		if c.currentStartBlock > 0 {
			saveStartBlock = c.currentStartBlock
		}
		c.Save(ctx, saveStartBlock, c.currentEndBlock)
		initialize = true
	}

	if initialize {
		c.initialize(ctx, startBlock, endBlock)
	}
}

func (c *Cache) load(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentCache = newCache()
	c.currentCache.load(ctx, c.store, c.currentFilename)
}

func (c *Cache) Save(ctx context.Context, startBlock uint64, endBlock uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.currentCache == nil {
		return
	}

	saveStartBlock := startBlock
	if c.currentStartBlock > startBlock {
		saveStartBlock = c.currentStartBlock
	}
	c.currentCache.save(ctx, c.store, cacheFileName(saveStartBlock, endBlock))

	c.totalHits += c.currentCache.hits
	c.totalMisses += c.currentCache.misses

	c.currentStartBlock = endBlock
	c.currentCache = nil
}

func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.currentCache.Get(CacheKey(key))
}

func (c *Cache) Set(ctx context.Context, key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.currentCache.Set(CacheKey(key), value)
}

type cache struct {
	kv map[CacheKey][]byte

	hits   int
	misses int
}

func newCache() *cache {
	return &cache{
		kv: make(map[CacheKey][]byte),
	}
}

func (c *cache) load(ctx context.Context, store dstore.Store, filename string) {
	if store == nil {
		zlog.Info("skipping rpccache load: no read store is defined")
		return
	}

	obj, err := store.OpenObject(ctx, filename)
	if err != nil {
		zlog.Info("rpc cache not found", zap.String("filename", filename), zap.String("read_store_url", store.BaseURL().Redacted()), zap.Error(err))
		return
	}

	b, err := ioutil.ReadAll(obj)
	if err != nil {
		zlog.Info("cannot read all rpc cache bytes", zap.String("filename", filename), zap.String("read_store_url", store.BaseURL().Redacted()), zap.Error(err))
		return
	}

	kv := make(map[CacheKey][]byte)
	err = json.Unmarshal(b, &kv)
	if err != nil {
		zlog.Info("cannot unmarshal rpc cache", zap.String("filename", filename), zap.String("read_store_url", store.BaseURL().Redacted()), zap.Error(err))
		return
	}
	c.kv = kv
}

func (c *cache) save(ctx context.Context, store dstore.Store, filename string) {
	if store == nil {
		zlog.Info("skipping rpccache save: no store is defined")
		return
	}

	b, err := json.Marshal(c.kv)
	if err != nil {
		zlog.Info("cannot marshal rpc cache to bytes", zap.Error(err))
		return
	}
	ioreader := bytes.NewReader(b)

	err = store.WriteObject(ctx, filename, ioreader)
	if err != nil {
		zlog.Info("cannot write rpc cache to store", zap.String("filename", filename), zap.String("write_store_url", store.BaseURL().Redacted()), zap.Error(err))
	}

	return
}

func (_ *cache) Key(prefix string, items ...interface{}) CacheKey {
	key := prefix
	for _, it := range items {
		key = fmt.Sprintf("%s:%v", key, it)
	}
	return CacheKey(key)
}

func (c *cache) Set(k CacheKey, v []byte) {
	c.kv[k] = v
}

func (c *cache) Get(k CacheKey) (v []byte, found bool) {
	v, found = c.kv[k]

	if !found {
		c.misses++
	} else {
		c.hits++
	}

	return
}

func cacheFileName(start, end uint64) string {
	return fmt.Sprintf("cache-%d-%d.cache", start, end)
}

func cacheFileStartBlockPrefix(start uint64) string {
	return fmt.Sprintf("cache-%d", start)
}

func cacheFilePrefix() string {
	return fmt.Sprintf("cache-")
}

func mustParseCacheFileName(filename string) (start uint64, end uint64) {
	res := cacheFileRegex.FindAllStringSubmatch(filename, 1)
	if len(res) == 0 || len(res[0]) != 3 {
		panic(fmt.Sprintf("invalid cache file name %s", filename))
	}

	var err error
	start, err = strconv.ParseUint(res[0][1], 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid start block in Cache file name %s", filename))
	}

	end, err = strconv.ParseUint(res[0][2], 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid end block in Cache file name %s", filename))
	}

	return
}
