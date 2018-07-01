package http

import (
	"io"
	"net/url"
)

// RFC2616 Method
const (
	MethodGet  = "GET"
	MethodPost = "POST"
)

// HTTP literal constants.
const (
	// HTTPVersion is unique http proto here.
	HTTPVersion = "HTTP/1.1"

	HeaderContentLength    = "Content-Length"
	HeaderHost             = "Host"
	HeaderContentTypeValue = "text/plain"
)

// Buffer size.
const (
	ServerRequestBufSize  = 256
	ServerResponseBufSize = 256
	ClientResponseBufSize = 256
	ClientRequestBufSize  = 256
	WriteBuffInitSize     = 256
)

// Request is the Http Request. The format is below.
//  Request = Request-Line
//             *(( general-header | request-header | entity-header ) CRLF)
//            CRLF
//            [message-body]
//  Request-Line = Method SP Request-URI SP HTTP-Version CRLF
//  Header = Key: Value CRLF
type Request struct {
	Method string
	URL    *url.URL
	Proto  string

	// Header is key-value pair for simplicity.
	Header        map[string]string
	ContentLength int64
	Body          io.Reader
}

// Response is the Http Response. The format is below.
//  Response = Status-Line
//            *(( general-header | response-header | entity-header ) CRLF)
//            CRLF
//            [ message-body ]
//  Status-Line = HTTP-Version SP Status-Code SP Reason-Phrase CRLF
//  Header = Key: Value CRLF
type Response struct {
	Status     string
	StatusCode int
	Proto      string

	// Header is key-value pair for simplicity.
	Header        map[string]string
	ContentLength int64

	// Body represents the response body.
	//
	// The http Client and Transport guarantee that Body is always
	// non-nil, even on responses without a body or responses with
	// a zero-length body. It is the caller's responsibility to
	// close Body. It does not attempt to reuse TCP connections
	// unless the Body is closed.
	Body *ResponseReader

	writeBuff []byte
}

// ResponseReader is reader of the response body.
type ResponseReader struct {
	c    *Client
	conn io.Reader
	host string
	r    io.Reader
}

// Read implements io.Reader interface.
func (reader *ResponseReader) Read(p []byte) (n int, err error) {
	return reader.r.Read(p)
}

// Close should be called to release the TCP connection.
// It is the caller's responsibility to close Body. It
// implements io.Closer interface.
func (reader *ResponseReader) Close() {
	// Put back the connection for the possible future use.
	reader.c.connPools.Put(reader.host, reader.conn.(io.ReadWriteCloser))
}

// Write writes data to the response body and update the
// Content-Length header.
func (resp *Response) Write(data []byte) {
	if len(data) == 0 {
		return
	}
	resp.ContentLength += int64(len(data))
	if resp.writeBuff == nil {
		resp.writeBuff = make([]byte, 0, WriteBuffInitSize)
	}
	resp.writeBuff = append(resp.writeBuff, data...)
}

// WriteStatus sets the status code of the response.
func (resp *Response) WriteStatus(code int) {
	status, ok := statusText[code]
	if !ok {
		panic("Code doesn't exist in HTTP/1.1(RFC2616) protocol")
	}
	resp.StatusCode = code
	resp.Status = status
}

var unixDir = "/var/tmp"

// UnixDir sets the dir for unix domain sockets. The default value is
// "/var/tmp"
func UnixDir(dir string) {
	unixDir = dir
}

// UnixSocketFile gets the corresponding domain socket file from the host.
// In case that unix dir is "/var/tmp/", the relation is below.
//  "localhost:8080" --> "/var/tmp/localhost:8080"
func UnixSocketFile(host string) string {
	return unixDir + "/" + host
}
