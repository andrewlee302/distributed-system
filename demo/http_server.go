package main

import (
	"distributed-system/http"
	"fmt"
	"os"
	"time"
)

func main() {
	server := &http.Server{Addr: ":8080"}

	server.AddHandlerFunc("/index", func(w http.Response, r *http.Request) {
		// logics
		fmt.Println("Response to /index")
	})

	// start server
	go func() {
		if err := server.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
		} else {
			fmt.Println("Start server")
		}
	}()

	// shutdown it in 5 seconds.
	time.Sleep(5 * time.Second)
	fmt.Println("Prepare to shutdown the server")
	server.Shutdown()
	fmt.Println("The server has been closed")
}
