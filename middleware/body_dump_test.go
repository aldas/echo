package middleware

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestBodyDump(t *testing.T) {
	e := echo.New()
	hw := "Hello, World!"
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(hw))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := func(c echo.Context) error {
		body, err := ioutil.ReadAll(c.Request().Body)
		if err != nil {
			return err
		}
		return c.String(http.StatusOK, string(body))
	}

	requestBody := ""
	responseBody := ""
	mw, err := BodyDumpWithConfig(BodyDumpConfig{Handler: func(c echo.Context, reqBody, resBody []byte) {
		requestBody = string(reqBody)
		responseBody = string(resBody)
	}})
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
	mw, err := BodyDumpWithConfig(BodyDumpConfig{
		Skipper: func(c echo.Context) bool {
			return true
		},
		Handler: func(c echo.Context, reqBody, resBody []byte) {
			isCalled = true
		},
	})
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

	mw, err := BodyDumpWithConfig(BodyDumpConfig{Handler: func(c echo.Context, reqBody, resBody []byte) {}})
	assert.NoError(t, err)

	err = mw(h)(c)
	assert.EqualError(t, err, "some error")
	assert.Equal(t, http.StatusOK, rec.Code)

}

func TestMustBodyDump(t *testing.T) {
	assert.Panics(t, func() {
		mw := MustBodyDumpWithConfig(BodyDumpConfig{
			Skipper: nil,
			Handler: nil,
		})
		assert.NotNil(t, mw)
	})

	assert.NotPanics(t, func() {
		mw := MustBodyDumpWithConfig(BodyDumpConfig{Handler: func(c echo.Context, reqBody, resBody []byte) {}})
		assert.NotNil(t, mw)
	})
}
