package http

import (
	"bytes"
	"fmt"
	"io/ioutil"
	go_http "net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Server host.
const (
	ServerAddr = "localhost:9000"
	HTTPHost   = "http://" + ServerAddr
)

func setupYourServer() (server *Server, ch chan error) {
	ch = make(chan error)
	server = NewServer(ServerAddr)

	// start server
	go func() {
		if err := server.ListenAndServe(); err != nil && err != ErrServerClosed {
			ch <- err
		} else {
			close(ch)
		}
	}()
	time.Sleep(time.Second)
	return server, ch
}

func setupGoServer() (server *go_http.Server, serverMux *go_http.ServeMux, ch chan error) {
	ch = make(chan error)
	serverMux = go_http.NewServeMux()
	server = &go_http.Server{
		Addr:    ServerAddr,
		Handler: serverMux,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != go_http.ErrServerClosed {
			ch <- err
		} else {
			close(ch)
		}
	}()
	time.Sleep(time.Second)
	return server, serverMux, ch
}

func setOKResponse(resp *Response) {
	resp.Write([]byte{})
	resp.WriteStatus(StatusOK)
}

// Use golang standard client and your server to check your server.
func checkYourServer(t *testing.T, client *go_http.Client, path string, method string,
	reqBodyData []byte, statusCode int, expectedRespBodyData []byte) {
	url := HTTPHost + path
	var resp *go_http.Response
	var err error
	if method == MethodGet {
		resp, err = client.Get(url)
	} else {
		resp, err = client.Post(url, HeaderContentTypeValue, bytes.NewReader(reqBodyData))
	}
	if err != nil || resp == nil {
		t.Fatalf("Get(%v) failed, error: %v", url, err)
	} else {
		if resp.StatusCode != statusCode {
			t.Fatalf("Get(%v) status=%v, expected=%v",
				url, resp.StatusCode, statusCode)
		}
		respBodyData, _ := ioutil.ReadAll(resp.Body)
		if bytes.Compare(respBodyData, expectedRespBodyData) != 0 {
			t.Fatalf("Get(%v) body=%v, expected=%v",
				url, string(respBodyData), string(expectedRespBodyData))
		}
	}
}

// Use your client and golang standard server to check your client.
func checkYourClient(t *testing.T, client *Client, path string, method string,
	reqBodyData []byte, statusCode int, expectedRespBodyData []byte) {
	url := HTTPHost + path
	var resp *Response
	var err error
	if method == MethodGet {
		resp, err = client.Get(url)
	} else {
		resp, err = client.Post(url, int64(len(reqBodyData)), bytes.NewReader(reqBodyData))
	}
	if err != nil || resp == nil {
		t.Fatalf("Get(%v) failed, error: %v", url, err)
	} else {
		if resp.StatusCode != statusCode {
			t.Fatalf("Get(%v) status=%v, expected=%v", url, resp.StatusCode, statusCode)
		}
		respBodyData, _ := ioutil.ReadAll(resp.Body)
		if bytes.Compare(respBodyData, expectedRespBodyData) != 0 {
			t.Fatalf("Get(%v) body=%v, expected=%v", url, string(respBodyData), string(expectedRespBodyData))
		}
	}
}

// Add closure. Add the delta to the value and respond with nothing.
// The delta may be negative.
func wrapYourAddFunc(value *int64) func(resp *Response, req *Request) {
	return func(resp *Response, req *Request) {
		vb, err := ioutil.ReadAll(req.Body)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		delta, err := strconv.ParseInt(string(vb), 10, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		atomic.AddInt64(value, delta)
		resp.WriteStatus(StatusOK)
		resp.Write([]byte{})
	}
}

// Value closure. Respond with the value.
func wrapYourValueFunc(value *int64) func(resp *Response, req *Request) {
	return func(resp *Response, req *Request) {
		resp.WriteStatus(StatusOK)
		resp.Write([]byte(strconv.FormatInt(atomic.LoadInt64(value), 10)))
	}
}

// Add closure for golang standard server. Add the delta to the value
// and respond without anything. The delta may be negative.
func wrapGoAddFunc(value *int64) func(resp go_http.ResponseWriter, req *go_http.Request) {
	return func(resp go_http.ResponseWriter, req *go_http.Request) {
		vb, err := ioutil.ReadAll(req.Body)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		delta, err := strconv.ParseInt(string(vb), 10, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		atomic.AddInt64(value, delta)
		resp.WriteHeader(go_http.StatusOK)
		resp.Write([]byte{})
	}
}

// Value closure for golang standard server. Respond with the value.
func wrapGoValueFunc(value *int64) func(resp go_http.ResponseWriter, req *go_http.Request) {
	return func(resp go_http.ResponseWriter, req *go_http.Request) {
		resp.WriteHeader(go_http.StatusOK)
		resp.Write([]byte(strconv.FormatInt(atomic.LoadInt64(value), 10)))
	}
}

// Test basic functions of your server.
// * Serial GET (/value) and POST(/add) requests and responses.
// * User-defined handler.
func TestServerBasic(t *testing.T) {
	server, sCloseChan := setupYourServer()
	c := new(go_http.Client)
	var value int64

	fmt.Printf("Test: Your basic server ...\n")

	server.AddHandlerFunc("/add", wrapYourAddFunc(&value))

	server.AddHandlerFunc("/value", wrapYourValueFunc(&value))

	checkYourServer(t, c, "/add", MethodPost, []byte("10"), StatusOK, []byte(""))
	if value != 10 {
		t.Fatalf("value -> %v, expected %v", value, 10)
	}
	checkYourServer(t, c, "/value", MethodGet, []byte{}, StatusOK, []byte(strconv.Itoa(int(value))))

	checkYourServer(t, c, "/add", MethodPost, []byte("-5"), StatusOK, []byte(""))
	if value != 5 {
		t.Fatalf("value -> %v, expected %v", value, 5)
	}
	checkYourServer(t, c, "/value", MethodGet, []byte{}, StatusOK, []byte(strconv.Itoa(int(value))))

	server.Close()
	if err := <-sCloseChan; err == nil {
		fmt.Printf("Server closed\n")
	} else {
		t.Fatalf("%v", err)
	}

	fmt.Printf("  ... Passed\n")
}

// Test the concurrency feature of your server.
func TestServerConcurrence(t *testing.T) {
	runtime.GOMAXPROCS(8)
	server, sCloseChan := setupYourServer()
	c := new(go_http.Client)
	var value int64

	fmt.Printf("Test: Concurrent perf of your server ...\n")

	server.AddHandlerFunc("/add", wrapYourAddFunc(&value))

	server.AddHandlerFunc("/value", wrapYourValueFunc(&value))

	incrReqNum, decrReqNum := 50, 50
	type Status struct {
		flag bool
		url  string
		err  error
	}
	start := time.Now()
	waitComplete := make(chan Status, incrReqNum+decrReqNum)
	go func() {
		for i := 0; i < incrReqNum; i++ {
			go func(ii int) {
				if resp, err := c.Post(HTTPHost+"/add", HeaderContentTypeValue, strings.NewReader("1")); err != nil || resp == nil {
					waitComplete <- Status{flag: false, url: "/add", err: err}
				} else {
					waitComplete <- Status{flag: true}
				}
			}(i)
		}
	}()

	go func() {
		for i := 0; i < decrReqNum; i++ {
			go func(ii int) {
				if resp, err := c.Post(HTTPHost+"/add", HeaderContentTypeValue, strings.NewReader("-1")); err != nil || resp == nil {
					waitComplete <- Status{flag: false, url: "/add", err: err}
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
				t.Fatalf("Get(%v) failed, error:%v", status.url, status.err)
			}
		case <-time.After(time.Millisecond * 500):
			t.Fatalf("wait reply timeout")
		}
	}
	elapsedMs := time.Since(start).Nanoseconds() / int64(time.Millisecond)
	fmt.Printf("Cost: %v ms\n", elapsedMs)

	expectedValue := int64(incrReqNum - decrReqNum)
	if value != expectedValue {
		t.Fatalf("value=%v, expected=%v", value, expectedValue)
	}
	checkYourServer(t, c, "/value", MethodGet, []byte{}, StatusOK, []byte(strconv.FormatInt(value, 10)))

	if elapsedMs > 500 {
		t.Fatalf("%v concurent client requests cost too much time: %v, expected < %v\n", incrReqNum+decrReqNum, elapsedMs, 50)
	}

	server.Close()
	if err := <-sCloseChan; err == nil {
		fmt.Printf("Server closed\n")
	} else {
		t.Fatalf("%v", err)
	}
	fmt.Printf("  ... Passed\n")
}

// Test your client functions.
// * Serial GET and POST requests and responses.
func TestClientBasic(t *testing.T) {
	runtime.GOMAXPROCS(8)
	server, serverMux, sCloseChan := setupGoServer()

	var value int64

	serverMux.HandleFunc("/add", wrapGoAddFunc(&value))
	serverMux.HandleFunc("/value", wrapGoValueFunc(&value))

	fmt.Printf("Test: Your basic client ...\n")
	c := NewClient()

	checkYourClient(t, c, "/add", MethodPost, []byte("10"), StatusOK, []byte(""))
	if value != 10 {
		t.Fatalf("value -> %v, expected %v", value, 10)
	}
	checkYourClient(t, c, "/value", MethodGet, []byte{}, StatusOK, []byte(strconv.Itoa(int(value))))

	checkYourClient(t, c, "/add", MethodPost, []byte("-5"), StatusOK, []byte(""))
	if value != 5 {
		t.Fatalf("value -> %v, expected %v", value, 5)
	}
	checkYourClient(t, c, "/value", MethodGet, []byte{}, StatusOK, []byte(strconv.Itoa(int(value))))

	server.Close()
	if err := <-sCloseChan; err == nil {
		fmt.Printf("Server closed\n")
	} else {
		t.Fatalf("%v", err)
	}
	fmt.Printf("  ... Passed\n")
}

// Test the concurrency and reuse features of your client.
func TestClientConcurrence(t *testing.T) {
	server, serverMux, sCloseChan := setupGoServer()

	var value int64
	serverMux.HandleFunc("/add", wrapGoAddFunc(&value))
	serverMux.HandleFunc("/value", wrapGoValueFunc(&value))

	c := NewClientSize(100)

	fmt.Printf("Test: Concurrent perf of your client ...\n")
	incrReqNum, decrReqNum := 5000, 5000
	type Status struct {
		flag bool
		url  string
		err  error
	}

	waitComplete := make(chan Status, incrReqNum+decrReqNum)
	start := time.Now()
	go func() {
		for i := 0; i < incrReqNum; i++ {
			go func(ii int) {
				reqBodyData := []byte("1")

				if resp, err := c.Post(HTTPHost+"/add", int64(len(reqBodyData)),
					bytes.NewReader(reqBodyData)); err != nil || resp == nil {
					waitComplete <- Status{flag: false, url: "/add", err: err}
				} else {
					waitComplete <- Status{flag: true}
				}
			}(i)
		}
	}()

	go func() {
		for i := 0; i < decrReqNum; i++ {
			go func(ii int) {
				reqBodyData := []byte("-1")
				if resp, err := c.Post(HTTPHost+"/add", int64(len(reqBodyData)),
					bytes.NewReader(reqBodyData)); err != nil || resp == nil {
					waitComplete <- Status{flag: false, url: "/add", err: err}
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
				t.Fatalf("Get(%v) failed, error:%v", status.url, status.err)
			}
		case <-time.After(time.Millisecond * 500):
			t.Fatalf("wait reply timeout")
		}
	}
	elapsedMs := time.Since(start).Nanoseconds() / int64(time.Millisecond)

	fmt.Printf("Cost: %v ms\n", elapsedMs)

	expectedValue := int64(incrReqNum - decrReqNum)
	if value != expectedValue {
		t.Fatalf("value=%v, expected=%v", value, expectedValue)
	}
	checkYourClient(t, c, "/value", MethodGet, []byte{}, StatusOK, []byte(strconv.Itoa(int(value))))

	if elapsedMs > 500 {
		t.Fatalf("%v concurent client requests cost too much time: %v, expected < %v\n", incrReqNum+decrReqNum, elapsedMs, 1000)
	}

	server.Close()
	if err := <-sCloseChan; err == nil {
		fmt.Printf("Server closed\n")
	} else {
		t.Fatalf("%v", err)
	}
	fmt.Printf("  ... Passed\n")
}
