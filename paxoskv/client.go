package paxoskv

import (
	"crypto/rand"
	"distributed-system/util"
	"math/big"
)

// Client is for the Paxos-based KV-store service.
type Client struct {
	servers []string
	// You may add code here.
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

// MakeClient inits a new Client with the servers addresses.
func MakeClient(servers []string) *Client {
	ck := new(Client)
	ck.servers = servers

	// You may add code here.
	return ck
}

// Get gets the corresponding value for the specific key. Return "" if the
// key doesn't exist. It tries forever in the face of all other errors (i.e.
// except for OK and ErrNoKey).
func (ck *Client) Get(key string) string {
	// TODO Your code here
	args := &GetArgs{Key: key, ID: nrand()}
	var reply GetReply
	for i := 0; ; {
		if ok := util.Call("unix", ck.servers[i], "KVPaxos.Get", args, &reply); ok && (reply.Err == OK || reply.Err == ErrNoKey) {
			return reply.Value
		}
		i++
		i %= len(ck.servers)
	}
}

// PutAppend puts or append a value onto the specific key. Op arg indicates
// the type of operations, "Put" or "Append".
func (ck *Client) PutAppend(key string, value string, op string) {
	// TODO Your code here
	args := &PutAppendArgs{Key: key, Value: value, Op: op, ID: nrand()}
	var reply PutAppendReply
	for i := 0; ; {
		if ok := util.Call("unix", ck.servers[i], "KVPaxos.PutAppend", args, &reply); ok && reply.Err == OK {
			return
		}
		i++
		i %= len(ck.servers)
	}
}

// Put puts the key-value pair into the database.
func (ck *Client) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}

// Append a value to the original value of the specific key.
func (ck *Client) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
