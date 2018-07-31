package tinykv

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

func checkCall(t *testing.T, val string, existed bool, expectedVal string, expectedFlag bool) {
	if val != expectedVal || existed != expectedFlag {
		t.Fatalf("wrong reply \"%v\" %v; expected \"%v\" %v", val, existed, expectedVal, expectedFlag)
	}
}

func TestBasic(t *testing.T) {
	fmt.Printf("Test: Basic kvstore R/W ...\n")
	srvAddr := "localhost:9091"
	ts := NewKVStoreService("tcp", srvAddr)
	ts.Serve()
	defer ts.Kill()

	client := NewClient(srvAddr)

	value, existed := client.Get("key1")
	checkCall(t, value, existed, "", false)

	oldValue, existed := client.Put("key1", "1")
	checkCall(t, oldValue, existed, "", false)

	value, existed = client.Get("key1")
	checkCall(t, value, existed, "1", true)
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
			oldValue, existed := client.Put("key"+strconv.Itoa(i), "1")
			checkCall(t, oldValue, existed, "", false)
		}(i)
	}
	wg.Wait()

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			value, existed := client.Get("key" + strconv.Itoa(i))
			checkCall(t, value, existed, "1", true)
		}(i)
	}
	wg.Wait()
	fmt.Printf("  ... Passed\n")
}
