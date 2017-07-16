package http

import (
	"io"
)

// RFC2616 Method
const (
	MethodGet     = "GET"
	MethodHead    = "HEAD"
	MethodPost    = "POST"
	MethodPut     = "PUT"
	MethodDelete  = "DELETE"
	MethodConnect = "CONNECT"
	MethodOptions = "OPTIONS"
	MethodTrace   = "TRACE"
)

// RFC2616 Status Code
const (
	StatusContinue           = 100
	StatusSwitchingProtocols = 101

	StatusOK                   = 200
	StatusCreated              = 201
	StatusAccepted             = 202
	StatusNonAuthoritativeInfo = 203
	StatusNoContent            = 204
	StatusResetContent         = 205
	StatusPartialContent       = 206

	StatusMultipleChoices  = 300
	StatusMovedPermanently = 301
	StatusFound            = 302
	StatusSeeOther         = 303
	StatusNotModified      = 304
	StatusUseProxy         = 305

	StatusTemporaryRedirect = 307

	StatusBadRequest                   = 400
	StatusUnauthorized                 = 401
	StatusPaymentRequired              = 402
	StatusForbidden                    = 403
	StatusNotFound                     = 404
	StatusMethodNotAllowed             = 405
	StatusNotAcceptable                = 406
	StatusProxyAuthRequired            = 407
	StatusRequestTimeout               = 408
	StatusConflict                     = 409
	StatusGone                         = 410
	StatusLengthRequired               = 411
	StatusPreconditionFailed           = 412
	StatusRequestEntityTooLarge        = 413
	StatusRequestURITooLong            = 414
	StatusUnsupportedMediaType         = 415
	StatusRequestedRangeNotSatisfiable = 416
	StatusExpectationFailed            = 417

	StatusInternalServerError     = 500
	StatusNotImplemented          = 501
	StatusBadGateway              = 502
	StatusServiceUnavailable      = 503
	StatusGatewayTimeout          = 504
	StatusHTTPVersionNotSupported = 505
)

type Request struct {
	Method string
	URL    string
	Proto  string
	Header Header
	Body   io.Reader
}

type Response struct {
	Status     string
	StatusCode int
	Proto      string
	Header     Header
	Body       io.Reader
}

// !Note: the type of the value of one header entry is
// []string, which could consist of more than one string.
type Header map[string][]string

// The following methods (Add, Del, Get, Set) are the
// same as the golang builtin methods.
// Refer to https://golang.org/pkg/net/http/#Header.
func (h Header) Add(key, value string) {
	// TODO
}

func (h Header) Del(key string) {
	// TODO
}

func (h Header) Get(key string) string {
	// TODO
	return ""
}

func (h Header) Set(key, value string) {
	// TODO
}
