package lru

import (
	"errors"
	"iter"
	"sync"
)

type node[K comparable, V any] struct {
	next int
	last int
	key  K
	val  V
}

// A Cache is a generic, concurrency-safe least-recently used (LRU) cache. A Cache should not be copied.
type Cache[K comparable, V any] struct {
	m     sync.Mutex
	len   int
	head  int
	tail  int
	cap   uint64
	evict func(K, V) error
	data  []node[K, V]
	keys  map[K]int
}

// New creates a new Cache with a capacity of cap items. If evict is non-nil, it is called each time a key-value
// pair is evicted.
func New[K comparable, V any](cap uint64, evict func(K, V) error) *Cache[K, V] {
	return &Cache[K, V]{
		cap:   cap,
		keys:  make(map[K]int, cap),
		data:  make([]node[K, V], cap),
		evict: evict,
	}
}

// promote moves the node at index i to the front of the queue.
func (c *Cache[K, V]) promote(i int) {
	ptr := &c.data[i]

	if i == c.head {
		return
	}

	if i == c.tail {
		c.tail = ptr.last
	} else {
		c.data[ptr.last].next = ptr.next
		c.data[ptr.next].last = ptr.last
	}

	ptr.next = c.head
	c.data[c.head].last = i
	c.head = i
}

// Get returns the cached value associated with key and a bool, which is true if the key was found
// and false otherwise.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.m.Lock()
	defer c.m.Unlock()
	i, ok := c.keys[key]
	if ok {
		val := c.data[i].val
		c.promote(i)
		return val, true
	}
	// cache miss, nothing to do
	return *new(V), false
}

// Put adds a key-value pair to the Cache. If the Cache is full and the key is not already cached, it
// evicts the least-recently used entry. If an eviction occurs and the Cache's evict func is non-nil,
// Put returns any error returned by evict. Otherwise, the returned error will be nil.
func (c *Cache[K, V]) Put(key K, val V) error {
	c.m.Lock()
	defer c.m.Unlock()
	var err error

	if c.cap == 0 {
		return err
	}

	// if the key is cached, just update the val and move to front
	i, ok := c.keys[key]
	if ok {
		c.data[i].val = val
		c.promote(i)
		return err
	}

	// if there's space, no need to evict
	if uint64(c.len) < c.cap {
		// take the highest unused
		c.data[c.len] = node[K, V]{
			next: c.head,
			key:  key,
			val:  val,
		}
		c.data[c.head].last = c.len
		// no need to update the tail; the initial tail will be at index 0
		c.head = c.len
		c.keys[key] = c.len
		c.len++
		return err
	}

	victim := &c.data[c.tail]
	if c.evict != nil {
		err = c.evict(victim.key, victim.val)
	}

	// take from the tail
	delete(c.keys, victim.key)
	c.keys[key] = c.tail

	victim.key = key
	victim.val = val

	c.promote(c.tail)
	return err
}

// Clear evicts all entries from the Cache (calling the evict func if it exists) and resets the Cache.
// A cleared Cache is safe for re-use.
func (c *Cache[K, V]) Clear() error {
	c.m.Lock()
	defer c.m.Unlock()
	var err error

	if c.evict != nil {
		var n node[K, V]
		for _, n = range c.data[:c.len] {
			err = errors.Join(err, c.evict(n.key, n.val))
		}
	}
	clear(c.data[:c.len])
	clear(c.keys)
	c.len = 0
	return err
}

// All returns an iter.Seq2 that iterates over all Cache entries.
func (c *Cache[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		c.m.Lock()
		defer c.m.Unlock()
		var n node[K, V]
		for _, n = range c.data[:c.len] {
			if !yield(n.key, n.val) {
				return
			}
		}
	}
}

// Keys returns an iter.Seq that iterates over all cached keys.
func (c *Cache[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		c.m.Lock()
		defer c.m.Unlock()
		var n node[K, V]
		for _, n = range c.data[:c.len] {
			if !yield(n.key) {
				return
			}
		}
	}
}

// Values returns an iter.Seq that iterates over all cached values.
func (c *Cache[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		c.m.Lock()
		defer c.m.Unlock()
		var n node[K, V]
		for _, n = range c.data[:c.len] {
			if !yield(n.val) {
				return
			}
		}
	}
}
