/*
Package util provides basic utilities.
*/
package util

import (
	"container/list"
	"sync"
	"sync/atomic"
)

type Resource interface {
	Close() error
}

type ResourcePool struct {
	New        func() Resource
	entities   *list.List
	maxSize    int64
	activeSize int64
	mu         sync.Mutex
	cond       sync.Cond
}

func NewResourcePool(new func() Resource, maxSize int) *ResourcePool {
	pool := ResourcePool{New: new, entities: list.New(),
		maxSize: int64(maxSize), activeSize: 0}
	pool.cond = sync.Cond{L: &pool.mu}
	return &pool
}

// Put back the available resource into the pool for the future use.
func (pool *ResourcePool) Put(r Resource) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.entities.PushBack(r)
	pool.cond.Broadcast()
}

// Get a resource from the pool.
func (pool *ResourcePool) Get() Resource {
	pool.mu.Lock()

	for pool.entities.Front() == nil && atomic.LoadInt64(&pool.activeSize) >= pool.maxSize {
		pool.cond.Wait()
	}
	ele := pool.entities.Front()
	if ele == nil {
		// *** The following code should be here instead of the end of the block.
		atomic.AddInt64(&pool.activeSize, 1)
		pool.mu.Unlock()

		entity := pool.New()
		if entity == nil {
			atomic.AddInt64(&pool.activeSize, -1)
			return nil
		}
		return entity
	}
	// Reuse
	c := ele.Value.(Resource)
	pool.entities.Remove(ele)
	pool.mu.Unlock()
	return c
}

// Close one resource in the pool and the active resource size decreases.
func (pool *ResourcePool) Clean(r Resource) {
	r.Close()
	atomic.AddInt64(&pool.activeSize, -1)
	pool.cond.Broadcast()
}

type ResourcePoolsArray struct {
	pools []*ResourcePool
}

func NewResourcePoolsArray(news []func() Resource, maxSizeForOne, poolNum int) *ResourcePoolsArray {
	pools := make([]*ResourcePool, poolNum)
	for i := 0; i < poolNum; i++ {
		pools[i] = NewResourcePool(news[i], maxSizeForOne)
	}
	return &ResourcePoolsArray{pools: pools}
}

func (array ResourcePoolsArray) Get(i int) Resource {
	return array.pools[i].Get()
}

func (array ResourcePoolsArray) Put(i int, r Resource) {
	array.pools[i].Put(r)
}

func (array ResourcePoolsArray) Clean(i int, r Resource) {
	array.pools[i].Clean(r)
}

type ResourcePoolsMap struct {
	mu            sync.RWMutex
	pools         map[string]*ResourcePool
	new           func(id string) func() Resource
	maxSizeForOne int
}

func NewResourcePoolsMap(new func(id string) func() Resource, maxSizeForOne int) *ResourcePoolsMap {
	pools := make(map[string]*ResourcePool)
	return &ResourcePoolsMap{pools: pools, new: new, maxSizeForOne: maxSizeForOne}
}

func (pm ResourcePoolsMap) Get(id string) Resource {
	pm.mu.Lock()
	pool, ok := pm.pools[id]
	if !ok {
		pool = NewResourcePool(pm.new(id), pm.maxSizeForOne)
		pm.pools[id] = pool
	}
	pm.mu.Unlock()
	return pool.Get()
}

func (pm ResourcePoolsMap) Put(id string, r Resource) {
	pm.mu.RLock()
	pool, ok := pm.pools[id]
	pm.mu.RUnlock()
	if !ok {
		panic("There is no pool for " + id)
	}
	pool.Put(r)
}

func (pm ResourcePoolsMap) Clean(id string, r Resource) {
	pm.mu.RLock()
	pool, ok := pm.pools[id]
	pm.mu.RUnlock()
	if !ok {
		panic("There is no pool for " + id)
	}
	pool.Clean(r)
}
