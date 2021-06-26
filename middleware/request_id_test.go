package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestRequestID(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	}

	rid := RequestID()
	h := rid(handler)
	err := h(c)
	assert.NoError(t, err)
	assert.Len(t, rec.Header().Get(echo.HeaderXRequestID), 32)
}

func TestMustRequestIDWithConfig_customGenerator(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	}

	rid := MustRequestIDWithConfig(RequestIDConfig{
		Generator: func() string { return "customGenerator" },
	})
	h := rid(handler)
	err := h(c)
	assert.NoError(t, err)
	assert.Equal(t, rec.Header().Get(echo.HeaderXRequestID), "customGenerator")
}

func TestRequestIDWithConfig(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	}

	rid, err := RequestIDWithConfig(RequestIDConfig{})
	assert.NoError(t, err)
	h := rid(handler)
	h(c)
	assert.Len(t, rec.Header().Get(echo.HeaderXRequestID), 32)

	// Custom generator
	rid = MustRequestIDWithConfig(RequestIDConfig{
		Generator: func() string { return "customGenerator" },
	})
	h = rid(handler)
	h(c)
	assert.Equal(t, rec.Header().Get(echo.HeaderXRequestID), "customGenerator")
}

func TestRequestID_IDNotAltered(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add(echo.HeaderXRequestID, "<sample-request-id>")

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "test")
	}

	rid := MustRequestIDWithConfig(RequestIDConfig{})
	h := rid(handler)
	_ = h(c)
	assert.Equal(t, rec.Header().Get(echo.HeaderXRequestID), "<sample-request-id>")
}
