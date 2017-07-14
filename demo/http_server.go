package main

import (
	"distributed-system/http"
	"fmt"
	"os"
)

func main() {
	server := &http.Server{Addr: ":8080"}

	server.AddHandlerFunc("/index", func(w http.Response, r *http.Request) {
		// logics
	})

	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
	} else {
		fmt.Println("Start server")
	}
}
