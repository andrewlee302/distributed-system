/*
Package tinykv provides concurrent primitive operations for a hashmap-based
storage. The primitives are Put/Get/Incr/Del. They are all thread-safe and
mutually exclusive with each other.

RPC interfaces for the basic operations are intended for the extended services,
which could wrap the KVStore and then enhance it.
*/
package tinykv

import (
	"distributed-system/kv"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// KVStore is the hashmap-based storage.
type KVStore struct {
	data map[string]string

	// RwLock is the public Read-Write Mutex, which can
	// be used in the extended KV-Store.
	RwLock sync.RWMutex

	dead       int32 // for testing
	unreliable int32 // for testing

	// debug
	costNs int64
}

// NewKVStore inits a KVStore.
func NewKVStore() *KVStore {
	ks := &KVStore{data: make(map[string]string)}
	go func() {
		for _ = range time.Tick(time.Second * 5) {
			ns := atomic.LoadInt64(&ks.costNs)
			fmt.Printf("KVStore cost %v ms\n", ns/time.Millisecond.Nanoseconds())
		}
	}()
	return ks
}

// KVStoreService provides the RPC service for KVStore.
type KVStoreService struct {
	*KVStore
	l       net.Listener
	network string
	addr    string
}

// NewKVStoreService inits a KV-Store service.
func NewKVStoreService(network, addr string) *KVStoreService {
	log.Printf("Start kvstore service on %s\n", addr)
	service := &KVStoreService{KVStore: NewKVStore(),
		network: network, addr: addr,
	}
	return service
}

// Serve starts the KVStore service to serve the RPC requests.
func (ks *KVStoreService) Serve() {
	rpcs := rpc.NewServer()
	rpcs.Register(ks)
	l, e := net.Listen(ks.network, ks.addr)
	if e != nil {
		log.Fatal("listen error: ", e)
	}
	ks.l = l
	go func() {
		for !ks.IsDead() {
			if conn, err := l.Accept(); err == nil {
				if !ks.IsDead() {
					// concurrent processing
					go rpcs.ServeConn(conn)
				} else if err == nil {
					conn.Close()
				}
			} else {
				if !ks.IsDead() {
					log.Fatalln(err.Error())
				}
			}
		}
	}()
}

// IsDead returns whether the KVStore service is dead or not. If dead, the
// RPC requests can be served until the service recovers.
func (ks *KVStoreService) IsDead() bool {
	return atomic.LoadInt32(&ks.dead) != 0
}

// Kill closes the kvstore service, which makes the service dead.
func (ks *KVStoreService) Kill() {
	log.Println("Kill the kvstore")
	atomic.StoreInt32(&ks.dead, 1)
	// ks.data = nil
	if err := ks.l.Close(); err != nil {
		log.Fatal("Kvsotre rPC server close error:", err)
	}
}

// Put the key-value pair into the stroage and return the old value and
// existed flag. If the key exists before, then the flag will be true and
// the old value is not nil. Otherwise, the old value is nil and the existed
// flag is false.
//
// It's thread-safe and mutually exclusive with other operations.
func (ks *KVStore) Put(key, value string) (oldValue string, existed bool) {
	now := time.Now()
	defer func() {
		atomic.AddInt64(&ks.costNs, time.Since(now).Nanoseconds())
	}()

	ks.RwLock.Lock()
	defer ks.RwLock.Unlock()
	oldValue, existed = ks.data[key]
	ks.data[key] = value
	return
}

// Get the corresponding value for a specific key. If the key exists, the
// value is not nil and the existed flag is true. Otherwise, the value is nil
// and the flag is false.
//
// It's thread-safe and mutually exclusive with other operations.
func (ks *KVStore) Get(key string) (value string, existed bool) {
	now := time.Now()
	defer func() {
		atomic.AddInt64(&ks.costNs, time.Since(now).Nanoseconds())
	}()

	ks.RwLock.RLock()
	defer ks.RwLock.RUnlock()

	value, existed = ks.data[key]
	return
}

// Incr the corresponding value by delta for a specific key. If the key exists
// and its value is numeric, then return the new value, and the existed flag
// is true without any error. If the corresponding value is not numberic, the
// NumError is returned. If the key doesn't exist, the delta will be set as
// the value of the key, and the new value is delta, existed flag is false
// without any error.
//
// It's thread-safe and mutually exclusive with other operations.
func (ks *KVStore) Incr(key string, delta int) (newVal string, existed bool, err error) {
	now := time.Now()
	defer func() {
		atomic.AddInt64(&ks.costNs, time.Since(now).Nanoseconds())
	}()

	ks.RwLock.Lock()
	defer ks.RwLock.Unlock()
	var oldVal string
	if oldVal, existed = ks.data[key]; existed {
		var iOldVal int
		if iOldVal, err = strconv.Atoi(oldVal); err == nil {
			newVal = strconv.Itoa(iOldVal + delta)
			ks.data[key] = newVal
			return
		}
		return
	}
	newVal = strconv.Itoa(delta)
	ks.data[key] = newVal
	return
}

// Del deletes the specific key-value pair. If it exists before, the existed
// flag is true. Otherwise, it's false.
//
// It's thread-safe and mutually exclusive with other operations.
func (ks *KVStore) Del(key string) (oldValue string, existed bool) {
	now := time.Now()
	defer func() {
		atomic.AddInt64(&ks.costNs, time.Since(now).Nanoseconds())
	}()

	ks.RwLock.Lock()
	defer ks.RwLock.Unlock()
	if oldValue, existed = ks.data[key]; existed {
		delete(ks.data, key)
	}
	return
}

// RPCPut is the RPC interface for Put operation.
func (ks *KVStore) RPCPut(args *kv.PutArgs, reply *kv.Reply) error {
	reply.Value, reply.Flag = ks.Put(args.Key, args.Value)
	return nil
}

// RPCGet is the RPC interface for Get operation.
func (ks *KVStore) RPCGet(args *kv.GetArgs, reply *kv.Reply) error {
	reply.Value, reply.Flag = ks.Get(args.Key)
	return nil
}

// RPCIncr is the RPC interface for Incr operation.
func (ks *KVStore) RPCIncr(args *kv.IncrArgs, reply *kv.Reply) (err error) {
	reply.Value, reply.Flag, err = ks.Incr(args.Key, args.Delta)
	return err
}

// RPCDel is the RPC interface for Del operation.
func (ks *KVStore) RPCDel(args *kv.DelArgs, reply *kv.Reply) (err error) {
	reply.Value, reply.Flag = ks.Del(args.Key)
	return nil
}
