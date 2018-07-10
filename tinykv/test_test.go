package tinykv

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

func checkCall(t *testing.T, ok bool, reply, expected Reply) {
	if !ok {
		t.Fatalf("call error")
	} else if reply != expected {
		t.Fatalf("wrong reply %v; expected %v", reply, expected)
	}
}

func TestBasic(t *testing.T) {
	fmt.Printf("Test: Basic kvstore R/W ...\n")
	srvAddr := "localhost:9091"
	ts := NewKVStoreService("tcp", srvAddr)
	ts.Serve()
	defer ts.Kill()

	client := NewClient(srvAddr)

	ok, reply := client.Get("key1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok, reply = client.Put("key1", "1")
	checkCall(t, ok, reply, Reply{Flag: false, Value: ""})

	ok, reply = client.Get("key1")
	checkCall(t, ok, reply, Reply{Flag: true, Value: "1"})
	fmt.Printf("  ... Passed\n")
}

func TestConcurrent(t *testing.T) {
	runtime.GOMAXPROCS(4)
	fmt.Printf("Test: Concurrent kvstore R/W ...\n")

	srvAddr := "localhost:9090"
	ts := NewKVStoreService("tcp", srvAddr)
	ts.Serve()
	defer ts.Kill()
	// StartTinyStore(srvAddr)

	client := NewClient(srvAddr)

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, reply := client.Put("key"+strconv.Itoa(i), "1")
			checkCall(t, ok, reply, Reply{Flag: false, Value: ""})
		}(i)
	}
	wg.Wait()

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, reply := client.Get("key" + strconv.Itoa(i))
			checkCall(t, ok, reply, Reply{Flag: true, Value: "1"})
		}(i)
	}
	wg.Wait()
	fmt.Printf("  ... Passed\n")
}
