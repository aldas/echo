package echo

import (
	"net/http"
)

type (
	// Group is a set of sub-routes for a specified route. It can be used for inner
	// routes that share a common middleware or functionality that should be separate
	// from the parent echo instance while still inheriting from it.
	Group struct {
		common
		host       string
		prefix     string
		middleware []MiddlewareFunc
		echo       *Echo
	}
)

// Use implements `Echo#Use()` for sub-routes within the Group.
func (g *Group) Use(middleware ...MiddlewareFunc) {
	g.middleware = append(g.middleware, middleware...)
	if len(g.middleware) == 0 {
		return
	}
	// Allow all requests to reach the group as they might get dropped if router
	// doesn't find a match, making none of the group middleware process.
	g.Any("", NotFoundHandler)
	g.Any("/*", NotFoundHandler)
}

// CONNECT implements `Echo#CONNECT()` for sub-routes within the Group.
func (g *Group) CONNECT(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodConnect, path, h, m...)
}

// DELETE implements `Echo#DELETE()` for sub-routes within the Group.
func (g *Group) DELETE(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodDelete, path, h, m...)
}

// GET implements `Echo#GET()` for sub-routes within the Group.
func (g *Group) GET(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodGet, path, h, m...)
}

// HEAD implements `Echo#HEAD()` for sub-routes within the Group.
func (g *Group) HEAD(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodHead, path, h, m...)
}

// OPTIONS implements `Echo#OPTIONS()` for sub-routes within the Group.
func (g *Group) OPTIONS(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodOptions, path, h, m...)
}

// PATCH implements `Echo#PATCH()` for sub-routes within the Group.
func (g *Group) PATCH(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodPatch, path, h, m...)
}

// POST implements `Echo#POST()` for sub-routes within the Group.
func (g *Group) POST(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodPost, path, h, m...)
}

// PUT implements `Echo#PUT()` for sub-routes within the Group.
func (g *Group) PUT(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodPut, path, h, m...)
}

// TRACE implements `Echo#TRACE()` for sub-routes within the Group.
func (g *Group) TRACE(path string, h HandlerFunc, m ...MiddlewareFunc) error {
	return g.Add(http.MethodTrace, path, h, m...)
}

// Any implements `Echo#Any()` for sub-routes within the Group.
func (g *Group) Any(path string, handler HandlerFunc, middleware ...MiddlewareFunc) []error {
	errors := make([]error, 0)
	for _, m := range methods {
		if err := g.Add(m, path, handler, middleware...); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// Match implements `Echo#Match()` for sub-routes within the Group.
func (g *Group) Match(methods []string, path string, handler HandlerFunc, middleware ...MiddlewareFunc) []error {
	errors := make([]error, 0)
	for _, m := range methods {
		if err := g.Add(m, path, handler, middleware...); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// Group creates a new sub-group with prefix and optional sub-group-level middleware.
func (g *Group) Group(prefix string, middleware ...MiddlewareFunc) (sg *Group) {
	m := make([]MiddlewareFunc, 0, len(g.middleware)+len(middleware))
	m = append(m, g.middleware...)
	m = append(m, middleware...)
	sg = g.echo.Group(g.prefix+prefix, m...)
	sg.host = g.host
	return
}

// Static implements `Echo#Static()` for sub-routes within the Group.
func (g *Group) Static(prefix, root string, middleware ...MiddlewareFunc) error {
	return g.static(prefix, root, g.GET, middleware...)
}

// File implements `Echo#File()` for sub-routes within the Group.
func (g *Group) File(path, file string, middleware ...MiddlewareFunc) error {
	return g.file(path, file, g.GET, middleware...)
}

// Add implements `Echo#Add()` for sub-routes within the Group.
func (g *Group) Add(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) error {
	return g.AddRoute(Route{
		Method:      method,
		Path:        path,
		Handler:     handler,
		Middlewares: middleware,
	})
}

// AddRoute registers a new Route with Router
func (g *Group) AddRoute(route Route) error {
	// Combine into a new slice to avoid accidentally passing the same slice for
	// multiple routes, which would lead to later add() calls overwriting the
	// middleware from earlier calls.
	mLen := len(g.middleware) + len(route.Middlewares)
	if mLen > 0 {
		m := make([]MiddlewareFunc, 0, mLen)
		m = append(m, g.middleware...)
		m = append(m, route.Middlewares...)
		route.Middlewares = m
	}
	route.Path = g.prefix + route.Path
	return g.echo.add(g.host, route)
}
