package http

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

const (
	SERVER_ADDR = ":8080"
	HTTP_HOST   = "http://localhost" + SERVER_ADDR
)

func setupServer() *Server {
	server := &Server{Addr: SERVER_ADDR}

	// start server
	go func() {
		if err := server.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
		} else {
			fmt.Println("Start server")
		}
	}()
	return server
}

func setOKResponse(resp *Response) {
	resp.Status = "OK"
	resp.StatusCode = StatusOK
}

func checkGetRequest(t *testing.T, url string, statusCode int, body []byte) {
	var readBuffer [1024]byte
	if resp, err := Get(url); err != nil || resp == nil {
		t.Fatalf("Get(%v) failed", url)
	} else {
		if resp.StatusCode != statusCode {
			t.Fatalf("Get(%v) -> %v, expected %v", url, resp.StatusCode, statusCode)
		}
		bodyIndex := 0
		readCnt := 0
		for {
			n, err := resp.Body.Read(readBuffer[:])
			readCnt += n
			if n == 0 {
				if bodyIndex != len(body) {
					t.Fatalf("Get(%v), body length: %v < expected %v", url, readCnt, len(body))
				} else if err == io.EOF {
					break
				} else {
					continue
				}
			} else if n+bodyIndex > len(body) {
				t.Fatalf("Get(%v) -> body length > expected %v", url, len(body))
			} else {
				if bytes.Compare(readBuffer[:n], body[bodyIndex:bodyIndex+n]) != 0 {
					t.Fatalf("Get(%v) -> %v, expected %v", url, string(readBuffer[:n]), string(body[bodyIndex:bodyIndex+n]))
				}
				bodyIndex += n
			}
		}
	}
}

// Test GET method in a serial way. We leave the REST
// semantics alone now, i.e. ignoring the GET read-
// only sementics.
func TestGetBasic(t *testing.T) {
	var value int64 = 0

	server := setupServer()
	server.AddHandlerFunc("/incr", func(resp *Response, req *Request) {
		value += 1
		setOKResponse(resp)
	})

	server.AddHandlerFunc("/decr", func(resp *Response, req *Request) {
		value -= 1
		setOKResponse(resp)
	})

	server.AddHandlerFunc("/value", func(resp *Response, req *Request) {
		resp.Body = bytes.NewBufferString(strconv.Itoa(int(value)))
		setOKResponse(resp)
	})

	checkGetRequest(t, "/incr", StatusOK, []byte(""))
	if value != 1 {
		t.Fatalf("value -> %v, expected %v", value, 1)
	}
	checkGetRequest(t, "/value", StatusOK, []byte(strconv.Itoa(int(value))))

	checkGetRequest(t, "/decr", StatusOK, []byte(""))
	if value != 0 {
		t.Fatalf("value -> %v, expected %v", value, 0)
	}
	checkGetRequest(t, "/value", StatusOK, []byte(strconv.Itoa(int(value))))

	server.Shutdown()
}

// Test GET method following the above code,
// additionally in the concurrent way.
func TestGetConcurrence(t *testing.T) {
	runtime.GOMAXPROCS(4)
	var value int64 = 0

	server := setupServer()
	server.AddHandlerFunc("/incr", func(resp *Response, req *Request) {
		atomic.AddInt64(&value, 1)
		setOKResponse(resp)
	})

	server.AddHandlerFunc("/decr", func(resp *Response, req *Request) {
		atomic.AddInt64(&value, -1)
		setOKResponse(resp)
	})

	server.AddHandlerFunc("/value", func(resp *Response, req *Request) {
		resp.Body = bytes.NewBufferString(strconv.Itoa(int(value)))
		setOKResponse(resp)
	})

	incrNum, decrNum := 100, 50
	type Status struct {
		flag bool
		url  string
	}

	waitComplete := make(chan Status, incrNum+decrNum)
	go func() {
		for i := 0; i < incrNum; i++ {
			go func(ii int) {
				if resp, err := Get("/incr"); err != nil || resp == nil {
					waitComplete <- Status{flag: false, url: "/incr"}
				} else {
					waitComplete <- Status{flag: true}
				}
			}(i)
		}
	}()

	go func() {
		for i := 0; i < decrNum; i++ {
			go func(ii int) {
				if resp, err := Get("/decr"); err != nil || resp == nil {
					waitComplete <- Status{flag: false, url: "/decr"}
				} else {
					waitComplete <- Status{flag: true}
				}
			}(i)
		}
	}()

	for i := 0; i < cap(waitComplete); i++ {
		select {
		case status := <-waitComplete:
			if !status.flag {
				t.Fatalf("Get(%v) failed", status.url)
			}
		case <-time.After(time.Millisecond * 10):
			t.Fatalf("wait reply timeout")
		}
	}

	expectedValue := value + int64(incrNum-decrNum)
	if value != expectedValue {
		t.Fatalf("value -> %v, expected %v", value, expectedValue)
	}
	checkGetRequest(t, "/value", StatusOK, []byte(strconv.Itoa(int(value))))
	server.Shutdown()
}
