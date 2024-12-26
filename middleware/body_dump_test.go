// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
)

func TestBodyDump(t *testing.T) {
	e := echo.New()
	hw := "Hello, World!"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(hw))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := func(c echo.Context) error {
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	}

	requestBody := ""
	responseBody := ""
	mw, err := BodyDumpConfig{Handler: func(c echo.Context, reqBody, resBody []byte) {
		requestBody = string(reqBody)
		responseBody = string(resBody)
	}}.ToMiddleware()
	assert.NoError(t, err)

	if assert.NoError(t, mw(h)(c)) {
		assert.Equal(t, requestBody, hw)
		assert.Equal(t, responseBody, hw)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, hw, rec.Body.String())
	}

}

func TestBodyDump_skipper(t *testing.T) {
	e := echo.New()

	isCalled := false
	mw, err := BodyDumpConfig{
		Skipper: func(c echo.Context) bool {
			return true
		},
		Handler: func(c echo.Context, reqBody, resBody []byte) {
			isCalled = true
		},
	}.ToMiddleware()
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := func(c echo.Context) error {
		return errors.New("some error")
	}

	err = mw(h)(c)
	assert.EqualError(t, err, "some error")
	assert.False(t, isCalled)
}

func TestBodyDump_fails(t *testing.T) {
	e := echo.New()
	hw := "Hello, World!"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(hw))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := func(c echo.Context) error {
		return errors.New("some error")
	}

	mw, err := BodyDumpConfig{Handler: func(c echo.Context, reqBody, resBody []byte) {}}.ToMiddleware()
	assert.NoError(t, err)

	err = mw(h)(c)
	assert.EqualError(t, err, "some error")
	assert.Equal(t, http.StatusOK, rec.Code)

}

func TestBodyDumpWithConfig_panic(t *testing.T) {
	assert.Panics(t, func() {
		mw := BodyDumpWithConfig(BodyDumpConfig{
			Skipper: nil,
			Handler: nil,
		})
		assert.NotNil(t, mw)
	})

	assert.NotPanics(t, func() {
		mw := BodyDumpWithConfig(BodyDumpConfig{Handler: func(c echo.Context, reqBody, resBody []byte) {}})
		assert.NotNil(t, mw)
	})
}

func TestBodyDump_panic(t *testing.T) {
	assert.Panics(t, func() {
		mw := BodyDump(nil)
		assert.NotNil(t, mw)
	})

	assert.NotPanics(t, func() {
		BodyDump(func(c echo.Context, reqBody, resBody []byte) {})
	})
}

func TestBodyDumpResponseWriter_CanNotFlush(t *testing.T) {
	bdrw := bodyDumpResponseWriter{
		ResponseWriter: new(testResponseWriterNoFlushHijack), // this RW does not support flush
	}
	assert.PanicsWithError(t, "response writer flushing is not supported", func() {
		bdrw.Flush()
	})
}

func TestBodyDumpResponseWriter_CanFlush(t *testing.T) {
	trwu := testResponseWriterUnwrapperHijack{testResponseWriterUnwrapper: testResponseWriterUnwrapper{rw: httptest.NewRecorder()}}
	bdrw := bodyDumpResponseWriter{
		ResponseWriter: &trwu,
	}
	bdrw.Flush()
	assert.Equal(t, 1, trwu.unwrapCalled)
}

func TestBodyDumpResponseWriter_CanUnwrap(t *testing.T) {
	trwu := &testResponseWriterUnwrapper{rw: httptest.NewRecorder()}
	bdrw := bodyDumpResponseWriter{
		ResponseWriter: trwu,
	}
	result := bdrw.Unwrap()
	assert.Equal(t, trwu, result)
}

func TestBodyDumpResponseWriter_CanHijack(t *testing.T) {
	trwu := testResponseWriterUnwrapperHijack{testResponseWriterUnwrapper: testResponseWriterUnwrapper{rw: httptest.NewRecorder()}}
	bdrw := bodyDumpResponseWriter{
		ResponseWriter: &trwu, // this RW supports hijacking through unwrapping
	}
	_, _, err := bdrw.Hijack()
	assert.EqualError(t, err, "can hijack")
}

func TestBodyDumpResponseWriter_CanNotHijack(t *testing.T) {
	trwu := testResponseWriterUnwrapper{rw: httptest.NewRecorder()}
	bdrw := bodyDumpResponseWriter{
		ResponseWriter: &trwu, // this RW supports hijacking through unwrapping
	}
	_, _, err := bdrw.Hijack()
	assert.EqualError(t, err, "feature not supported")
}
