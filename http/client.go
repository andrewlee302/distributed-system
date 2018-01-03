package http

//
// Http client library.
// Support concurrent and keep-alive http requests.
// Not support: chuck transfer encoding.

import (
	"bufio"
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

// Client send the http request and recevice response.
//
// Supports concurrency.
type Client struct {
	tcpConns map[string]*net.TCPConn // host(hostname:port) -> TCPConn
	mu       sync.Mutex
}

// NewClient initilize a Client
func NewClient() *Client {
	return &Client{tcpConns: make(map[string]*net.TCPConn)}
}

// Get implements GET Method of HTTP/1.1.
//
// Must set the Content-Length, Host header.
func (c *Client) Get(URL string) (resp *Response, err error) {
	// TODO
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
// Must set the Content-Length, Host header and body.
//
// Write the contentLength bytes data into the body of HTTP request.
// Discard the sequential data after the reading contentLength bytes
// from body(io.Reader).
func (c *Client) Post(URL string, contentLength int64, body io.Reader) (resp *Response, err error) {
	// TODO
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
// An error is returned if caused by client policy (such as invalid HTTP response),
// or failure to speak HTTP (such as a network connectivity problem).
// A non-2xx status code doesn't cause an error.
//
// If the returned error is nil, the Response will contain a non-nil
// Body which the user is expected to close. If the Body is not
// closed, the Client's underlying Transport may not be able to re-use
// a persistent TCP connection to the server for a subsequent "keep-alive" request.
func (c *Client) Send(req *Request) (resp *Response, err error) {

	// TODO
	if req.URL == nil {
		return nil, errors.New("http: nil Request.URL")
	}
	// reuse logics
	c.mu.Lock()
	tcpConn, ok := c.tcpConns[req.URL.Host]
	if !ok {
		tcpAddr, err := net.ResolveTCPAddr("tcp", req.URL.Host)
		if err != nil {
			return nil, err
		}
		tcpConn, err = net.DialTCP("tcp", nil, tcpAddr)
		if err != nil {
			return nil, err
		}
		c.tcpConns[req.URL.Host] = tcpConn
	}
	c.mu.Unlock()

	err = writeReq(tcpConn, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		c.cleanConn(tcpConn, req)
		return nil, err
	}
	resp, err = readResp(tcpConn, req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		c.cleanConn(tcpConn, req)
	}
	return
}

func (c *Client) cleanConn(tcpConn *net.TCPConn, req *Request) {
	tcpConn.Close()
	delete(c.tcpConns, req.URL.Host)
}

// Send the request to tcp stream.
//
// The number of transmit body must be the same as the specific
// Content-Length. So the quantity of the available data in body is
// at least Content-Length. If not, throws an error.
func writeReq(tcpConn *net.TCPConn, req *Request) (err error) {
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

// ResponseReader delimit the responses in tcp stream. Read will
// return io.EOF if users digest data of contentLength bytes for
// the current response.

// err is not nil if tcp conn occurs, of course req is nil.
func readResp(tcpConn *net.TCPConn, req *Request) (*Response, error) {

	// receive and prase repsonse message
	resp := &Response{Header: make(map[string]string)}
	reader := bufio.NewReaderSize(tcpConn, ClientResponseBufSize)
	var wholeLine []byte
	var lastWait = false
	var step = ResponseStepStatusLine
LOOP:
	for {
		if line, isWait, err := reader.ReadLine(); err == nil {
			if !isWait {
				// complete line
				if !lastWait {
					wholeLine = line
				} else {
					wholeLine = append(wholeLine, line...)
				}
				// process the line
				switch step {
				case ResponseStepStatusLine:
					{
						statusLineWords := strings.SplitN(string(wholeLine), " ", 3)
						fmt.Println(statusLineWords)
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
							fmt.Println(resp.Header)
							step = ResponseStepBody
							cLenStr, ok := resp.Header[HeaderContentLength]
							if !ok {
								return nil, errors.New("No Content-Length in Response header")
							}
							cLen, _ := strconv.ParseInt(cLenStr, 10, 64)
							resp.ContentLength = cLen

							// transfer the body to Response
							resp.Body = &io.LimitedReader{
								R: reader,
								N: resp.ContentLength,
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
				// not complete
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
