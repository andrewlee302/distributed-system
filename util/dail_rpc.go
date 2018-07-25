package util

import (
	"fmt"
	"net"
	"net/rpc"
	"syscall"
)

func DialServer(network, addr string) *rpc.Client {
	var err error
	rpcClient, err := rpc.Dial(network, addr)
	if err != nil {
		err1 := err.(*net.OpError)
		if err1.Err != syscall.ENOENT && err1.Err != syscall.ECONNREFUSED {
			fmt.Printf("Dial(%v) failed: %v\n", addr, err1)
		}
		return nil
	}
	return rpcClient
}

// debug
var RPCCallNs int64 = 0

func RPCPoolCall(pool *ResourcePool, name string, args interface{}, reply interface{}) bool {
	// now := time.Now()
	// defer func() {
	// 	atomic.AddInt64(&RPCCallNs, time.Since(now).Nanoseconds())
	// }()
	c := pool.Get().(*rpc.Client)
	if c != nil {
		ok := RPCCall(c, name, args, reply)
		if ok {
			pool.Put(c)
		} else {
			pool.Clean(c)
		}
		return ok
	}
	return false
}

func RPCPoolArrayCall(pa *ResourcePoolsArray, i int, name string, args interface{}, reply interface{}) bool {
	// now := time.Now()
	// defer func() {
	// 	atomic.AddInt64(&RPCCallNs, time.Since(now).Nanoseconds())
	// }()
	c := pa.Get(i).(*rpc.Client)
	if c != nil {
		ok := RPCCall(c, name, args, reply)
		if ok {
			pa.Put(i, c)
		} else {
			pa.Clean(i, c)
		}
		return ok
	}
	return false
}

// RPCCall is the RPC utility for reusing. Note that if successfully, the channel won't
// be closed.
func RPCCall(c *rpc.Client, name string, args interface{}, reply interface{}) bool {
	// now := time.Now()
	// defer func() {
	// 	atomic.AddInt64(&RPCCallNs, time.Since(now).Nanoseconds())
	// }()
	err := c.Call(name, args, reply)
	if err == nil {
		return true
	}
	c.Close()
	fmt.Println(err)
	return false
}

// Call is the one-time RPC communication utility. No matter successfully or
// not, the channel will be closed.
func Call(network, srv, name string, args interface{}, reply interface{}) bool {
	if c := DialServer(network, srv); c != nil {
		ok := RPCCall(c, name, args, reply)
		if ok {
			c.Close()
		}
		return ok
	}
	return false
}
