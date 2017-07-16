package http

import (
	"net"
)

type Server struct {
	Addr string
	// TODO
}

// A Handler responds to an HTTP request.
//
// Except for reading the body, handlers should not modify the
// provided Request.
type Handler interface {
	ServeHTTP(*Response, *Request)
}

type HandlerFunc func(Response, *Request)

func (s *Server) AddHandler(path string, handler Handler) {
	// TODO
}

func (s *Server) AddHandlerFunc(path string, handlerFunc HandlerFunc) {
	// TODO
}

// Shutdown the server without interrupting any active
// connections.
func (srv *Server) Shutdown() (err error) {
	// TODO
	return
}

// Start listen and http service.
// The method is blocking, which doesn't return until other
// goroutines shutdown the server.
func (srv *Server) ListenAndServe() (err error) {
	// TODO
	// listen on the specific port, then call Serve()
	return
}

// Start serve the http connections
func (srv *Server) Serve() (err error) {
	// wait loop for accepting new connection (httpConn), then
	// serve in the asynchronous style.
	return
}

// A http connection processing the request and
// replying with a response.
type httpConn struct {
	srv        *Server
	conn       net.Conn
	remoteAddr string
}

// Serve a new connection.
func (c *httpConn) serve() {
	// TODO
}
