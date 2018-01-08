package main

import (
	your_http "distributed-system/http"
	"flag"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

func main() {
	runtime.GOMAXPROCS(8)
	var ServerAddr string
	flag.StringVar(&ServerAddr, "server_addr", "", "e.g. localhost:8080")
	flag.Parse()
	if ServerAddr == "" {
		flag.PrintDefaults()
		return
	}
	HTTPHost := "http://" + ServerAddr

	c := new(http.Client)
	incrNum, decrNum := 100, 50
	type Status struct {
		flag bool
		url  string
		err  error
	}

	start := time.Now()
	waitComplete := make(chan Status, incrNum+decrNum)
	go func() {
		for i := 0; i < incrNum; i++ {
			go func(ii int) {
				if resp, err := c.Post(HTTPHost+"/add", your_http.HeaderContentTypeValue, strings.NewReader("1")); err != nil || resp == nil {
					waitComplete <- Status{flag: false, url: "/add", err: err}
				} else {
					waitComplete <- Status{flag: true}
				}
			}(i)
		}
	}()

	go func() {
		for i := 0; i < decrNum; i++ {
			go func(ii int) {
				if resp, err := c.Post(HTTPHost+"/add", your_http.HeaderContentTypeValue, strings.NewReader("-1")); err != nil || resp == nil {
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
				fmt.Printf("Get(%v) failed, error:%v\n", status.url, status.err)
				return
			}
		case <-time.After(time.Millisecond * 5000):
			fmt.Println("wait reply timeout")
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("Cost: %v ms\n", elapsed.Nanoseconds()/int64(time.Millisecond))
}
