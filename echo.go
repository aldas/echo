// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

/*
Package echo implements high performance, minimalist Go web framework.

Example:

	  package main

		import (
			"github.com/labstack/echo/v5"
			"github.com/labstack/echo/v5/middleware"
			"log"
			"net/http"
		)

	  // Handler
	  func hello(c *echo.Context) error {
	    return c.String(http.StatusOK, "Hello, World!")
	  }

	  func main() {
	    // Echo instance
	    e := echo.New()

	    // Middleware
	    e.Use(middleware.RequestLogger())
	    e.Use(middleware.Recover())

	    // Routes
	    e.GET("/", hello)

	    // Start server
	    if err := e.Start(":8080"); err != http.ErrServerClosed {
			  log.Fatal(err)
		  }
	  }

Learn more at https://echo.labstack.com
*/
package echo

import (
	stdContext "context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// Echo is the top-level framework instance.
//
// Goroutine safety: Do not mutate Echo instance fields after server has started. Accessing these
// fields from handlers/middlewares and changing field values at the same time leads to data-races.
// Same rule applies to adding new routes after server has been started - Adding a route is not Goroutine safe action.
type Echo struct {
	serveHTTPFunc func(http.ResponseWriter, *http.Request)

	Binder           Binder
	Filesystem       fs.FS
	Renderer         Renderer
	Validator        Validator
	JSONSerializer   JSONSerializer
	IPExtractor      IPExtractor
	OnAddRoute       func(route Route) error
	HTTPErrorHandler HTTPErrorHandler
	Logger           *slog.Logger

	contextPool sync.Pool

	router Router

	// premiddleware are middlewares that are called before routing is done
	premiddleware []MiddlewareFunc

	// middleware are middlewares that are called after routing is done and before handler is called
	middleware []MiddlewareFunc

	contextPathParamAllocSize atomic.Int32
}

// JSONSerializer is the interface that encodes and decodes JSON to and from interfaces.
type JSONSerializer interface {
	Serialize(c *Context, target any, indent string) error
	Deserialize(c *Context, target any) error
}

// HTTPErrorHandler is a centralized HTTP error handler.
type HTTPErrorHandler func(c *Context, err error)

// HandlerFunc defines a function to serve HTTP requests.
type HandlerFunc func(c *Context) error

// MiddlewareFunc defines a function to process middleware.
type MiddlewareFunc func(next HandlerFunc) HandlerFunc

// MiddlewareConfigurator defines interface for creating middleware handlers with possibility to return configuration errors instead of panicking.
type MiddlewareConfigurator interface {
	ToMiddleware() (MiddlewareFunc, error)
}

// Validator is the interface that wraps the Validate function.
type Validator interface {
	Validate(i any) error
}

// Map defines a generic map of type `map[string]any`.
type Map map[string]any

// MIME types
const (
	// MIMEApplicationJSON JavaScript Object Notation (JSON) https://www.rfc-editor.org/rfc/rfc8259
	MIMEApplicationJSON = "application/json"
	// Deprecated: Please use MIMEApplicationJSON instead. JSON should be encoded using UTF-8 by default.
	// No "charset" parameter is defined for this registration.
	// Adding one really has no effect on compliant recipients.
	// See RFC 8259, section 8.1. https://datatracker.ietf.org/doc/html/rfc8259#section-8.1n"
	MIMEApplicationJSONCharsetUTF8       = MIMEApplicationJSON + "; " + charsetUTF8
	MIMEApplicationJavaScript            = "application/javascript"
	MIMEApplicationJavaScriptCharsetUTF8 = MIMEApplicationJavaScript + "; " + charsetUTF8
	MIMEApplicationXML                   = "application/xml"
	MIMEApplicationXMLCharsetUTF8        = MIMEApplicationXML + "; " + charsetUTF8
	MIMETextXML                          = "text/xml"
	MIMETextXMLCharsetUTF8               = MIMETextXML + "; " + charsetUTF8
	MIMEApplicationForm                  = "application/x-www-form-urlencoded"
	MIMEApplicationProtobuf              = "application/protobuf"
	MIMEApplicationMsgpack               = "application/msgpack"
	MIMETextHTML                         = "text/html"
	MIMETextHTMLCharsetUTF8              = MIMETextHTML + "; " + charsetUTF8
	MIMETextPlain                        = "text/plain"
	MIMETextPlainCharsetUTF8             = MIMETextPlain + "; " + charsetUTF8
	MIMEMultipartForm                    = "multipart/form-data"
	MIMEOctetStream                      = "application/octet-stream"
)

const (
	charsetUTF8 = "charset=UTF-8"
	// PROPFIND Method can be used on collection and property resources.
	PROPFIND = "PROPFIND"
	// REPORT Method can be used to get information about a resource, see rfc 3253
	REPORT = "REPORT"
	// RouteNotFound is special method type for routes handling "route not found" (404) cases
	RouteNotFound = "echo_route_not_found"
)

// Headers
const (
	HeaderAccept         = "Accept"
	HeaderAcceptEncoding = "Accept-Encoding"
	// HeaderAllow is the name of the "Allow" header field used to list the set of methods
	// advertised as supported by the target resource. Returning an Allow header is mandatory
	// for status 405 (method not found) and useful for the OPTIONS method in responses.
	// See RFC 7231: https://datatracker.ietf.org/doc/html/rfc7231#section-7.4.1
	HeaderAllow               = "Allow"
	HeaderAuthorization       = "Authorization"
	HeaderContentDisposition  = "Content-Disposition"
	HeaderContentEncoding     = "Content-Encoding"
	HeaderContentLength       = "Content-Length"
	HeaderContentType         = "Content-Type"
	HeaderCookie              = "Cookie"
	HeaderSetCookie           = "Set-Cookie"
	HeaderIfModifiedSince     = "If-Modified-Since"
	HeaderLastModified        = "Last-Modified"
	HeaderLocation            = "Location"
	HeaderRetryAfter          = "Retry-After"
	HeaderUpgrade             = "Upgrade"
	HeaderVary                = "Vary"
	HeaderWWWAuthenticate     = "WWW-Authenticate"
	HeaderXForwardedFor       = "X-Forwarded-For"
	HeaderXForwardedProto     = "X-Forwarded-Proto"
	HeaderXForwardedProtocol  = "X-Forwarded-Protocol"
	HeaderXForwardedSsl       = "X-Forwarded-Ssl"
	HeaderXUrlScheme          = "X-Url-Scheme"
	HeaderXHTTPMethodOverride = "X-HTTP-Method-Override"
	HeaderXRealIP             = "X-Real-Ip"
	HeaderXRequestID          = "X-Request-Id"
	HeaderXCorrelationID      = "X-Correlation-Id"
	HeaderXRequestedWith      = "X-Requested-With"
	HeaderServer              = "Server"
	HeaderOrigin              = "Origin"
	HeaderCacheControl        = "Cache-Control"
	HeaderConnection          = "Connection"

	// Access control
	HeaderAccessControlRequestMethod    = "Access-Control-Request-Method"
	HeaderAccessControlRequestHeaders   = "Access-Control-Request-Headers"
	HeaderAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	HeaderAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	HeaderAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	HeaderAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	HeaderAccessControlExposeHeaders    = "Access-Control-Expose-Headers"
	HeaderAccessControlMaxAge           = "Access-Control-Max-Age"

	// Security
	HeaderStrictTransportSecurity         = "Strict-Transport-Security"
	HeaderXContentTypeOptions             = "X-Content-Type-Options"
	HeaderXXSSProtection                  = "X-XSS-Protection"
	HeaderXFrameOptions                   = "X-Frame-Options"
	HeaderContentSecurityPolicy           = "Content-Security-Policy"
	HeaderContentSecurityPolicyReportOnly = "Content-Security-Policy-Report-Only"
	HeaderXCSRFToken                      = "X-CSRF-Token" // #nosec G101
	HeaderReferrerPolicy                  = "Referrer-Policy"
)

var methods = [...]string{ // TODO refactor: maybe `routeMethod` could have `any` as method.
	http.MethodConnect,
	http.MethodDelete,
	http.MethodGet,
	http.MethodHead,
	http.MethodOptions,
	http.MethodPatch,
	http.MethodPost,
	PROPFIND,
	http.MethodPut,
	http.MethodTrace,
	REPORT,
}

// New creates an instance of Echo.
func New() *Echo {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	e := &Echo{
		Logger:         logger,
		Filesystem:     newDefaultFS(),
		Binder:         &DefaultBinder{},
		JSONSerializer: &DefaultJSONSerializer{},
	}

	e.serveHTTPFunc = e.serveHTTP
	e.router = NewRouter(RouterConfig{})
	e.HTTPErrorHandler = DefaultHTTPErrorHandler(false)
	e.contextPool.New = func() any {
		return e.NewContext(nil, nil)
	}
	return e
}

// NewContext returns a new Context instance.
//
// Note: both request and response can be left to nil as Echo.ServeHTTP will call c.Reset(req,resp) anyway
// these arguments are useful when creating context for tests and cases like that.
func (e *Echo) NewContext(r *http.Request, w http.ResponseWriter) *Context {
	p := make(PathValues, e.contextPathParamAllocSize.Load())
	c := &Context{
		pathValues: &p,
		store:      make(Map),
		echo:       e,
		logger:     e.Logger,
	}
	c.SetRequest(r)
	c.orgResponse = NewResponse(w, e.Logger)
	c.response = c.orgResponse
	return c
}

// Router returns the default router.
func (e *Echo) Router() Router {
	return e.router
}

// DefaultHTTPErrorHandler creates new default HTTP error handler implementation. It sends a JSON response
// with status code. `exposeError` parameter decides if returned message will contain also error message or not
//
// Note: DefaultHTTPErrorHandler does not log errors. Use middleware for it if errors need to be logged (separately)
// Note: In case errors happens in middleware call-chain that is returning from handler (which did not return an error).
// When handler has already sent response (ala c.JSON()) and there is error in middleware that is returning from
// handler. Then the error that global error handler received will be ignored because we have already "committed" the
// response and status code header has been sent to the client.
func DefaultHTTPErrorHandler(exposeError bool) HTTPErrorHandler {
	return func(c *Context, err error) {
		if r, _ := UnwrapResponse(c.response); r != nil && r.Committed {
			return
		}

		// in-case we have errors wrapping each other first HTTPError we see in this chain, will be the one used as
		// error we send to the client
		var he *HTTPError
		if !errors.As(err, &he) {
			he = &HTTPError{
				Code:    http.StatusInternalServerError,
				Message: http.StatusText(http.StatusInternalServerError),
			}
		}

		// Issue #1426
		code := he.Code
		message := he.Message
		switch m := he.Message.(type) {
		case string:
			if exposeError {
				message = Map{"message": m, "error": err.Error()}
			} else {
				message = Map{"message": m}
			}
		case json.Marshaler:
			// do nothing - this type knows how to format itself to JSON
		case error:
			message = Map{"message": m.Error()}
		}

		// Send response
		var cErr error
		if c.Request().Method == http.MethodHead { // Issue #608
			cErr = c.NoContent(he.Code)
		} else {
			cErr = c.JSON(code, message)
		}
		if cErr != nil {
			c.Logger().Error("echo default error handler failed to send error to client", "error", cErr) // truly rare case. ala client already disconnected
		}
	}
}

// Pre adds middleware to the chain which is run before router tries to find matching route.
// Meaning middleware is executed even for 404 (not found) cases.
func (e *Echo) Pre(middleware ...MiddlewareFunc) {
	e.premiddleware = append(e.premiddleware, middleware...)
}

// Use adds middleware to the chain which is run after router has found matching route and before route/request handler method is executed.
func (e *Echo) Use(middleware ...MiddlewareFunc) {
	e.middleware = append(e.middleware, middleware...)
}

// CONNECT registers a new CONNECT route for a path with matching handler in the
// router with optional route-level middleware. Panics on error.
func (e *Echo) CONNECT(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodConnect, path, h, m...)
}

// DELETE registers a new DELETE route for a path with matching handler in the router
// with optional route-level middleware. Panics on error.
func (e *Echo) DELETE(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodDelete, path, h, m...)
}

// GET registers a new GET route for a path with matching handler in the router
// with optional route-level middleware. Panics on error.
func (e *Echo) GET(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodGet, path, h, m...)
}

// HEAD registers a new HEAD route for a path with matching handler in the
// router with optional route-level middleware. Panics on error.
func (e *Echo) HEAD(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodHead, path, h, m...)
}

// OPTIONS registers a new OPTIONS route for a path with matching handler in the
// router with optional route-level middleware. Panics on error.
func (e *Echo) OPTIONS(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodOptions, path, h, m...)
}

// PATCH registers a new PATCH route for a path with matching handler in the
// router with optional route-level middleware. Panics on error.
func (e *Echo) PATCH(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodPatch, path, h, m...)
}

// POST registers a new POST route for a path with matching handler in the
// router with optional route-level middleware. Panics on error.
func (e *Echo) POST(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodPost, path, h, m...)
}

// PUT registers a new PUT route for a path with matching handler in the
// router with optional route-level middleware. Panics on error.
func (e *Echo) PUT(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodPut, path, h, m...)
}

// TRACE registers a new TRACE route for a path with matching handler in the
// router with optional route-level middleware. Panics on error.
func (e *Echo) TRACE(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(http.MethodTrace, path, h, m...)
}

// RouteNotFound registers a special-case route which is executed when no other route is found (i.e. HTTP 404 cases)
// for current request URL.
// Path supports static and named/any parameters just like other http method is defined. Generally path is ended with
// wildcard/match-any character (`/*`, `/download/*` etc).
//
// Example: `e.RouteNotFound("/*", func(c *echo.Context) error { return c.NoContent(http.StatusNotFound) })`
func (e *Echo) RouteNotFound(path string, h HandlerFunc, m ...MiddlewareFunc) RouteInfo {
	return e.Add(RouteNotFound, path, h, m...)
}

// Any registers a new route for all HTTP methods (supported by Echo) and path with matching handler
// in the router with optional route-level middleware.
//
// Note: this method only adds specific set of supported HTTP methods as handler and is not true
// "catch-any-arbitrary-method" way of matching requests.
func (e *Echo) Any(path string, handler HandlerFunc, middleware ...MiddlewareFunc) Routes {
	errs := make([]error, 0)
	ris := make(Routes, 0)
	for _, m := range methods { // could we just register RouteNotFound here?
		ri, err := e.AddRoute(Route{
			Method:      m,
			Path:        path,
			Handler:     handler,
			Middlewares: middleware,
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ris = append(ris, ri)
	}
	if len(errs) > 0 {
		panic(errs) // this is how `v4` handles errors. `v5` has methods to have panic-free usage
	}
	return ris
}

// Match registers a new route for multiple HTTP methods and path with matching
// handler in the router with optional route-level middleware. Panics on error.
func (e *Echo) Match(methods []string, path string, handler HandlerFunc, middleware ...MiddlewareFunc) Routes {
	errs := make([]error, 0)
	ris := make(Routes, 0)
	for _, m := range methods {
		ri, err := e.AddRoute(Route{
			Method:      m,
			Path:        path,
			Handler:     handler,
			Middlewares: middleware,
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		ris = append(ris, ri)
	}
	if len(errs) > 0 {
		panic(errs) // this is how `v4` handles errors. `v5` has methods to have panic-free usage
	}
	return ris
}

// Static registers a new route with path prefix to serve static files from the provided root directory.
func (e *Echo) Static(pathPrefix, fsRoot string) RouteInfo {
	subFs := MustSubFS(e.Filesystem, fsRoot)
	return e.Add(
		http.MethodGet,
		pathPrefix+"*",
		StaticDirectoryHandler(subFs, false),
	)
}

// StaticFS registers a new route with path prefix to serve static files from the provided file system.
//
// When dealing with `embed.FS` use `fs := echo.MustSubFS(fs, "rootDirectory") to create sub fs which uses necessary
// prefix for directory path. This is necessary as `//go:embed assets/images` embeds files with paths
// including `assets/images` as their prefix.
func (e *Echo) StaticFS(pathPrefix string, filesystem fs.FS) RouteInfo {
	return e.Add(
		http.MethodGet,
		pathPrefix+"*",
		StaticDirectoryHandler(filesystem, false),
	)
}

// StaticDirectoryHandler creates handler function to serve files from provided file system
// When disablePathUnescaping is set then file name from path is not unescaped and is served as is.
func StaticDirectoryHandler(fileSystem fs.FS, disablePathUnescaping bool) HandlerFunc {
	return func(c *Context) error {
		p := c.Param("*")
		if !disablePathUnescaping { // when router is already unescaping we do not want to do is twice
			tmpPath, err := url.PathUnescape(p)
			if err != nil {
				return fmt.Errorf("failed to unescape path variable: %w", err)
			}
			p = tmpPath
		}

		// fs.FS.Open() already assumes that file names are relative to FS root path and considers name with prefix `/` as invalid
		name := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(p, "/")))
		fi, err := fs.Stat(fileSystem, name)
		if err != nil {
			return ErrNotFound
		}

		// If the request is for a directory and does not end with "/"
		p = c.Request().URL.Path // path must not be empty.
		if fi.IsDir() && len(p) > 0 && p[len(p)-1] != '/' {
			// Redirect to ends with "/"
			return c.Redirect(http.StatusMovedPermanently, sanitizeURI(p+"/"))
		}
		return fsFile(c, name, fileSystem)
	}
}

// FileFS registers a new route with path to serve file from the provided file system.
func (e *Echo) FileFS(path, file string, filesystem fs.FS, m ...MiddlewareFunc) RouteInfo {
	return e.GET(path, StaticFileHandler(file, filesystem), m...)
}

// StaticFileHandler creates handler function to serve file from provided file system
func StaticFileHandler(file string, filesystem fs.FS) HandlerFunc {
	return func(c *Context) error {
		return fsFile(c, file, filesystem)
	}
}

// File registers a new route with path to serve a static file with optional route-level middleware. Panics on error.
func (e *Echo) File(path, file string, middleware ...MiddlewareFunc) RouteInfo {
	handler := func(c *Context) error {
		return c.File(file)
	}
	return e.Add(http.MethodGet, path, handler, middleware...)
}

// AddRoute registers a new Route with default host Router
func (e *Echo) AddRoute(route Route) (RouteInfo, error) {
	return e.add("", route)
}

func (e *Echo) add(host string, route Route) (RouteInfo, error) {
	if e.OnAddRoute != nil {
		if err := e.OnAddRoute(route); err != nil {
			return RouteInfo{}, err
		}
	}

	ri, err := e.router.Add(route)
	if err != nil {
		return RouteInfo{}, err
	}

	paramsCount := int32(len(ri.Parameters))
	if paramsCount > e.contextPathParamAllocSize.Load() {
		e.contextPathParamAllocSize.Store(paramsCount)
	}
	return ri, nil
}

// Add registers a new route for an HTTP method and path with matching handler
// in the router with optional route-level middleware.
func (e *Echo) Add(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) RouteInfo {
	ri, err := e.add(
		"",
		Route{
			Method:      method,
			Path:        path,
			Handler:     handler,
			Middlewares: middleware,
			Name:        "",
		},
	)
	if err != nil {
		panic(err) // this is how `v4` handles errors. `v5` has methods to have panic-free usage
	}
	return ri
}

// Group creates a new router group with prefix and optional group-level middleware.
func (e *Echo) Group(prefix string, m ...MiddlewareFunc) (g *Group) {
	g = &Group{prefix: prefix, echo: e}
	g.Use(m...)
	return
}

// AcquireContext returns an empty `Context` instance from the pool.
// You must return the context by calling `ReleaseContext()`.
func (e *Echo) AcquireContext() *Context {
	return e.contextPool.Get().(*Context)
}

// ReleaseContext returns the `Context` instance back to the pool.
// You must call it after `AcquireContext()`.
func (e *Echo) ReleaseContext(c *Context) {
	e.contextPool.Put(c)
}

// ServeHTTP implements `http.Handler` interface, which serves HTTP requests.
func (e *Echo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.serveHTTPFunc(w, r)
}

// serveHTTP implements `http.Handler` interface, which serves HTTP requests.
func (e *Echo) serveHTTP(w http.ResponseWriter, r *http.Request) {
	c := e.contextPool.Get().(*Context)
	c.Reset(r, w)
	var h HandlerFunc

	if e.premiddleware == nil {
		h = applyMiddleware(e.router.Route(c), e.middleware...)
	} else {
		h = func(cc *Context) error {
			h1 := applyMiddleware(e.router.Route(cc), e.middleware...)
			return h1(cc)
		}
		h = applyMiddleware(h, e.premiddleware...)
	}

	// Execute chain
	if err := h(c); err != nil {
		e.HTTPErrorHandler(c, err)
	}

	e.contextPool.Put(c)
}

// Start stars HTTP server on given address with Echo as a handler serving requests. The server can be shutdown by
// sending os.Interrupt signal with `ctrl+c`.
//
// Note: this method is created for use in examples/demos and is deliberately simple without providing configuration
// options.
//
// In need of customization use:
//
//	sc := echo.StartConfig{Address: ":8080"}
//	if err := sc.Start(e); err != http.ErrServerClosed {
//		log.Fatal(err)
//	}
//
// // or standard library `http.Server`
//
//	s := http.Server{Addr: ":8080", Handler: e}
//	if err := s.ListenAndServe(); err != http.ErrServerClosed {
//		log.Fatal(err)
//	}
func (e *Echo) Start(address string) error {
	sc := StartConfig{Address: address}
	ctx, cancel := signal.NotifyContext(stdContext.Background(), os.Interrupt) // start shutdown process on ctrl+c
	defer cancel()
	sc.GracefulContext = ctx

	return sc.Start(e)
}

// WrapHandler wraps `http.Handler` into `echo.HandlerFunc`.
func WrapHandler(h http.Handler) HandlerFunc {
	return func(c *Context) error {
		req := c.Request()
		req.Pattern = c.Path()
		for _, p := range c.PathValues() {
			req.SetPathValue(p.Name, p.Value)
		}

		h.ServeHTTP(c.Response(), req)
		return nil
	}
}

// WrapMiddleware wraps `func(http.Handler) http.Handler` into `echo.MiddlewareFunc`
func WrapMiddleware(m func(http.Handler) http.Handler) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) (err error) {
			req := c.Request()
			req.Pattern = c.Path()
			for _, p := range c.PathValues() {
				req.SetPathValue(p.Name, p.Value)
			}

			m(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.SetRequest(r)
				c.SetResponse(NewResponse(w, c.echo.Logger))
				err = next(c)
			})).ServeHTTP(c.Response(), req)
			return
		}
	}
}

func applyMiddleware(h HandlerFunc, middleware ...MiddlewareFunc) HandlerFunc {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// defaultFS emulates os.Open behaviour with filesystem opened by `os.DirFs`. Difference between `os.Open` and `fs.Open`
// is that FS does not allow to open path that start with `..` or `/` etc. For example previously you could have `../images`
// in your application but `fs := os.DirFS("./")` would not allow you to use `fs.Open("../images")` and this would break
// all old applications that rely on being able to traverse up from current executable run path.
// NB: private because you really should use fs.FS implementation instances
type defaultFS struct {
	fs     fs.FS
	prefix string
}

func newDefaultFS() *defaultFS {
	dir, _ := os.Getwd()
	return &defaultFS{
		prefix: dir,
		fs:     nil,
	}
}

func (fs defaultFS) Open(name string) (fs.File, error) {
	if fs.fs == nil {
		return os.Open(name) // #nosec G304
	}
	return fs.fs.Open(name)
}

func subFS(currentFs fs.FS, root string) (fs.FS, error) {
	root = filepath.ToSlash(filepath.Clean(root)) // note: fs.FS operates only with slashes. `ToSlash` is necessary for Windows
	if dFS, ok := currentFs.(*defaultFS); ok {
		// we need to make exception for `defaultFS` instances as it interprets root prefix differently from fs.FS.
		// fs.Fs.Open does not like relative paths ("./", "../") and absolute paths at all but prior echo.Filesystem we
		// were able to use paths like `./myfile.log`, `/etc/hosts` and these would work fine with `os.Open` but not with fs.Fs
		if !filepath.IsAbs(root) {
			root = filepath.Join(dFS.prefix, root)
		}
		return &defaultFS{
			prefix: root,
			fs:     os.DirFS(root),
		}, nil
	}
	return fs.Sub(currentFs, root)
}

// MustSubFS creates sub FS from current filesystem or panic on failure.
// Panic happens when `fsRoot` contains invalid path according to `fs.ValidPath` rules.
//
// MustSubFS is helpful when dealing with `embed.FS` because for example `//go:embed assets/images` embeds files with
// paths including `assets/images` as their prefix. In that case use `fs := echo.MustSubFS(fs, "rootDirectory") to
// create sub fs which uses necessary prefix for directory path.
func MustSubFS(currentFs fs.FS, fsRoot string) fs.FS {
	subFs, err := subFS(currentFs, fsRoot)
	if err != nil {
		panic(fmt.Errorf("can not create sub FS, invalid root given, err: %w", err))
	}
	return subFs
}

func sanitizeURI(uri string) string {
	// double slash `\\`, `//` or even `\/` is absolute uri for browsers and by redirecting request to that uri
	// we are vulnerable to open redirect attack. so replace all slashes from the beginning with single slash
	if len(uri) > 1 && (uri[0] == '\\' || uri[0] == '/') && (uri[1] == '\\' || uri[1] == '/') {
		uri = "/" + strings.TrimLeft(uri, `/\`)
	}
	return uri
}
