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

func (pool *ResourcePool) Put(r Resource) {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	pool.entities.PushBack(r)
	pool.cond.Broadcast()
}

func (pool *ResourcePool) Get() Resource {
	pool.mu.Lock()

	for pool.entities.Front() == nil && atomic.LoadInt64(&pool.activeSize) >= pool.maxSize {
		pool.cond.Wait()
	}
	ele := pool.entities.Front()
	if ele == nil {
		atomic.AddInt64(&pool.activeSize, 1)
		pool.mu.Unlock()
		entity := pool.New()
		if entity == nil {
			atomic.AddInt64(&pool.activeSize, -1)
			return nil
		}
		return entity
	}
	c := ele.Value.(Resource)
	pool.entities.Remove(ele)
	pool.mu.Unlock()
	return c
}

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
