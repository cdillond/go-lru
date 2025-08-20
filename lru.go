package lru

import (
	"errors"
	"iter"
	"sync"
)

// A Cache is a generic least-recently used (LRU) cache.
type Cache[K comparable, V any] struct {
	m     sync.Mutex
	keys  []K
	vals  []V
	seen  []uint64
	len   uint64
	count uint64
	evict func(K, V) error
}

// New creates a new Cache of size size. If evict is non-nil, it is called each time a key-value
// pair is evicted. Since the lookup is O(n), cache sizes should remain small.
func New[K comparable, V any](size uint, evict func(K, V) error) *Cache[K, V] {
	return &Cache[K, V]{
		keys:  make([]K, size),
		vals:  make([]V, size),
		seen:  make([]uint64, size),
		evict: evict,
	}
}

// Get returns the cached value associated with key and a bool, which is true if the key was found
// and false otherwise. The lookup is O(n).
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.m.Lock()
	defer c.m.Unlock()
	c.count++
	l := c.len
	var i uint64
	for ; i < l; i++ {
		if c.keys[i] == key {
			c.seen[i] = c.count
			return c.vals[i], true
		}
	}
	return *new(V), false
}

// Put adds a key-value pair to the Cache. If the Cache is full and the key is not already cached, it
// evicts the least-recently used entry. If an eviction occurs and the Cache's evict func is non-nil,
// Put returns any error returned by evict. Otherwise, the returned error will be nil.
func (c *Cache[K, V]) Put(key K, val V) error {
	c.m.Lock()
	defer c.m.Unlock()
	var err error
	c.count++

	if len(c.keys) == 0 {
		return err
	}

	if c.len < uint64(len(c.keys)) {
		c.keys[c.len] = key
		c.vals[c.len] = val
		c.seen[c.len] = c.count
		c.len++
		return err
	}

	var i, n uint64
	min := uint64((1 << 64) - 1)
	l := c.len
	x := -1

	for ; i < l; i++ {
		if c.seen[i] < min {
			n = i
			min = c.seen[i]
		}
		if key == c.keys[i] {
			x = int(i)
		}
	}

	// no eviction necessary, just overwrite the current value
	if x != -1 {
		c.vals[x] = val
		c.seen[x] = c.count
		return err
	}

	// sadly, someone must go
	if c.evict != nil {
		err = c.evict(c.keys[n], c.vals[n])
	}

	c.keys[n] = key
	c.vals[n] = val
	c.seen[n] = c.count

	return err
}

// Grow increases the Cache's capacity by more entries.
func (c *Cache[K, V]) Grow(more uint64) {
	c.m.Lock()
	defer c.m.Unlock()

	c.keys = append(c.keys, make([]K, more)...)
	c.vals = append(c.vals, make([]V, more)...)
	c.seen = append(c.seen, make([]uint64, more)...)
}

// Clear evicts all entries from the Cache (calling the evict func if it exists) and resets the Cache.
// A cleared Cache is safe for re-use.
func (c *Cache[K, V]) Clear() error {
	c.m.Lock()
	defer c.m.Unlock()
	var err error

	if c.evict != nil {
		l := c.len
		var i uint64
		for ; i < l; i++ {
			err = errors.Join(err, c.evict(c.keys[i], c.vals[i]))
		}
	}

	clear(c.keys)
	clear(c.vals)
	clear(c.seen)
	c.len = 0
	c.count = 0

	return err
}

// All returns an iter.Seq2 that iterates over all Cache entries.
func (c *Cache[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		c.m.Lock()
		defer c.m.Unlock()
		i, l := uint64(0), c.len
		for ; i < l; i++ {
			if !yield(c.keys[i], c.vals[i]) {
				return
			}
		}
	}
}
