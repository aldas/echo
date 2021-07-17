package echo

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroup_withoutRouteWillNotExecuteMiddleware(t *testing.T) {
	e := New()

	called := false
	mw := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			called = true
			return c.NoContent(http.StatusTeapot)
		}
	}
	// even though group has middleware it will not be executed when there are no routes under that group
	_ = e.Group("/group", mw)

	status, body := request(http.MethodGet, "/group/nope", e)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, `{"message":"Not Found"}`+"\n", body)

	assert.False(t, called)
}

func TestGroup_withRoutesWillNotExecuteMiddlewareFor404(t *testing.T) {
	e := New()

	called := false
	mw := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			called = true
			return c.NoContent(http.StatusTeapot)
		}
	}
	// even though group has middleware and routes when we have no match on some route the middlewares for that
	// group will not be executed
	g := e.Group("/group", mw)
	err := g.GET("/yes", handlerFunc)
	assert.NoError(t, err)

	status, body := request(http.MethodGet, "/group/nope", e)
	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, `{"message":"Not Found"}`+"\n", body)

	assert.False(t, called)
}

func TestGroup_multiLevelGroup(t *testing.T) {
	e := New()

	api := e.Group("/api")
	users := api.Group("/users")
	assert.NoError(t, users.GET("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodGet, "/api/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroupFile(t *testing.T) {
	e := New()
	g := e.Group("/group")
	g.File("/walle", "_fixture/images/walle.png")
	expectedData, err := ioutil.ReadFile("_fixture/images/walle.png")
	assert.Nil(t, err)
	req := httptest.NewRequest(http.MethodGet, "/group/walle", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, expectedData, rec.Body.Bytes())
}

func TestGroupRouteMiddleware(t *testing.T) {
	// Ensure middleware slices are not re-used
	e := New()
	g := e.Group("/group")
	h := func(Context) error { return nil }
	m1 := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			return next(c)
		}
	}
	m2 := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			return next(c)
		}
	}
	m3 := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			return next(c)
		}
	}
	m4 := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			return c.NoContent(404)
		}
	}
	m5 := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			return c.NoContent(405)
		}
	}
	g.Use(m1, m2, m3)
	g.GET("/404", h, m4)
	g.GET("/405", h, m5)

	c, _ := request(http.MethodGet, "/group/404", e)
	assert.Equal(t, 404, c)
	c, _ = request(http.MethodGet, "/group/405", e)
	assert.Equal(t, 405, c)
}

func TestGroupRouteMiddlewareWithMatchAny(t *testing.T) {
	// Ensure middleware and match any routes do not conflict
	e := New()
	g := e.Group("/group")
	m1 := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			return next(c)
		}
	}
	m2 := func(next HandlerFunc) HandlerFunc {
		return func(c Context) error {
			return c.String(http.StatusOK, c.Path())
		}
	}
	h := func(c Context) error {
		return c.String(http.StatusOK, c.Path())
	}
	g.Use(m1)
	assert.NoError(t, g.GET("/help", h, m2))
	assert.NoError(t, g.GET("/*", h, m2))
	assert.NoError(t, g.GET("", h, m2))
	assert.NoError(t, e.GET("unrelated", h, m2))
	assert.NoError(t, e.GET("*", h, m2))

	_, m := request(http.MethodGet, "/group/help", e)
	assert.Equal(t, "/group/help", m)
	_, m = request(http.MethodGet, "/group/help/other", e)
	assert.Equal(t, "/group/*", m)
	_, m = request(http.MethodGet, "/group/404", e)
	assert.Equal(t, "/group/*", m)
	_, m = request(http.MethodGet, "/group", e)
	assert.Equal(t, "/group", m)
	_, m = request(http.MethodGet, "/other", e)
	assert.Equal(t, "/*", m)
	_, m = request(http.MethodGet, "/", e)
	assert.Equal(t, "/*", m)

}

func TestGroup_CONNECT(t *testing.T) {
	e := New()

	users := e.Group("/users")
	assert.NoError(t, users.CONNECT("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodConnect, "/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroup_DELETE(t *testing.T) {
	e := New()

	users := e.Group("/users")
	assert.NoError(t, users.DELETE("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodDelete, "/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroup_HEAD(t *testing.T) {
	e := New()

	users := e.Group("/users")
	assert.NoError(t, users.HEAD("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodHead, "/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroup_OPTIONS(t *testing.T) {
	e := New()

	users := e.Group("/users")
	assert.NoError(t, users.OPTIONS("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodOptions, "/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroup_PATCH(t *testing.T) {
	e := New()

	users := e.Group("/users")
	assert.NoError(t, users.PATCH("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodPatch, "/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroup_POST(t *testing.T) {
	e := New()

	users := e.Group("/users")
	assert.NoError(t, users.POST("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodPost, "/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroup_PUT(t *testing.T) {
	e := New()

	users := e.Group("/users")
	assert.NoError(t, users.PUT("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodPut, "/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroup_TRACE(t *testing.T) {
	e := New()

	users := e.Group("/users")
	assert.NoError(t, users.TRACE("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	}))

	status, body := request(http.MethodTrace, "/users/activate", e)
	assert.Equal(t, http.StatusTeapot, status)
	assert.Equal(t, `OK`, body)
}

func TestGroup_Any(t *testing.T) {
	e := New()

	users := e.Group("/users")
	errs := users.Any("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	})
	assert.Len(t, errs, 0)

	for _, m := range methods {
		status, body := request(m, "/users/activate", e)
		assert.Equal(t, http.StatusTeapot, status)
		assert.Equal(t, `OK`, body)
	}
}

func TestGroup_AnyWithErrors(t *testing.T) {
	e := New()

	users := e.Group("/users")
	err := users.GET("/activate", func(c Context) error {
		return c.String(http.StatusOK, "OK")
	})
	assert.NoError(t, err)

	errs := users.Any("/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	})
	assert.Len(t, errs, 1)
	assert.EqualError(t, errs[0], "GET /users/activate: adding duplicate route (same method+path) is not allowed")

	for _, m := range methods {
		status, body := request(m, "/users/activate", e)

		expect := http.StatusTeapot
		if m == http.MethodGet {
			expect = http.StatusOK
		}
		assert.Equal(t, expect, status)
		assert.Equal(t, `OK`, body)
	}
}

func TestGroup_Match(t *testing.T) {
	e := New()

	myMethods := []string{http.MethodGet, http.MethodPost}
	users := e.Group("/users")
	errs := users.Match(myMethods, "/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	})
	assert.Len(t, errs, 0)

	for _, m := range myMethods {
		status, body := request(m, "/users/activate", e)
		assert.Equal(t, http.StatusTeapot, status)
		assert.Equal(t, `OK`, body)
	}
}

func TestGroup_MatchWithErrors(t *testing.T) {
	e := New()

	users := e.Group("/users")
	err := users.GET("/activate", func(c Context) error {
		return c.String(http.StatusOK, "OK")
	})
	assert.NoError(t, err)
	myMethods := []string{http.MethodGet, http.MethodPost}

	errs := users.Match(myMethods, "/activate", func(c Context) error {
		return c.String(http.StatusTeapot, "OK")
	})
	assert.Len(t, errs, 1)
	assert.EqualError(t, errs[0], "GET /users/activate: adding duplicate route (same method+path) is not allowed")

	for _, m := range myMethods {
		status, body := request(m, "/users/activate", e)

		expect := http.StatusTeapot
		if m == http.MethodGet {
			expect = http.StatusOK
		}
		assert.Equal(t, expect, status)
		assert.Equal(t, `OK`, body)
	}
}

// TODO: group + .Use(middleware.Static()) mw // e and group level variants. See https://github.com/labstack/echo/issues/838 for usecases
