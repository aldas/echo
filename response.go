// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package echo

import (
	"errors"
	"log/slog"
	"net/http"
)

// Response wraps an http.ResponseWriter and implements its interface to be used
// by an HTTP handler to construct an HTTP response.
// See: https://golang.org/pkg/net/http/#ResponseWriter
type Response struct {
	http.ResponseWriter
	logger *slog.Logger
	// beforeFuncs are functions that are called just before the response (status) is written. Happens only once, during WriteHeader call.
	beforeFuncs []func()
	// afterFuncs are functions that are called just after the response is written. During every `Write` method call.
	afterFuncs []func()
	Status     int
	Size       int64
	Committed  bool
}

// NewResponse creates a new instance of Response.
func NewResponse(w http.ResponseWriter, logger *slog.Logger) (r *Response) {
	return &Response{ResponseWriter: w, logger: logger}
}

// Before registers a function which is called just before the response (status) is written.
func (r *Response) Before(fn func()) {
	r.beforeFuncs = append(r.beforeFuncs, fn)
}

// After registers a function which is called just after the response is written.
func (r *Response) After(fn func()) {
	r.afterFuncs = append(r.afterFuncs, fn)
}

// WriteHeader sends an HTTP response header with status code. If WriteHeader is
// not called explicitly, the first call to Write will trigger an implicit
// WriteHeader(http.StatusOK). Thus explicit calls to WriteHeader are mainly
// used to send error codes.
func (r *Response) WriteHeader(code int) {
	if r.Committed {
		r.logger.Error("echo: response already written to client")
		return
	}
	r.Status = code
	for _, fn := range r.beforeFuncs {
		fn()
	}
	r.ResponseWriter.WriteHeader(r.Status)
	r.Committed = true
}

// Write writes the data to the connection as part of an HTTP reply.
func (r *Response) Write(b []byte) (n int, err error) {
	if !r.Committed {
		if r.Status == 0 {
			r.Status = http.StatusOK
		}
		r.WriteHeader(r.Status)
	}
	n, err = r.ResponseWriter.Write(b)
	r.Size += int64(n)
	for _, fn := range r.afterFuncs {
		fn()
	}
	return
}

// Unwrap returns the original http.ResponseWriter.
// ResponseController can be used to access the original http.ResponseWriter.
// See [https://go.dev/blog/go1.20]
func (r *Response) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *Response) reset(w http.ResponseWriter) {
	r.beforeFuncs = nil
	r.afterFuncs = nil
	r.ResponseWriter = w
	r.Size = 0
	r.Status = http.StatusOK
	r.Committed = false
}

// UnwrapResponse unwraps given ResponseWriter to return contexts original Echo Response. rw has to implement
// following method `Unwrap() http.ResponseWriter`
func UnwrapResponse(rw http.ResponseWriter) (*Response, error) {
	for {
		switch t := rw.(type) {
		case *Response:
			return t, nil
		case interface{ Unwrap() http.ResponseWriter }:
			rw = t.Unwrap()
			continue
		default:
			return nil, errors.New("ResponseWriter does not implement 'Unwrap() http.ResponseWriter' interface")
		}
	}
}
