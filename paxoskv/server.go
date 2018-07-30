/*
Package paxoskv provides fault-tolerant KV-store components based on paxos,
including service and client.

Every KVPaxos wrap a paxos peer to do the consensus thing. All the KVPaxos
cooperate as whole fault-tolerant KV-store system. The system will get
an agreement on the Ops with the consecutive numbers, i.e. the paxos instance.
The consecutive Ops construct a State Machine, representing the actions on
the key-value pairs in the KV-store.

The ops are three kinds: Get/Put/Append. We can records all the ops in the
State Machine, so we can fetch the acual value of the specific key. Using
the State Machine to log the actions on the data is a popular way to write
and read the data. Because we needn't modify the data directly, we could
undo the ops to recover the data, or replay ops to make a replication.

Refer to the chapter 3 "Implementing a State Machine" in "Paxos Made Simple"
about how to use a state machine to represent the states in a system. Likewise,
the key-value pairs are values of the KV-store.
*/
package paxoskv

import (
	"distributed-system/paxos"
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Op is a Operation in the State Machine, representing an action on the
// key-value pairs. In the KV-store, there are three types of operations:
// Get/Put/Append. Every Op is the object, which will be gotten an agreement
// on the paxos peers.
type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	OpName string
	Key    string
	Value  string
	ID     int64
}

// KVPaxos is a peer in the KVPaxos group, which cooperate as a whole
// fault-tolerant KV-store. Every KVPaxos wraps a paxos peer to do the
// consensus thing, i.e. agreeing every Op in the State Machine.
type KVPaxos struct {
	mu         sync.Mutex
	l          net.Listener
	me         int
	dead       int32 // for testing
	unreliable int32 // for testing
	px         *paxos.Paxos

	// Your definitions here.
	content map[string]string
	seq     int // seq for next req
	history map[int64]bool
}

// apply reflects the operation to the key-value pairs in the KV-store.
func (kv *KVPaxos) apply(op *Op) {
	switch op.OpName {
	case Put:
		kv.content[op.Key] = op.Value
	case Append:
		kv.content[op.Key] += op.Value
	default:
		// nothing
	}
	// Inject the GET value into the history,
	// the Write op can be recorded without value for dedup.
	kv.history[op.ID] = true
	return
}

// TryDecide try to get an aggrement on the operation in one of the paxos
// instance. The seq will be increased until deciding the op in the State
// Machine. Then the chosen value by the paxos peers could be applied to the
// KV-store. Note that not every chosen value must be applied to the
// key-values immediately. Please consider that Put and Append operations
// don't return the actual value of the specific key.
func (kv *KVPaxos) TryDecide(op Op) (string, string) {
	// TODO concurrency optimization
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if _, ok := kv.history[op.ID]; ok {
		if op.OpName == Get {
			return OK, kv.content[op.Key]
		}
		return OK, ""
	}
	chosen := false
	for !chosen {
		timeout := 0 * time.Millisecond
		sleepInterval := 10 * time.Millisecond
		kv.px.Start(kv.seq, op)
	INNER:
		for {
			fate, v := kv.px.Status(kv.seq)
			switch fate {
			case paxos.Chosen:
				{
					_op := v.(Op)
					kv.px.Done(kv.seq)
					kv.apply(&_op)
					kv.seq++
					if _op.ID == op.ID {
						if _op.OpName == Get {
							if v, ok := kv.content[op.Key]; ok {
								return OK, v
							}
							return ErrNoKey, ""
						}
						// for put/append operation
						chosen = true
					}
					break INNER
				}
			case paxos.Pending:
				{
					if timeout > 10*time.Second {
						return ErrPending, ""
					}
					time.Sleep(sleepInterval)
					timeout += sleepInterval
					sleepInterval *= 2
				}
			default:
				// Forgotten, do nothing for impossibility
				return ErrForgotten, ""
			}
		}
	}
	return OK, ""
}

// Get is RPC routine invoked by paxoskv.Client to get the corresponding
// value of the specific key.
func (kv *KVPaxos) Get(args *GetArgs, reply *GetReply) error {
	// TODO Your code here
	op := Op{OpName: Get, Key: args.Key, Value: "", ID: args.ID}
	reply.Err, reply.Value = kv.TryDecide(op)
	return nil
}

// PutAppend is RPC routine invoked by paxoskv.Client to put or append a value
// for the specific key.
func (kv *KVPaxos) PutAppend(args *PutAppendArgs, reply *PutAppendReply) error {
	// TODO Your code here
	op := Op{OpName: args.Op, Key: args.Key, Value: args.Value, ID: args.ID}
	reply.Err, _ = kv.TryDecide(op)
	return nil
}

// tell the server to shut itself down.
// please do not change these two functions.
func (kv *KVPaxos) kill() {
	log.Printf("Kill(%d): die\n", kv.me)
	atomic.StoreInt32(&kv.dead, 1)
	kv.l.Close()
	kv.px.Kill()
}

// call this to find out if the server is dead.
func (kv *KVPaxos) isdead() bool {
	return atomic.LoadInt32(&kv.dead) != 0
}

// please do not change these two functions.
func (kv *KVPaxos) setunreliable(what bool) {
	if what {
		atomic.StoreInt32(&kv.unreliable, 1)
	} else {
		atomic.StoreInt32(&kv.unreliable, 0)
	}
}

func (kv *KVPaxos) isunreliable() bool {
	return atomic.LoadInt32(&kv.unreliable) != 0
}

// StartServer init a KV-store server based on the paxos. servers[] contains
// the addresses of the set of KV-stores servers that will coopreate via paxos
// to form a whole fault-tolerant key-value service. me is the index of the
// current server in servers[].
func StartServer(servers []string, me int) *KVPaxos {
	// call gob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	gob.Register(Op{})

	kv := new(KVPaxos)
	kv.me = me

	// Your initialization code here.
	kv.content = make(map[string]string)
	kv.history = make(map[int64]bool)
	kv.seq = 0

	rpcs := rpc.NewServer()
	rpcs.Register(kv)

	kv.px = paxos.Make(servers, me, rpcs)

	os.Remove(servers[me])
	l, e := net.Listen("unix", servers[me])
	if e != nil {
		log.Fatal("listen error: ", e)
	}
	kv.l = l

	// please do not change any of the following code,
	// or do anything to subvert it.

	go func() {
		for kv.isdead() == false {
			conn, err := kv.l.Accept()
			if err == nil && kv.isdead() == false {
				if kv.isunreliable() && (rand.Int63()%1000) < 100 {
					// discard the request.
					conn.Close()
				} else if kv.isunreliable() && (rand.Int63()%1000) < 200 {
					// process the request but force discard of reply.
					c1 := conn.(*net.UnixConn)
					f, _ := c1.File()
					err := syscall.Shutdown(int(f.Fd()), syscall.SHUT_WR)
					if err != nil {
						fmt.Printf("shutdown: %v\n", err)
					}
					go rpcs.ServeConn(conn)
				} else {
					go rpcs.ServeConn(conn)
				}
			} else if err == nil {
				conn.Close()
			}
			if err != nil && kv.isdead() == false {
				fmt.Printf("KVPaxos(%v) accept: %v\n", me, err.Error())
				kv.kill()
			}
		}
	}()

	return kv
}
