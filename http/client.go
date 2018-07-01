package http

// Http client library.

import (
	"bufio"
	"distributed-system/util"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Client sends the http request and recevice response. Supports concurrency
// on multiple connections.
type Client struct {
	// Your data here.

	// tcp or unix
	Network string
	// map: server host -> connection pool
	connPools *util.ResourcePoolsMap
	mu        sync.Mutex
	cond      sync.Cond
}

// DefaultMaxConnSizeForOne is the default max size of active connections for one
// host.
const DefaultMaxConnSizeForOne = 500

// NewClient initilize a Client with DefaultMaxConnSizeForOne.
func NewClient(network string) *Client {
	return NewClientSize(network, DefaultMaxConnSizeForOne)
}

// NewClientSize initilize a Client with a specific maxConnSize.
func NewClientSize(network string, maxConnSizeForOne int) *Client {
	if network != "unix" && network != "tcp" {
		return nil
	}
	c := &Client{Network: network}

	// Your initialization code here.
	c.connPools = util.NewResourcePoolsMap(
		func(host string) func() util.Resource {
			return func() util.Resource {
				if network == "tcp" {
					conn, err := net.Dial("tcp", host)
					if err != nil {
						return nil
					}
					return conn
				}
				// network == "unix"
				socketFile := UnixSocketFile(host)
				conn, err := net.Dial("unix", socketFile)
				if err != nil {
					return nil
				}
				return conn
			}

		},
		maxConnSizeForOne)
	c.cond = sync.Cond{L: &c.mu}
	return c
}

// Get implements GET Method of HTTP/1.1.
//
// Must set the body and following headers in the request:
// * Content-Length
// * Host
func (c *Client) Get(URL string) (resp *Response, err error) {
	urlObj, err := url.ParseRequestURI(URL)
	if err != nil {
		return
	}
	header := make(map[string]string)
	header[HeaderContentLength] = "0"
	header[HeaderHost] = urlObj.Host
	req := &Request{
		Method:        MethodGet,
		URL:           urlObj,
		Proto:         HTTPVersion,
		Header:        header,
		ContentLength: 0,
		Body:          strings.NewReader(""),
	}
	resp, err = c.Send(req)
	return
}

// Post implements POST Method of HTTP/1.1.
//
// Must set the body and following headers in the request:
// * Content-Length
// * Host
//
// Write the contentLength bytes data into the body of HTTP request.
// Discard the sequential data after the reading contentLength bytes
// from body(io.Reader).
func (c *Client) Post(URL string, contentLength int64, body io.Reader) (resp *Response, err error) {
	urlObj, err := url.ParseRequestURI(URL)
	if err != nil {
		return
	}
	header := make(map[string]string)
	header[HeaderContentLength] = strconv.FormatInt(contentLength, 10)
	header[HeaderHost] = urlObj.Host
	req := &Request{
		Method:        MethodPost,
		URL:           urlObj,
		Proto:         HTTPVersion,
		Header:        header,
		ContentLength: contentLength,
		Body:          body,
	}
	resp, err = c.Send(req)
	return
}

// Step flags for response stream processing.
const (
	ResponseStepStatusLine = iota
	ResponseStepHeader
	ResponseStepBody
)

// Send http request and returns an HTTP response.
//
// An error is returned if caused by client policy (such as invalid
// HTTP response), or failure to speak HTTP (such as a network
// connectivity problem).
//
// Note that a non-2xx status code doesn't mean any above errors.
//
// If the returned error is nil, the Response will contain a non-nil
// Body which is the caller's responsibility to close. If the Body is
// not closed, the Client may not be able to reuse a keep-alive
// connection to the same server.
func (c *Client) Send(req *Request) (resp *Response, err error) {
	if req.URL == nil {
		return nil, errors.New("http: nil Request.URL")
	}

	// Get a available connection to the host for HTTP communication.
	conn := c.connPools.Get(req.URL.Host).(io.ReadWriteCloser)

	// Write the request to the connection stream.
	err = c.writeReq(conn, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		c.connPools.Clean(req.URL.Host, conn)
		return nil, err
	}

	// Construct the response from the connection stream.
	resp, err = c.constructResp(conn, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		c.connPools.Clean(req.URL.Host, conn)
		return nil, err
	}
	return
}

// Write the request to TCP stream.
//
// The number of bytes in transmit body of a request must be more
// than the value of Content-Length header. If not, throws an error.
func (c *Client) writeReq(conn io.Writer, req *Request) (err error) {
	// TODO
	writer := bufio.NewWriterSize(conn, ClientRequestBufSize)
	reqLine := fmt.Sprintf("%s %s %s\n", req.Method, req.URL.Path, req.Proto)
	_, err = writer.WriteString(reqLine)
	if err != nil {
		return err
	}
	for key, value := range req.Header {
		_, err = writer.WriteString(fmt.Sprintf("%s: %s\n", key, value))
		if err != nil {
			return err
		}
	}
	err = writer.WriteByte('\n')
	if err != nil {
		return err
	}
	switch req.Method {
	case MethodPost:
		{
			if _, err = io.CopyN(writer, req.Body, req.ContentLength); err != nil {
				return err
			}

		}
	case MethodGet:
		// pass
	}
	err = writer.Flush()
	return
}

// Construct response from the TCP stream.
//
// Body of the response will return io.EOF if reading
// Content-Length bytes.
//
// If TCP errors occur, err is not nil and req is nil.
func (c *Client) constructResp(conn io.Reader, req *Request) (*Response, error) {
	// TODO
	// Receive and prase repsonse message
	resp := &Response{Header: make(map[string]string)}
	reader := bufio.NewReaderSize(conn, ClientResponseBufSize)
	var wholeLine []byte
	var lastWait = false
	var step = ResponseStepStatusLine
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
				case ResponseStepStatusLine:
					{
						statusLineWords := strings.SplitN(string(wholeLine), " ", 3)
						resp.Proto = statusLineWords[0]
						resp.StatusCode, err = strconv.Atoi(statusLineWords[1])
						resp.Status = statusLineWords[2]
						step = ResponseStepHeader
					}
				case ResponseStepHeader:
					{
						if len(line) != 0 {
							headerWords := strings.SplitN(string(wholeLine), ": ", 2)
							resp.Header[headerWords[0]] = headerWords[1]

						} else {
							// fmt.Println(resp.Header)
							step = ResponseStepBody
							cLenStr, ok := resp.Header[HeaderContentLength]
							if !ok {
								return nil, errors.New("No Content-Length in Response header")
							}
							cLen, _ := strconv.ParseInt(cLenStr, 10, 64)
							resp.ContentLength = cLen

							// Transfer the body to Response
							resp.Body = &ResponseReader{
								c:    c,
								conn: conn,
								host: req.URL.Host,
								r: &io.LimitedReader{
									R: reader,
									N: resp.ContentLength,
								},
							}
							break LOOP
						}
					}
				case ResponseStepBody:
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
			break
		} else {
			return nil, err
		}
	}
	return resp, nil
}
