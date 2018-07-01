/*
Package http provides server and client interfaces in http protocol.

The reliable transmission protocol could be TCP and unix domain socket, both of
which support transmission of a reliable stream of bytes.

For TCP transmission, IP and port could be extracted from the URL.
To extend unix domain socket as the stream channel, we provide a transformation
from URL to unix domain socket file description.

Default unix domain directory is /var/tmp/. The unix file for a host is located
in the unix domain directory. For example.
 http://localhost:8080/add --> socket:/var/tmp/localhost:8080 path:/add

Features

The library supports concurrent and keep-alive http requests. But it doesn't
support chuck transfer encoding for large data transferring.

 * Server always uses keep-alive http connections regardless of "Connection:
 keep-alive" header.
 * Content-Length and Host headers are necessary in requests.
 * Content-Length header is necessary in responses.
 * Header value is single.
 * Request-URI must be absolute path. Like: "/add", "/incr".
*/
package http

// Http server library.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Server here resembles ServeMux in golang standard lib.
// Refer to https://golang.org/pkg/net/http/#ServeMux.
type Server struct {
	Addr     string
	Network  string
	l        net.Listener
	mu       sync.Mutex
	doneChan chan struct{}

	// Your data here.
	handlers   map[string]Handler
	activeConn map[*httpConn]struct{}
}

// NewServer initilizes the server of the speficif host.
// The host param includes the hostname and port.
func NewServer(network string, host string) (s *Server) {
	var addr string
	var err error
	if network == "tcp" {
		addr = host
	} else if network == "unix" {
		addr = UnixSocketFile(host)
		defer os.Mkdir(unixDir, 0777)
	} else {
		return nil
	}
	if err != nil {
		return nil
	}
	srv := &Server{Addr: addr, Network: network}
	srv.doneChan = make(chan struct{})

	// Your initialization code here.
	srv.handlers = make(map[string]Handler)
	srv.activeConn = make(map[*httpConn]struct{})
	return srv
}

// Handler processes the HTTP request and get the response.
//
// Handler should not modify the request.
type Handler interface {
	ServeHTTP(resp *Response, req *Request)
}

// HandlerFunc responds to an HTTP request.
// Behave the same as he Handler.
type HandlerFunc func(resp *Response, req *Request)

// ServeHTTP calls f(w, r).
func (f HandlerFunc) ServeHTTP(w *Response, r *Request) {
	f(w, r)
}

// NotFoundHandler gives 404 with the blank content.
var NotFoundHandler HandlerFunc = func(resp *Response, req *Request) {
	resp.Write([]byte{})
	resp.WriteStatus(StatusNotFound)
}

// AddHandlerFunc adds handlerFunc to the list of handlers.
func (srv *Server) AddHandlerFunc(pattern string, handlerFunc HandlerFunc) {
	srv.AddHandler(pattern, handlerFunc)
}

// AddHandler adds handler to the list of handlers.
//
// "" pattern or nil handler is forbidden.
func (srv *Server) AddHandler(pattern string, handler Handler) {
	if pattern == "" {
		panic("http: invalid pattern " + pattern)
	}
	if handler == nil {
		panic("http: nil handler")
	}

	// TODO
	srv.handlers[pattern] = handler

}

// Find a handler matching the path using most-specific
// (longest) matching. If no handler matches, return
// the NotFoundHandler.
func (srv *Server) match(path string) (h Handler) {
	// TODO
	matchLen := 0
	for k, v := range srv.handlers {
		if pathMatch(k, path) && len(k) > matchLen {
			matchLen = len(k)
			h = v
		}
	}
	if h == nil {
		h = NotFoundHandler
	}
	return
}

// Does path match pattern?
// "/" matches path: "/*"
// "/cart/" matches path: "/cart/*"
// "/login" only matches path: "/login"
func pathMatch(pattern, path string) bool {
	if len(pattern) == 0 {
		// should not happen
		return false
	}
	n := len(pattern)
	if pattern[n-1] != '/' {
		return pattern == path
	}
	return len(path) >= n && path[0:n] == pattern
}

// Close immediately closes active net.Listener and any
// active connections.
//
// Close returns any error returned from closing the Server's
// underlying Listener.
func (srv *Server) Close() (err error) {
	// TODO
	srv.mu.Lock()
	defer srv.mu.Unlock()

	select {
	case <-srv.doneChan:
		// Already closed. Don't close again.
	default:
		// Safe to close here. We're the only closer, guarded
		// by s.mu.
		close(srv.doneChan)
	}
	err = srv.l.Close()

	// TODO
	for c := range srv.activeConn {
		c.closeLocked()
	}
	return
}

// ErrServerClosed is returned by the Server's Serve, ListenAndServe,
// and ListenAndServeTLS methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("http: Server closed")

// ListenAndServe starts listening and serve http connections.
// The method is blocking, which doesn't return until other
// goroutines close the server.
func (srv *Server) ListenAndServe() (err error) {
	// TODO
	// listen on the specific tcp addr, then call Serve()
	var l net.Listener
	if srv.Network == "tcp" {
		l, err = net.Listen("tcp", srv.Addr)
	} else {
		l, err = net.Listen("unix", srv.Addr)
	}
	if err != nil {
		return
	}
	defer l.Close()
	srv.l = l

	// TODO
	// wait loop for accepting new connection (httpConn), then
	// serve in the asynchronous style.
	for {
		conn, err := l.Accept()
		if err != nil {
			select {
			case <-srv.doneChan:
				return ErrServerClosed
			default:
			}
			return err
		}
		c := srv.newConn(srv.Network, conn)
		srv.mu.Lock()
		srv.activeConn[c] = struct{}{}
		srv.mu.Unlock()
		go c.serve()
	}
}

func (srv *Server) newConn(network string, conn net.Conn) *httpConn {
	if network == "tcp" {
		return &httpConn{srv: srv, rw: conn.(*net.TCPConn)}
	} else {
		return &httpConn{srv: srv, rw: conn.(*net.UnixConn)}
	}
}

// Step flags for request strem processing.
const (
	RequestStepRequestLine = iota
	RequestStepHeader
	RequestStepBody
)

// A httpConn represents an HTTP connection in the server side.
type httpConn struct {
	srv *Server
	rw  io.ReadWriter
}

// Serve a new connection.
func (hc *httpConn) serve() {
	// Server the http connection in loop way until something goes wrong.
	// The http connection will be closed in the case of errors.
	for {
		// Construct the request from the TCP stream.
		req, err := hc.constructReq()
		if err != nil {
			hc.close()
			return
		}
		resp := &Response{Proto: HTTPVersion, Header: make(map[string]string)}

		// Find the matched handler.
		handler := hc.srv.match(req.URL.Path)

		// Handler it in user-defined logics or NotFoundHandler.
		handler.ServeHTTP(resp, req)

		// The response must contain HeaderContentLength in its Header.
		resp.Header[HeaderContentLength] = strconv.FormatInt(resp.ContentLength, 10)

		// *** Discard rest of request body.
		io.Copy(ioutil.Discard, req.Body)

		// Write the response to the TCP stream.
		err = hc.writeResp(resp)
		if err != nil {
			hc.close()
			return
		}
	}
}

func (hc *httpConn) closeLocked() {
	if hc.srv.Network == "tcp" {
		hc.rw.(*net.TCPConn).Close()
	} else {
		hc.rw.(*net.UnixConn).Close()
	}
	delete(hc.srv.activeConn, hc)
}

// Close the http connection.
func (hc *httpConn) close() {
	// TODO
	hc.srv.mu.Lock()
	defer hc.srv.mu.Unlock()
	hc.closeLocked()
}

// Write the response to the TCP stream.
//
// If TCP errors occur, err is not nil.
func (hc *httpConn) writeResp(resp *Response) (err error) {
	// TODO
	writer := bufio.NewWriterSize(hc.rw, ServerResponseBufSize)
	_, err = writer.WriteString(fmt.Sprintf("%s %d %s\n", resp.Proto, resp.StatusCode, resp.Status))
	if err != nil {
		return
	}

	for key, value := range resp.Header {
		// fmt.Println("header:", fmt.Sprintf("%s: %s\n", key, value))
		_, err = writer.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		if err != nil {
			return
		}
	}
	err = writer.WriteByte('\n')
	if err != nil {
		return
	}
	// fmt.Println("body", resp.writeBuff)
	_, err = writer.Write(resp.writeBuff[:resp.ContentLength])
	if err != nil {
		return
	}
	err = writer.Flush()
	if err != nil {
		return
	}
	return
}

// Construct the request from the TCP stream.
//
// If TCP errors occur, err is not nil and req is nil.
// Request header must contain the Content-Length.
func (hc *httpConn) constructReq() (*Request, error) {
	// TODO
	req := &Request{Header: make(map[string]string)}
	reader := bufio.NewReaderSize(hc.rw, ServerRequestBufSize)
	var wholeLine []byte
	var lastWait = false
	var step = RequestStepRequestLine
LOOP:
	for {
		if line, isWait, err := reader.ReadLine(); err == nil {
			if !isWait {
				// Complete line
				if !lastWait {
					wholeLine = line
				} else {
					wholeLine = append(wholeLine, line...)
				}
				// Process the line
				switch step {
				case RequestStepRequestLine:
					{
						reqLineWords := strings.SplitN(string(wholeLine), " ", 3)
						// fmt.Println("RequestStepRequestLine", reqLineWords)
						if len(reqLineWords) != 3 || reqLineWords[0] != MethodPost &&
							reqLineWords[0] != MethodGet || reqLineWords[2] != HTTPVersion {
							return nil, errors.New("Invalid request line")
						}
						req.Method = reqLineWords[0]
						urlObj, err := url.ParseRequestURI(reqLineWords[1])

						if err != nil {
							return nil, err
						}
						req.URL = urlObj
						req.Proto = reqLineWords[2]
						step = RequestStepHeader

					}
				case RequestStepHeader:
					{
						if len(line) != 0 {
							headerWords := strings.SplitN(string(wholeLine), ": ", 2)
							if len(headerWords) != 2 {
								return nil, errors.New("Invalid request header")
							}
							req.Header[headerWords[0]] = headerWords[1]

						} else {
							step = RequestStepBody
							if cLenStr, ok := req.Header[HeaderContentLength]; !ok {
								if req.Method == MethodPost {
									return nil, errors.New("No Content-Length in POST request header")
								}
								req.Header[HeaderContentLength] = "0"
								req.ContentLength = 0

							} else {
								cLen, err := strconv.Atoi(cLenStr)
								if err != nil {
									return nil, errors.New("Content-Length must be numeric")
								}
								req.ContentLength = int64(cLen)
							}
							// transfer the body to Request
							req.Body = &io.LimitedReader{
								R: reader,
								N: req.ContentLength,
							}

							break LOOP
						}
					}
				case RequestStepBody:
					{
						panic("Cannot be here")
					}
				}

			} else {
				// Not complete
				if !lastWait {
					wholeLine = line
				} else {
					wholeLine = append(wholeLine, line...)
				}
			}
			lastWait = isWait
		} else if err == io.EOF {
			return nil, err
		} else {
			return nil, err
		}
	}
	return req, nil
}
