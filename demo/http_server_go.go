package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
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

	serverMux := http.NewServeMux()
	var value int64
	serverMux.HandleFunc("/add", wrapGoAddFunc(&value))
	serverMux.HandleFunc("/value", wrapGoValueFunc(&value))

	server := http.Server{
		Addr:    ServerAddr,
		Handler: serverMux,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Printf("%v\n", err)
	} else {
		fmt.Printf("Server closed\n")
	}
}

// Add closure for golang standard server. Add the delta to the value
// and respond without anything. The delta may be negative.
func wrapGoAddFunc(value *int64) func(resp http.ResponseWriter, req *http.Request) {
	return func(resp http.ResponseWriter, req *http.Request) {
		vb, err := ioutil.ReadAll(req.Body)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		delta, err := strconv.ParseInt(string(vb), 10, 64)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		atomic.AddInt64(value, delta)
		resp.WriteHeader(http.StatusOK)
		resp.Write([]byte{})
	}
}

// Value closure for golang standard server. Respond with the value.
func wrapGoValueFunc(value *int64) func(resp http.ResponseWriter, req *http.Request) {
	return func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusOK)
		resp.Write([]byte(strconv.FormatInt(atomic.LoadInt64(value), 10)))
	}
}
