package kv

// KVStore is the embeded KV-store.
type KVStore interface {
	Get(key string) (value string, existed bool)
	Put(key string, value string) (oldValue string, existed bool)
	Incr(key string, delta int) (oldValue string, existed bool, err error)
	Del(key string) (oldValue string, existed bool)
}

// KVStoreService is a service wrapping a KVStore.
type KVStoreService interface {
	KVStore
	RPCPut(args *PutArgs, reply *Reply) error
	RPCGet(args *GetArgs, reply *Reply) error
	RPCIncr(args *IncrArgs, reply *Reply) (err error)
	RPCDel(args *DelArgs, reply *Reply) (err error)
}

type GetArgs struct {
	Key string
}

type DelArgs GetArgs

type PutArgs struct {
	Key   string
	Value string
}

type IncrArgs struct {
	Key   string
	Delta int
}

type Reply struct {
	Flag  bool
	Value string
}
