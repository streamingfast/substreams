package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
)

type Cache struct {
	fileName   string
	readStore  dstore.Store
	writeStore dstore.Store
	kv         map[CacheKey][]byte
}

type CacheKey string

func NewCache(readStore, writeStore dstore.Store, startBlockNum, endBlockNum uint64) *Cache {
	return &Cache{
		kv:         make(map[CacheKey][]byte),
		fileName:   cacheFileName(startBlockNum, endBlockNum),
		readStore:  readStore,
		writeStore: writeStore,
	}
}

func cacheFileName(start, end uint64) string {
	return fmt.Sprintf("%d-%d", start, end)
}

func (c *Cache) Load(ctx context.Context) {
	if c.readStore == nil {
		zlog.Info("skipping rpccache load: no store is defined")
		return
	}
	obj, err := c.readStore.OpenObject(ctx, c.fileName)
	if err != nil {
		zlog.Info("rpc cache not found", zap.String("filename", c.fileName), zap.String("read_store_url", c.readStore.BaseURL().Redacted()), zap.Error(err))
		return
	}

	b, err := ioutil.ReadAll(obj)
	if err != nil {
		zlog.Info("cannot read all rpc cache bytes", zap.String("filename", c.fileName), zap.String("read_store_url", c.readStore.BaseURL().Redacted()), zap.Error(err))
		return
	}

	kv := make(map[CacheKey][]byte)
	err = json.Unmarshal(b, &kv)
	if err != nil {
		zlog.Info("cannot unmarshal rpc cache", zap.String("filename", c.fileName), zap.String("read_store_url", c.readStore.BaseURL().Redacted()), zap.Error(err))
		return
	}
	c.kv = kv
}

func (c *Cache) Save(ctx context.Context) {
	if c.writeStore == nil {
		zlog.Info("skipping rpccache save: no store is defined")
		return
	}
	b, err := json.Marshal(c.kv)
	if err != nil {
		zlog.Info("cannot marshal rpc cache to bytes", zap.Error(err))
		return
	}
	ioreader := bytes.NewReader(b)

	err = c.writeStore.WriteObject(ctx, c.fileName, ioreader)
	if err != nil {
		zlog.Info("cannot write rpc cache to store", zap.String("filename", c.fileName), zap.String("write_store_url", c.writeStore.BaseURL().Redacted()), zap.Error(err))
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
		return // skipping is OK for a cache
	}
	c.kv[k] = b
}
func (c *Cache) GetRaw(k CacheKey) (v []byte, found bool) {
	v, found = c.kv[k]
	return
}

func (c *Cache) Get(k CacheKey, out interface{}) (found bool) {
	v, found := c.kv[k]
	if !found {
		return false
	}

	if err := json.Unmarshal(v, &out); err != nil {
		return false
	}
	return true
}
