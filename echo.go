/*
Package echo implements high performance, minimalist Go web framework.

Example:

  package main

  import (
    "net/http"

    "github.com/labstack/echo/v4"
    "github.com/labstack/echo/v4/middleware"
  )

  // Handler
  func hello(c echo.Context) error {
    return c.String(http.StatusOK, "Hello, World!")
  }

  func main() {
    // Echo instance
    e := echo.New()

    // Middleware
    e.Use(middleware.Logger())
    e.Use(middleware.Recover())

    // Routes
    e.GET("/", hello)

    // Start server
    e.Logger.Fatal(e.Start(":1323"))
  }

Learn more at https://echo.labstack.com
*/
package echo

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/labstack/gommon/log"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
)

type (
	// Echo is the top-level framework instance.
	Echo struct {
		common
		premiddleware    []MiddlewareFunc
		middleware       []MiddlewareFunc
		maxParam         *int
		router           *Router
		routers          map[string]*Router
		notFoundHandler  HandlerFunc
		pool             sync.Pool
		Debug            bool
		HTTPErrorHandler HTTPErrorHandler
		Binder           Binder
		Validator        Validator
		Renderer         Renderer
		Logger           Logger
		IPExtractor      IPExtractor
	}

	// Route contains a handler and information for matching against requests.
	Route struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Name   string `json:"name"`
	}

	// HTTPError represents an error that occurred while handling a request.
	HTTPError struct {
		Code     int         `json:"-"`
		Message  interface{} `json:"message"`
		Internal error       `json:"-"` // Stores the error returned by an external dependency
	}

	// MiddlewareFunc defines a function to process middleware.
	MiddlewareFunc func(HandlerFunc) HandlerFunc

	// HandlerFunc defines a function to serve HTTP requests.
	HandlerFunc func(Context) error

	// HTTPErrorHandler is a centralized HTTP error handler.
	HTTPErrorHandler func(error, Context)

	// Validator is the interface that wraps the Validate function.
	Validator interface {
		Validate(i interface{}) error
	}

	// Renderer is the interface that wraps the Render function.
	Renderer interface {
		Render(io.Writer, string, interface{}, Context) error
	}

	// Map defines a generic map of type `map[string]interface{}`.
	Map map[string]interface{}

	// Common struct for Echo & Group.
	common struct{}
)

// HTTP methods
// NOTE: Deprecated, please use the stdlib constants directly instead.
const (
	CONNECT = http.MethodConnect
	DELETE  = http.MethodDelete
	GET     = http.MethodGet
	HEAD    = http.MethodHead
	OPTIONS = http.MethodOptions
	PATCH   = http.MethodPatch
	POST    = http.MethodPost
	// PROPFIND = "PROPFIND"
	PUT   = http.MethodPut
	TRACE = http.MethodTrace
)

// MIME types
const (
	MIMEApplicationJSON                  = "application/json"
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
)

// Headers
const (
	HeaderAccept              = "Accept"
	HeaderAcceptEncoding      = "Accept-Encoding"
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
	HeaderUpgrade             = "Upgrade"
	HeaderVary                = "Vary"
	HeaderWWWAuthenticate     = "WWW-Authenticate"
	HeaderXForwardedFor       = "X-Forwarded-For"
	HeaderXForwardedProto     = "X-Forwarded-Proto"
	HeaderXForwardedProtocol  = "X-Forwarded-Protocol"
	HeaderXForwardedSsl       = "X-Forwarded-Ssl"
	HeaderXUrlScheme          = "X-Url-Scheme"
	HeaderXHTTPMethodOverride = "X-HTTP-Method-Override"
	HeaderXRealIP             = "X-Real-IP"
	HeaderXRequestID          = "X-Request-ID"
	HeaderXRequestedWith      = "X-Requested-With"
	HeaderServer              = "Server"
	HeaderOrigin              = "Origin"

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
	HeaderXCSRFToken                      = "X-CSRF-Token"
	HeaderReferrerPolicy                  = "Referrer-Policy"
)

const (
	// Version of Echo
	Version = "4.3.0"
)

var (
	methods = [...]string{
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
)

// Errors
var (
	ErrUnsupportedMediaType        = NewHTTPError(http.StatusUnsupportedMediaType)
	ErrNotFound                    = NewHTTPError(http.StatusNotFound)
	ErrUnauthorized                = NewHTTPError(http.StatusUnauthorized)
	ErrForbidden                   = NewHTTPError(http.StatusForbidden)
	ErrMethodNotAllowed            = NewHTTPError(http.StatusMethodNotAllowed)
	ErrStatusRequestEntityTooLarge = NewHTTPError(http.StatusRequestEntityTooLarge)
	ErrTooManyRequests             = NewHTTPError(http.StatusTooManyRequests)
	ErrBadRequest                  = NewHTTPError(http.StatusBadRequest)
	ErrBadGateway                  = NewHTTPError(http.StatusBadGateway)
	ErrInternalServerError         = NewHTTPError(http.StatusInternalServerError)
	ErrRequestTimeout              = NewHTTPError(http.StatusRequestTimeout)
	ErrServiceUnavailable          = NewHTTPError(http.StatusServiceUnavailable)
	ErrValidatorNotRegistered      = errors.New("validator not registered")
	ErrRendererNotRegistered       = errors.New("renderer not registered")
	ErrInvalidRedirectCode         = errors.New("invalid redirect status code")
	ErrCookieNotFound              = errors.New("cookie not found")
	ErrInvalidCertOrKeyType        = errors.New("invalid cert or key type, must be string or []byte")
	ErrInvalidListenerNetwork      = errors.New("invalid listener network")
)

// Error handlers
var (
	NotFoundHandler = func(c Context) error {
		return ErrNotFound
	}

	MethodNotAllowedHandler = func(c Context) error {
		return ErrMethodNotAllowed
	}
)

// New creates an instance of Echo.
func New() (e *Echo) {
	e = &Echo{
		Logger:   log.New("echo"),
		maxParam: new(int),
	}
	e.Logger.SetLevel(log.ERROR)
	if e.Debug {
		e.Logger.SetLevel(log.DEBUG)
	}

	e.HTTPErrorHandler = e.DefaultHTTPErrorHandler
	e.Binder = &DefaultBinder{}
	e.pool.New = func() interface{} {
		return e.NewContext(nil, nil)
	}
	e.router = NewRouter(e)
	e.routers = map[string]*Router{}
	return
}

// NewContext returns a Context instance.
func (e *Echo) NewContext(r *http.Request, w http.ResponseWriter) Context {
	return &context{
		request:  r,
		response: NewResponse(w, e),
		store:    make(Map),
		echo:     e,
		pvalues:  make([]string, *e.maxParam),
		handler:  NotFoundHandler,
	}
}

// Router returns the default router.
func (e *Echo) Router() *Router {
	return e.router
}

// Routers returns the map of host => router.
func (e *Echo) Routers() map[string]*Router {
	return e.routers
}

// DefaultHTTPErrorHandler is the default HTTP error handler. It sends a JSON response
// with status code.
func (e *Echo) DefaultHTTPErrorHandler(err error, c Context) {
	he, ok := err.(*HTTPError)
	if ok {
		if he.Internal != nil {
			if herr, ok := he.Internal.(*HTTPError); ok {
				he = herr
			}
		}
	} else {
		he = &HTTPError{
			Code:    http.StatusInternalServerError,
			Message: http.StatusText(http.StatusInternalServerError),
		}
	}

	// Issue #1426
	code := he.Code
	message := he.Message
	if m, ok := he.Message.(string); ok {
		if e.Debug {
			message = Map{"message": m, "error": err.Error()}
		} else {
			message = Map{"message": m}
		}
	}

	// Send response
	if !c.Response().Committed {
		if c.Request().Method == http.MethodHead { // Issue #608
			err = c.NoContent(he.Code)
		} else {
			err = c.JSON(code, message)
		}
		if err != nil {
			e.Logger.Error(err)
		}
	}
}

// Pre adds middleware to the chain which is run before router.
func (e *Echo) Pre(middleware ...MiddlewareFunc) {
	e.premiddleware = append(e.premiddleware, middleware...)
}

// Use adds middleware to the chain which is run after router.
func (e *Echo) Use(middleware ...MiddlewareFunc) {
	e.middleware = append(e.middleware, middleware...)
}

// CONNECT registers a new CONNECT route for a path with matching handler in the
// router with optional route-level middleware.
func (e *Echo) CONNECT(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodConnect, path, h, m...)
}

// DELETE registers a new DELETE route for a path with matching handler in the router
// with optional route-level middleware.
func (e *Echo) DELETE(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodDelete, path, h, m...)
}

// GET registers a new GET route for a path with matching handler in the router
// with optional route-level middleware.
func (e *Echo) GET(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodGet, path, h, m...)
}

// HEAD registers a new HEAD route for a path with matching handler in the
// router with optional route-level middleware.
func (e *Echo) HEAD(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodHead, path, h, m...)
}

// OPTIONS registers a new OPTIONS route for a path with matching handler in the
// router with optional route-level middleware.
func (e *Echo) OPTIONS(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodOptions, path, h, m...)
}

// PATCH registers a new PATCH route for a path with matching handler in the
// router with optional route-level middleware.
func (e *Echo) PATCH(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodPatch, path, h, m...)
}

// POST registers a new POST route for a path with matching handler in the
// router with optional route-level middleware.
func (e *Echo) POST(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodPost, path, h, m...)
}

// PUT registers a new PUT route for a path with matching handler in the
// router with optional route-level middleware.
func (e *Echo) PUT(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodPut, path, h, m...)
}

// TRACE registers a new TRACE route for a path with matching handler in the
// router with optional route-level middleware.
func (e *Echo) TRACE(path string, h HandlerFunc, m ...MiddlewareFunc) *Route {
	return e.Add(http.MethodTrace, path, h, m...)
}

// Any registers a new route for all HTTP methods and path with matching handler
// in the router with optional route-level middleware.
func (e *Echo) Any(path string, handler HandlerFunc, middleware ...MiddlewareFunc) []*Route {
	routes := make([]*Route, len(methods))
	for i, m := range methods {
		routes[i] = e.Add(m, path, handler, middleware...)
	}
	return routes
}

// Match registers a new route for multiple HTTP methods and path with matching
// handler in the router with optional route-level middleware.
func (e *Echo) Match(methods []string, path string, handler HandlerFunc, middleware ...MiddlewareFunc) []*Route {
	routes := make([]*Route, len(methods))
	for i, m := range methods {
		routes[i] = e.Add(m, path, handler, middleware...)
	}
	return routes
}

// Static registers a new route with path prefix to serve static files from the
// provided root directory.
func (e *Echo) Static(prefix, root string) *Route {
	if root == "" {
		root = "." // For security we want to restrict to CWD.
	}
	return e.static(prefix, root, e.GET)
}

func (common) static(prefix, root string, get func(string, HandlerFunc, ...MiddlewareFunc) *Route) *Route {
	h := func(c Context) error {
		p, err := url.PathUnescape(c.Param("*"))
		if err != nil {
			return err
		}

		name := filepath.Join(root, filepath.Clean("/"+p)) // "/"+ for security
		fi, err := os.Stat(name)
		if err != nil {
			// The access path does not exist
			return NotFoundHandler(c)
		}

		// If the request is for a directory and does not end with "/"
		p = c.Request().URL.Path // path must not be empty.
		if fi.IsDir() && p[len(p)-1] != '/' {
			// Redirect to ends with "/"
			return c.Redirect(http.StatusMovedPermanently, p+"/")
		}
		return c.File(name)
	}
	// Handle added routes based on trailing slash:
	// 	/prefix  => exact route "/prefix" + any route "/prefix/*"
	// 	/prefix/ => only any route "/prefix/*"
	if prefix != "" {
		if prefix[len(prefix)-1] == '/' {
			// Only add any route for intentional trailing slash
			return get(prefix+"*", h)
		}
		get(prefix, h)
	}
	return get(prefix+"/*", h)
}

func (common) file(path, file string, get func(string, HandlerFunc, ...MiddlewareFunc) *Route,
	m ...MiddlewareFunc) *Route {
	return get(path, func(c Context) error {
		return c.File(file)
	}, m...)
}

// File registers a new route with path to serve a static file with optional route-level middleware.
func (e *Echo) File(path, file string, m ...MiddlewareFunc) *Route {
	return e.file(path, file, e.GET, m...)
}

func (e *Echo) add(host, method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	name := handlerName(handler)
	router := e.findRouter(host)
	router.Add(method, path, func(c Context) error {
		h := applyMiddleware(handler, middleware...)
		return h(c)
	})
	r := &Route{
		Method: method,
		Path:   path,
		Name:   name,
	}
	e.router.routes[method+path] = r
	return r
}

// Add registers a new route for an HTTP method and path with matching handler
// in the router with optional route-level middleware.
func (e *Echo) Add(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route {
	return e.add("", method, path, handler, middleware...)
}

// Host creates a new router group for the provided host and optional host-level middleware.
func (e *Echo) Host(name string, m ...MiddlewareFunc) (g *Group) {
	e.routers[name] = NewRouter(e)
	g = &Group{host: name, echo: e}
	g.Use(m...)
	return
}

// Group creates a new router group with prefix and optional group-level middleware.
func (e *Echo) Group(prefix string, m ...MiddlewareFunc) (g *Group) {
	g = &Group{prefix: prefix, echo: e}
	g.Use(m...)
	return
}

// URI generates a URI from handler.
func (e *Echo) URI(handler HandlerFunc, params ...interface{}) string {
	name := handlerName(handler)
	return e.Reverse(name, params...)
}

// URL is an alias for `URI` function.
func (e *Echo) URL(h HandlerFunc, params ...interface{}) string {
	return e.URI(h, params...)
}

// Reverse generates an URL from route name and provided parameters.
func (e *Echo) Reverse(name string, params ...interface{}) string {
	uri := new(bytes.Buffer)
	ln := len(params)
	n := 0
	for _, r := range e.router.routes {
		if r.Name == name {
			for i, l := 0, len(r.Path); i < l; i++ {
				if (r.Path[i] == ':' || r.Path[i] == '*') && n < ln {
					for ; i < l && r.Path[i] != '/'; i++ {
					}
					uri.WriteString(fmt.Sprintf("%v", params[n]))
					n++
				}
				if i < l {
					uri.WriteByte(r.Path[i])
				}
			}
			break
		}
	}
	return uri.String()
}

// Routes returns the registered routes.
func (e *Echo) Routes() []*Route {
	routes := make([]*Route, 0, len(e.router.routes))
	for _, v := range e.router.routes {
		routes = append(routes, v)
	}
	return routes
}

// AcquireContext returns an empty `Context` instance from the pool.
// You must return the context by calling `ReleaseContext()`.
func (e *Echo) AcquireContext() Context {
	return e.pool.Get().(Context)
}

// ReleaseContext returns the `Context` instance back to the pool.
// You must call it after `AcquireContext()`.
func (e *Echo) ReleaseContext(c Context) {
	e.pool.Put(c)
}

// ServeHTTP implements `http.Handler` interface, which serves HTTP requests.
func (e *Echo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Acquire context
	c := e.pool.Get().(*context)
	c.Reset(r, w)
	h := NotFoundHandler

	if e.premiddleware == nil {
		e.findRouter(r.Host).Find(r.Method, GetPath(r), c)
		h = c.Handler()
		h = applyMiddleware(h, e.middleware...)
	} else {
		h = func(c Context) error {
			e.findRouter(r.Host).Find(r.Method, GetPath(r), c)
			h := c.Handler()
			h = applyMiddleware(h, e.middleware...)
			return h(c)
		}
		h = applyMiddleware(h, e.premiddleware...)
	}

	// Execute chain
	if err := h(c); err != nil {
		e.HTTPErrorHandler(err, c)
	}

	// Release context
	e.pool.Put(c)
}

// NewHTTPError creates a new HTTPError instance.
func NewHTTPError(code int, message ...interface{}) *HTTPError {
	he := &HTTPError{Code: code, Message: http.StatusText(code)}
	if len(message) > 0 {
		he.Message = message[0]
	}
	return he
}

// Error makes it compatible with `error` interface.
func (he *HTTPError) Error() string {
	if he.Internal == nil {
		return fmt.Sprintf("code=%d, message=%v", he.Code, he.Message)
	}
	return fmt.Sprintf("code=%d, message=%v, internal=%v", he.Code, he.Message, he.Internal)
}

// SetInternal sets error to HTTPError.Internal
func (he *HTTPError) SetInternal(err error) *HTTPError {
	he.Internal = err
	return he
}

// Unwrap satisfies the Go 1.13 error wrapper interface.
func (he *HTTPError) Unwrap() error {
	return he.Internal
}

// WrapHandler wraps `http.Handler` into `echo.HandlerFunc`.
func WrapHandler(h http.Handler) HandlerFunc {
	return func(c Context) error {
		h.ServeHTTP(c.Response(), c.Request())
		return nil
	}
}

// WrapMiddleware wraps `func(http.Handler) http.Handler` into `echo.MiddlewareFunc`
func WrapMiddleware(m func(http.Handler) http.Handler) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c Context) (err error) {
			m(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c.SetRequest(r)
				c.SetResponse(NewResponse(w, c.Echo()))
				err = next(c)
			})).ServeHTTP(c.Response(), c.Request())
			return
		}
	}
}

// GetPath returns RawPath, if it's empty returns Path from URL
// Difference between RawPath and Path is:
//  * Path is where request path is stored. Value is stored in decoded form: /%47%6f%2f becomes /Go/.
//  * RawPath is an optional field which only gets set if the default encoding is different from Path.
func GetPath(r *http.Request) string {
	path := r.URL.RawPath
	if path == "" {
		path = r.URL.Path
	}
	return path
}

func (e *Echo) findRouter(host string) *Router {
	if len(e.routers) > 0 {
		if r, ok := e.routers[host]; ok {
			return r
		}
	}
	return e.router
}

func handlerName(h HandlerFunc) string {
	t := reflect.ValueOf(h).Type()
	if t.Kind() == reflect.Func {
		return runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	}
	return t.String()
}

func applyMiddleware(h HandlerFunc, middleware ...MiddlewareFunc) HandlerFunc {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
