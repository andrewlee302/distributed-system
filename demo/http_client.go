package main

import (
	"distributed-system/http"
	"fmt"
	"os"
)

func main() {
	if resp, err := http.Get("http://www.baidu.com"); err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
	} else {
		fmt.Println(resp.Status)
	}
}
