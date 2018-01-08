package main

import (
	"distributed-system/http"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"sync/atomic"
)

func main() {
	var ServerAddr string
	flag.StringVar(&ServerAddr, "server_addr", "", "e.g. localhost:8080")
	flag.Parse()
	if ServerAddr == "" {
		flag.PrintDefaults()
		return
	}

	server := http.NewServer(ServerAddr)
	var value int64
	server.AddHandlerFunc("/add", wrapYourAddFunc(&value))
	server.AddHandlerFunc("/value", wrapYourValueFunc(&value))

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("%v\n", err)
	} else {
		fmt.Printf("Server closed\n")
	}
}

// Add closure. Add the delta to the value and respond with nothing.
// The delta may be negative.
func wrapYourAddFunc(value *int64) func(resp *http.Response, req *http.Request) {
	return func(resp *http.Response, req *http.Request) {
		vb, err := ioutil.ReadAll(req.Body)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		delta, err := strconv.ParseInt(string(vb), 10, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		atomic.AddInt64(value, delta)
		resp.WriteStatus(http.StatusOK)
		resp.Write([]byte{})
	}
}

// Value closure. Respond with the value.
func wrapYourValueFunc(value *int64) func(resp *http.Response, req *http.Request) {
	return func(resp *http.Response, req *http.Request) {
		resp.WriteStatus(http.StatusOK)
		resp.Write([]byte(strconv.FormatInt(atomic.LoadInt64(value), 10)))
	}
}
