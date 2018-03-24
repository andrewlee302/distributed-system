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
	err := c.Call(name, args, reply)
	if err == nil {
		pool.Put(c)
		return true
	}
	c.Close()
	pool.Clean(c)
	fmt.Println(err)
	return false
}

func RPCPoolArrayCall(pa *ResourcePoolsArray, i int, name string, args interface{}, reply interface{}) bool {
	// now := time.Now()
	// defer func() {
	// 	atomic.AddInt64(&RPCCallNs, time.Since(now).Nanoseconds())
	// }()
	c := pa.Get(i).(*rpc.Client)
	err := c.Call(name, args, reply)
	if err == nil {
		pa.Put(i, c)
		return true
	}
	c.Close()
	pa.Clean(i, c)
	fmt.Println(err)
	return false
}

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
