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
	manager *CacheManager
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

type CacheManager struct {
	store dstore.Store

	currentCache      *Cache
	currentFilename   string
	currentStartBlock uint64
	currentEndBlock   uint64

	totalHits   int
	totalMisses int

	mu sync.Mutex
}

func NewCacheManager(ctx context.Context, store dstore.Store, startBlock int64) *CacheManager {
	cf := &CacheManager{
		store: store,
	}

	if startBlock > 0 {
		cf.currentStartBlock = uint64(startBlock)
	}

	perfTracker := &CachePerformanceTracker{manager: cf}
	perfTracker.startTracking(ctx)

	return cf
}

func (cm *CacheManager) initialize(ctx context.Context, startBlock, endBlock uint64) *Cache {
	var filename string
	var found bool

	prefixSearchBlock := startBlock
	if cm.currentStartBlock > 0 {
		prefixSearchBlock = cm.currentStartBlock
	}

	///search by prefix first...
	cm.store.Walk(ctx, cacheFileStartBlockPrefix(prefixSearchBlock), ".tmp", func(fn string) (err error) {
		filename = fn
		found = true
		return nil
	})

	///if not found walk the store, look for closest start block
	if !found {
		closestDiff := uint64(math.MaxUint64)
		cm.store.Walk(ctx, cacheFilePrefix(), ".tmp", func(fn string) (err error) {
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
		cm.currentFilename = filename
	} else {
		fileStartBlock := startBlock
		if cm.currentStartBlock > 0 {
			fileStartBlock = cm.currentStartBlock
		}
		filename = cacheFileName(fileStartBlock, endBlock)
		cm.currentFilename = filename
	}

	cm.currentEndBlock = endBlock
	cm.currentCache = newCache()
	cm.Load(ctx)

	return cm.currentCache
}

func (cm *CacheManager) Get(ctx context.Context, startBlock, endBlock uint64) *Cache {
	var initialize bool

	if cm.currentCache == nil {
		initialize = true
	} else if cm.currentEndBlock <= math.MaxUint64 && startBlock >= cm.currentEndBlock {
		saveStartBlock := startBlock
		if cm.currentStartBlock > 0 {
			saveStartBlock = cm.currentStartBlock
		}
		cm.Save(ctx, saveStartBlock, cm.currentEndBlock)
		initialize = true
	}

	if initialize {
		cm.initialize(ctx, startBlock, endBlock)
	}

	return cm.currentCache
}

func (cm *CacheManager) Save(ctx context.Context, startBlock uint64, endBlock uint64) {
	if cm.currentCache == nil {
		return
	}

	saveStartBlock := startBlock
	if cm.currentStartBlock > startBlock {
		saveStartBlock = cm.currentStartBlock
	}
	cm.currentCache.save(ctx, cm.store, cacheFileName(saveStartBlock, endBlock))

	cm.totalHits += cm.currentCache.hits
	cm.totalMisses += cm.currentCache.misses

	cm.currentStartBlock = endBlock
	cm.currentCache = nil
}

func (cm *CacheManager) Load(ctx context.Context) {
	cm.currentCache.load(ctx, cm.store, cm.currentFilename)
}

type Cache struct {
	kv map[CacheKey][]byte

	hits   int
	misses int
}

func newCache() *Cache {
	return &Cache{
		kv: make(map[CacheKey][]byte),
	}
}

func (c *Cache) load(ctx context.Context, store dstore.Store, filename string) {
	if store == nil {
		zlog.Info("skipping rpccache load: no read store is defined")
		return
	}

	obj, err := store.OpenObject(ctx, filename)
	if err != nil {
		zlog.Info("rpc Cache not found", zap.String("filename", filename), zap.String("read_store_url", store.BaseURL().Redacted()), zap.Error(err))
		return
	}

	b, err := ioutil.ReadAll(obj)
	if err != nil {
		zlog.Info("cannot read all rpc Cache bytes", zap.String("filename", filename), zap.String("read_store_url", store.BaseURL().Redacted()), zap.Error(err))
		return
	}

	kv := make(map[CacheKey][]byte)
	err = json.Unmarshal(b, &kv)
	if err != nil {
		zlog.Info("cannot unmarshal rpc Cache", zap.String("filename", filename), zap.String("read_store_url", store.BaseURL().Redacted()), zap.Error(err))
		return
	}
	c.kv = kv
}

func (c *Cache) save(ctx context.Context, store dstore.Store, filename string) {
	if store == nil {
		zlog.Info("skipping rpccache save: no store is defined")
		return
	}

	b, err := json.Marshal(c.kv)
	if err != nil {
		zlog.Info("cannot marshal rpc Cache to bytes", zap.Error(err))
		return
	}
	ioreader := bytes.NewReader(b)

	err = store.WriteObject(ctx, filename, ioreader)
	if err != nil {
		zlog.Info("cannot write rpc Cache to store", zap.String("filename", filename), zap.String("write_store_url", store.BaseURL().Redacted()), zap.Error(err))
	}

	return
}

func (_ *Cache) Key(prefix string, items ...interface{}) CacheKey {
	key := prefix
	for _, it := range items {
		key = fmt.Sprintf("%s:%v", key, it)
	}
	return CacheKey(key)
}

func (c *Cache) Set(k CacheKey, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		return // skipping is OK for a Cache
	}
	c.kv[k] = b
}

func (c *Cache) Get(k CacheKey, out interface{}) (found bool) {
	v, found := c.kv[k]
	if !found {
		c.misses++
		return false
	}
	c.hits++

	if err := json.Unmarshal(v, &out); err != nil {
		return false
	}
	return true
}

func (c *Cache) GetRaw(k CacheKey) (v []byte, found bool) {
	v, found = c.kv[k]

	if !found {
		c.misses++
	} else {
		c.hits++
	}

	return
}

func (c *Cache) Stats() (hits int, misses int) {
	return c.hits, c.misses
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
