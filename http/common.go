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
	WriteBuffInitSize     = 25
)

//
// -------- Request --------
// Request = Request-Line
//            *(( general-header | request-header | entity-header ) CRLF)
//           CRLF
//           [message-body]
// Request-Line = Method SP Request-URI SP HTTP-Version CRLF
// Header = Key: Value CRLF
// -------- Response -------
// Response = Status-Line
//           *(( general-header | response-header | entity-header ) CRLF)
//           CRLF
//           [ message-body ]
// Status-Line = HTTP-Version SP Status-Code SP Reason-Phrase CRLF
// Header = Key: Value CRLF
//
// Request for http request.
type Request struct {
	Method string
	URL    *url.URL
	Proto  string

	// Header is key-value pair for simplicity.
	Header        map[string]string
	ContentLength int64
	Body          io.Reader
}

// Response for http Response.
type Response struct {
	Status     string
	StatusCode int
	Proto      string

	// Header is key-value pair for simplicity.
	Header        map[string]string
	ContentLength int64

	// TODO refine
	// Body represents the response body.
	//
	// The http Client and Transport guarantee that Body is always
	// non-nil, even on responses without a body or responses with
	// a zero-length body. It is the caller's responsibility to
	// close Body. The default HTTP client's Transport does not
	// attempt to reuse HTTP/1.0 or HTTP/1.1 TCP connections
	// ("keep-alive") unless the Body is read to completion and is
	// closed.
	Body io.Reader

	// Your data here.
	writeBuff []byte
}

// For HTTP/1.1 requests, handlers should read any
// needed request body data before writing the response.
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

// WriteStatus sends status code.
func (resp *Response) WriteStatus(code int) {
	// TODO
	status, ok := statusText[code]
	if !ok {
		panic("Code doesn't exist in HTTP/1.1(RFC2616) protocol")
	}
	resp.StatusCode = code
	resp.Status = status
}
