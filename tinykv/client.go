package tinykv

import (
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
func (c *Client) Close() {
	c.rpcClient.Close()
}

func (c *Client) Put(key string, value string) (ok bool, reply Reply) {
	args := &PutArgs{Key: key, Value: value}
	ok = util.RPCCall(c.rpcClient, "KVStoreService.RPCPut", args, &reply)
	return
}

func (c *Client) Get(key string) (ok bool, reply Reply) {
	args := &GetArgs{Key: key}
	ok = util.RPCCall(c.rpcClient, "KVStoreService.RPCGet", args, &reply)
	return
}

func (c *Client) Incr(key string, delta int) (ok bool, reply Reply) {
	args := &IncrArgs{Key: key, Delta: delta}
	ok = util.RPCCall(c.rpcClient, "KVStoreService.RPCIncr", args, &reply)
	return
}
