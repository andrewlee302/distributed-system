package http

//
// Http client library.
// Support concurrent and keep-alive http requests.
// Not support: chuck transfer encoding.

import (
	"bufio"
	"container/list"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Client send the http request and recevice response.
//
// Supports concurrency on multiple TCP connections.
type Client struct {
	connSize    int64
	maxConnSize int64

	// Your data here.

	// host(hostname:port) -> TCPConns
	tcpConnPool map[string](*list.List)
	mu          sync.Mutex
	cond        sync.Cond
}

// DefaultMaxConnSize is the default max size of
// active tcp connections.
const DefaultMaxConnSize = 500

// NewClient initilize a Client with DefaultMaxConnSize.
func NewClient() *Client {
	return NewClientSize(DefaultMaxConnSize)
}

// NewClientSize initilize a Client with a specific maxConnSize.
func NewClientSize(maxConnSize int64) *Client {
	c := &Client{maxConnSize: maxConnSize}

	// Your initialization code here.
	c.tcpConnPool = make(map[string](*list.List))
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
// not closed, the Client may not be able to reuse a keep-alive TCP
// connection to the same server.
func (c *Client) Send(req *Request) (resp *Response, err error) {
	if req.URL == nil {
		return nil, errors.New("http: nil Request.URL")
	}

	// Get a available connection to the host for HTTP communication.
	tc, err := c.getConn(req.URL.Host)
	if err != nil {
		return nil, err
	}

	// Write the request to the TCP stream.
	err = c.writeReq(tc, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		c.cleanConn(tc, req)
		return nil, err
	}

	// Construct the response from the TCP stream.
	resp, err = c.constructResp(tc, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		c.cleanConn(tc, req)
	}
	return
}

// Put back the available connection of the specific host
// for the future use.
func (c *Client) putConn(tc *net.TCPConn, host string) {
	// TODO
	c.mu.Lock()
	defer c.mu.Unlock()
	tcpConns, ok := c.tcpConnPool[host]
	if ok {
		tcpConns.PushBack(tc)
		c.cond.Broadcast()
	} else {
		panic("not here")
	}
}

// Get a TCP connection to the host.
func (c *Client) getConn(host string) (tc *net.TCPConn, err error) {
	// TODO
	c.mu.Lock()
	tcpConns, ok := c.tcpConnPool[host]
	if !ok {
		tcpConns = list.New()
		c.tcpConnPool[host] = tcpConns
	}

	for tcpConns.Front() == nil && atomic.LoadInt64(&c.connSize) >= c.maxConnSize {
		c.cond.Wait()
	}

	if tcpConns.Front() == nil {
		// *** The following code should be here instead of the end of the block.
		atomic.AddInt64(&c.connSize, 1)
		c.mu.Unlock()
		// New tcp connection
		tcpAddr, err := net.ResolveTCPAddr("tcp", host)
		if err != nil {
			return nil, err
		}
		tc, err = net.DialTCP("tcp", nil, tcpAddr)
		if err != nil {
			return nil, err
		}

	} else {
		// Reuse
		if ele := tcpConns.Front(); ele != nil {
			tc = ele.Value.(*net.TCPConn)
			tcpConns.Remove(ele)
			c.cond.Broadcast()
			c.mu.Unlock()
		} else {
			panic("not here")
		}

	}
	return tc, err
}

// Clean one connection in the case of errors.
func (c *Client) cleanConn(tc *net.TCPConn, req *Request) {
	// TODO
	tc.Close()
	atomic.AddInt64(&c.connSize, -1)
	c.cond.Broadcast()
}

// Write the request to TCP stream.
//
// The number of bytes in transmit body of a request must be more
// than the value of Content-Length header. If not, throws an error.
func (c *Client) writeReq(tcpConn *net.TCPConn, req *Request) (err error) {
	// TODO
	writer := bufio.NewWriterSize(tcpConn, ClientRequestBufSize)
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
func (c *Client) constructResp(tcpConn *net.TCPConn, req *Request) (*Response, error) {
	// TODO
	// Receive and prase repsonse message
	resp := &Response{Header: make(map[string]string)}
	reader := bufio.NewReaderSize(tcpConn, ClientResponseBufSize)
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
						// fmt.Println(statusLineWords)
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
								tc:   tcpConn,
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
