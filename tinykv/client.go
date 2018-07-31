package tinykv

import (
	"distributed-system/kv"
	"distributed-system/util"
	"fmt"
	"net"
	"net/rpc"
	"syscall"
)

type Client struct {
	SrvAddr   string
	rpcClient *rpc.Client
}

func NewClient(srvAddr string) *Client {
	rpcClient, err := rpc.Dial("tcp", srvAddr)
	if err != nil {
		err1 := err.(*net.OpError)
		if err1.Err != syscall.ENOENT && err1.Err != syscall.ECONNREFUSED {
			fmt.Printf("TinyKVStore Dial() failed: %v\n", err1)
		}
		return nil
	}
	return &Client{SrvAddr: srvAddr, rpcClient: rpcClient}
}

func (c *Client) Get(key string) (value string, existed bool) {
	args := &kv.GetArgs{Key: key}
	var reply kv.Reply
	util.RPCCall(c.rpcClient, "KVStoreService.RPCGet", args, &reply)
	value, existed = reply.Value, reply.Flag
	return
}

func (c *Client) Put(key string, value string) (oldValue string, existed bool) {
	args := &kv.PutArgs{Key: key, Value: value}
	var reply kv.Reply
	util.RPCCall(c.rpcClient, "KVStoreService.RPCPut", args, &reply)
	oldValue, existed = reply.Value, reply.Flag
	return
}

func (c *Client) Incr(key string, delta int) (oldValue string, existed bool) {
	args := &kv.IncrArgs{Key: key, Delta: delta}
	var reply kv.Reply
	util.RPCCall(c.rpcClient, "KVStoreService.RPCIncr", args, &reply)
	oldValue, existed = reply.Value, reply.Flag
	return
}

func (c *Client) Del(key string) (oldValue string, existed bool) {
	args := &kv.DelArgs{Key: key}
	var reply kv.Reply
	util.RPCCall(c.rpcClient, "KVStoreService.RPCDel", args, &reply)
	oldValue, existed = reply.Value, reply.Flag
	return
}

func (c *Client) Close() {
	c.rpcClient.Close()
}
