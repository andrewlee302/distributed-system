package commit

import (
	"fmt"
	"net"
	"net/rpc"
	"syscall"
)

type PreparedArgs struct {
	TxnID      string
	TxnPartIdx int
	Flag       int
}

type PreparedReply struct{}

type AbortedArgs struct {
	TxnID      string
	TxnPartIdx int
	Flag       int
}

type AbortedReply struct{}

type PrepareArgs struct {
	TxnPartID string
}

type PrepareReply struct {
	State int32
}

type AbortArgs struct {
	TxnPartID string
}

type AbortReply struct{}

type CommitArgs struct {
	TxnPartID string
}

type CommitReply struct {
	TxnPartID string
}

//
// call() sends an RPC to the rpcname handler on server srv
// with arguments args, waits for the reply, and leaves the
// reply in reply. the reply argument should be a pointer
// to a reply structure.
//
// the return value is true if the server responded, and false
// if call() was not able to contact the server. in particular,
// the replys contents are only valid if call() returned true.
//
// you should assume that call() will time out and return an
// error after a while if it does not get a reply from the server.
//
// please use call() to send all RPCs, in client.go and server.go.
// please do not change this function.
//
func call(srv string, name string, args interface{}, reply interface{}) bool {
	c, err := rpc.Dial("unix", srv)
	if err != nil {
		err1 := err.(*net.OpError)
		if err1.Err != syscall.ENOENT && err1.Err != syscall.ECONNREFUSED {
			fmt.Printf("paxos Dial() failed: %v\n", err1)
		}
		return false
	}
	defer c.Close()

	err = c.Call(name, args, reply)
	if err == nil {
		return true
	}
	return false
}
