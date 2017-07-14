package http

import (
	"io"
)

// Method: GET.
func Get(url string) (resp *Response, err error) {
	// TODO
	return
}

// Method: POST.
func Post(url string, h Header, body io.Reader) (resp *Response, err error) {
	// TODO
	return
}

// Method: PUT.
func Put(url string, h Header, body io.Reader) (resp *Response, err error) {
	// TODO
	return
}

// Method: DELETE.
func Delete(url string, h Header, body io.Reader) (resp *Response, err error) {
	// TODO
	return
}

// Send http request to the server.
func Send(req *Request) (resp *Response, err error) {
	// TODO
	return
}
