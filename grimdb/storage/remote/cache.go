//go:build enterprise

// Package remote implements S3-compatible remote storage for the Enterprise tier.
package remote

import (
	"container/list"
	"sync"
)

const defaultCacheBytes = 100 * 1024 * 1024 // 100 MB

// blockCache is a thread-safe LRU cache for encrypted block bytes.
// Entries are evicted when the total size exceeds the configured limit.
type blockCache struct {
	mu       sync.Mutex
	maxBytes int
	curBytes int
	items    map[string]*list.Element
	lru      *list.List
}

type cacheEntry struct {
	id   string
	data []byte
}

func newBlockCache(maxBytes int) *blockCache {
	if maxBytes <= 0 {
		maxBytes = defaultCacheBytes
	}
	return &blockCache{
		maxBytes: maxBytes,
		items:    make(map[string]*list.Element),
		lru:      list.New(),
	}
}

func (c *blockCache) get(id string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[id]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(el)
	return el.Value.(*cacheEntry).data, true
}

func (c *blockCache) put(id string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[id]; ok {
		c.lru.MoveToFront(el)
		old := el.Value.(*cacheEntry)
		c.curBytes -= len(old.data)
		old.data = data
		c.curBytes += len(data)
		return
	}

	el := c.lru.PushFront(&cacheEntry{id: id, data: data})
	c.items[id] = el
	c.curBytes += len(data)

	for c.curBytes > c.maxBytes && c.lru.Len() > 1 {
		back := c.lru.Back()
		if back == nil {
			break
		}
		e := back.Value.(*cacheEntry)
		delete(c.items, e.id)
		c.curBytes -= len(e.data)
		c.lru.Remove(back)
	}
}

func (c *blockCache) evict(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[id]
	if !ok {
		return
	}
	e := el.Value.(*cacheEntry)
	c.curBytes -= len(e.data)
	delete(c.items, id)
	c.lru.Remove(el)
}
