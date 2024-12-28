// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package echo

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// ContextKeyHeaderAllow is set by Router for getting value for `Allow` header in later stages of handler call chain.
	// Allow header is mandatory for status 405 (method not found) and useful for OPTIONS method requests.
	// It is added to context only when Router does not find matching method handler for request.
	ContextKeyHeaderAllow = "echo_header_allow"
)

const (
	defaultMemory = 32 << 20 // 32 MB
	indexPage     = "index.html"
)

// Context represents the context of the current HTTP request. It holds request and
// response objects, path, path parameters, data and registered handler.
type Context struct {
	route         RouteInfo
	request       *http.Request
	response      *Response
	pathParams    *PathParams
	query         url.Values
	store         Map
	echo          *Echo
	logger        *slog.Logger
	path          string
	currentParams PathParams
	lock          sync.RWMutex
}

// NewContext creates new instance of Context.
// Argument pathParamAllocSize must be value that is stored in *echo.ContextPathParamAllocSize field and is used
// to preallocate PathParams slice.
func NewContext(e *Echo, pathParamAllocSize int) *Context {
	p := make(PathParams, pathParamAllocSize)
	return &Context{
		pathParams: &p,
		store:      make(Map),
		echo:       e,
	}
}

// Reset resets the context after request completes. It must be called along
// with `Echo#AcquireContext()` and `Echo#ReleaseContext()`.
// See `Echo#ServeHTTP()`
func (c *Context) Reset(r *http.Request, w http.ResponseWriter) {
	c.request = r
	c.response.reset(w)
	c.query = nil
	c.store = nil

	c.route = nil
	c.path = ""
	// NOTE: Don't reset because it has to have length of c.echo.contextPathParamAllocSize at all times
	*c.pathParams = (*c.pathParams)[:0]
	c.currentParams = nil
}

func (c *Context) writeContentType(value string) {
	header := c.Response().Header()
	if header.Get(HeaderContentType) == "" {
		header.Set(HeaderContentType, value)
	}
}

// Request returns `*http.Request`.
func (c *Context) Request() *http.Request {
	return c.request
}

// SetRequest sets `*http.Request`.
func (c *Context) SetRequest(r *http.Request) {
	c.request = r
}

// Response returns `*Response`.
func (c *Context) Response() *Response {
	return c.response
}

// SetResponse sets `*Response`.
func (c *Context) SetResponse(r *Response) {
	c.response = r
}

// IsTLS returns true if HTTP connection is TLS otherwise false.
func (c *Context) IsTLS() bool {
	return c.request.TLS != nil
}

// IsWebSocket returns true if HTTP connection is WebSocket otherwise false.
func (c *Context) IsWebSocket() bool {
	upgrade := c.request.Header.Get(HeaderUpgrade)
	return strings.EqualFold(upgrade, "websocket")
}

// Scheme returns the HTTP protocol scheme, `http` or `https`.
func (c *Context) Scheme() string {
	// Can't use `r.Request.URL.Scheme`
	// See: https://groups.google.com/forum/#!topic/golang-nuts/pMUkBlQBDF0
	if c.IsTLS() {
		return "https"
	}
	if scheme := c.request.Header.Get(HeaderXForwardedProto); scheme != "" {
		return scheme
	}
	if scheme := c.request.Header.Get(HeaderXForwardedProtocol); scheme != "" {
		return scheme
	}
	if ssl := c.request.Header.Get(HeaderXForwardedSsl); ssl == "on" {
		return "https"
	}
	if scheme := c.request.Header.Get(HeaderXUrlScheme); scheme != "" {
		return scheme
	}
	return "http"
}

// RealIP returns the client's network address based on `X-Forwarded-For`
// or `X-Real-IP` request header.
// The behavior can be configured using `Echo#IPExtractor`.
func (c *Context) RealIP() string {
	if c.echo != nil && c.echo.IPExtractor != nil {
		return c.echo.IPExtractor(c.request)
	}
	// Fall back to legacy behavior
	if ip := c.request.Header.Get(HeaderXForwardedFor); ip != "" {
		i := strings.IndexAny(ip, ",")
		if i > 0 {
			xffip := strings.TrimSpace(ip[:i])
			xffip = strings.TrimPrefix(xffip, "[")
			xffip = strings.TrimSuffix(xffip, "]")
			return xffip
		}
		return ip
	}
	if ip := c.request.Header.Get(HeaderXRealIP); ip != "" {
		ip = strings.TrimPrefix(ip, "[")
		ip = strings.TrimSuffix(ip, "]")
		return ip
	}
	ra, _, _ := net.SplitHostPort(c.request.RemoteAddr)
	return ra
}

// Path returns the registered path for the handler.
func (c *Context) Path() string {
	return c.path
}

// SetPath sets the registered path for the handler.
func (c *Context) SetPath(p string) {
	c.path = p
}

// RouteInfo returns current request route information. Method, Path, Name and params if they exist for matched route.
// In case of 404 (route not found) and 405 (method not allowed) RouteInfo returns generic struct for these cases.
func (c *Context) RouteInfo() RouteInfo {
	return c.route
}

// SetRouteInfo sets the route info of this request to the context.
func (c *Context) SetRouteInfo(ri RouteInfo) {
	c.route = ri
}

// RawPathParams returns raw path pathParams value. Allocation of PathParams is handled by Context.
func (c *Context) RawPathParams() *PathParams {
	return c.pathParams
}

// SetRawPathParams replaces any existing param values with new values for this context lifetime (request).
//
// DO NOT USE!
// Do not set any other value than what you got from RawPathParams as allocation of PathParams is handled by Context.
// If you mess up size of pathParams size your application will panic/crash during routing
func (c *Context) SetRawPathParams(params *PathParams) {
	c.pathParams = params
}

// PathParam returns path parameter by name.
func (c *Context) PathParam(name string) string {
	if c.currentParams != nil {
		return c.currentParams.Get(name, "")
	}

	return c.pathParams.Get(name, "")
}

// PathParamDefault returns the path parameter or default value for the provided name.
//
// Notes for DefaultRouter implementation:
// Path parameter could be empty for cases like that:
// * route `/release-:version/bin` and request URL is `/release-/bin`
// * route `/api/:version/image.jpg` and request URL is `/api//image.jpg`
// but not when path parameter is last part of route path
// * route `/download/file.:ext` will not match request `/download/file.`
func (c *Context) PathParamDefault(name, defaultValue string) string {
	return c.pathParams.Get(name, defaultValue)
}

// PathParams returns path parameter values.
func (c *Context) PathParams() PathParams {
	if c.currentParams != nil {
		return c.currentParams
	}

	result := make(PathParams, len(*c.pathParams))
	copy(result, *c.pathParams)
	return result
}

// SetPathParams sets path parameters for current request.
func (c *Context) SetPathParams(params PathParams) {
	c.currentParams = params
}

// QueryParam returns the query param for the provided name.
func (c *Context) QueryParam(name string) string {
	if c.query == nil {
		c.query = c.request.URL.Query()
	}
	return c.query.Get(name)
}

// QueryParamDefault returns the query param or default value for the provided name.
// Note: QueryParamDefault does not distinguish if query had no value by that name or value was empty string
// This means URLs `/test?search=` and `/test` would both return `1` for `c.QueryParamDefault("search", "1")`
func (c *Context) QueryParamDefault(name, defaultValue string) string {
	value := c.QueryParam(name)
	if value == "" {
		value = defaultValue
	}
	return value
}

// QueryParams returns the query parameters as `url.Values`.
func (c *Context) QueryParams() url.Values {
	if c.query == nil {
		c.query = c.request.URL.Query()
	}
	return c.query
}

// QueryString returns the URL query string.
func (c *Context) QueryString() string {
	return c.request.URL.RawQuery
}

// FormValue returns the form field value for the provided name.
func (c *Context) FormValue(name string) string {
	return c.request.FormValue(name)
}

// FormValueDefault returns the form field value or default value for the provided name.
// Note: FormValueDefault does not distinguish if form had no value by that name or value was empty string
func (c *Context) FormValueDefault(name, defaultValue string) string {
	value := c.FormValue(name)
	if value == "" {
		value = defaultValue
	}
	return value
}

// FormValues returns the form field values as `url.Values`.
func (c *Context) FormValues() (url.Values, error) {
	if strings.HasPrefix(c.request.Header.Get(HeaderContentType), MIMEMultipartForm) {
		if err := c.request.ParseMultipartForm(defaultMemory); err != nil {
			return nil, err
		}
	} else {
		if err := c.request.ParseForm(); err != nil {
			return nil, err
		}
	}
	return c.request.Form, nil
}

// FormFile returns the multipart form file for the provided name.
func (c *Context) FormFile(name string) (*multipart.FileHeader, error) {
	f, fh, err := c.request.FormFile(name)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return fh, nil
}

// MultipartForm returns the multipart form.
func (c *Context) MultipartForm() (*multipart.Form, error) {
	err := c.request.ParseMultipartForm(defaultMemory)
	return c.request.MultipartForm, err
}

// Cookie returns the named cookie provided in the request.
func (c *Context) Cookie(name string) (*http.Cookie, error) {
	return c.request.Cookie(name)
}

// SetCookie adds a `Set-Cookie` header in HTTP response.
func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.Response(), cookie)
}

// Cookies returns the HTTP cookies sent with the request.
func (c *Context) Cookies() []*http.Cookie {
	return c.request.Cookies()
}

// Get retrieves data from the context.
func (c *Context) Get(key string) any {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.store[key]
}

// Set saves data in the context.
func (c *Context) Set(key string, val any) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.store == nil {
		c.store = make(Map)
	}
	c.store[key] = val
}

// Bind binds path params, query params and the request body into provided type `i`. The default binder
// binds body based on Content-Type header.
func (c *Context) Bind(i any) error {
	return c.echo.Binder.Bind(c, i)
}

// Validate validates provided `i`. It is usually called after `Context#Bind()`.
// Validator must be registered using `Echo#Validator`.
func (c *Context) Validate(i any) error {
	if c.echo.Validator == nil {
		return ErrValidatorNotRegistered
	}
	return c.echo.Validator.Validate(i)
}

// Render renders a template with data and sends a text/html response with status
// code. Renderer must be registered using `Echo.Renderer`.
func (c *Context) Render(code int, name string, data any) (err error) {
	if c.echo.Renderer == nil {
		return ErrRendererNotRegistered
	}
	buf := new(bytes.Buffer)
	if err = c.echo.Renderer.Render(c, buf, name, data); err != nil {
		return
	}
	return c.HTMLBlob(code, buf.Bytes())
}

// HTML sends an HTTP response with status code.
func (c *Context) HTML(code int, html string) (err error) {
	return c.HTMLBlob(code, []byte(html))
}

// HTMLBlob sends an HTTP blob response with status code.
func (c *Context) HTMLBlob(code int, b []byte) (err error) {
	return c.Blob(code, MIMETextHTMLCharsetUTF8, b)
}

// String sends a string response with status code.
func (c *Context) String(code int, s string) (err error) {
	return c.Blob(code, MIMETextPlainCharsetUTF8, []byte(s))
}

func (c *Context) jsonPBlob(code int, callback string, i any) (err error) {
	c.writeContentType(MIMEApplicationJavaScriptCharsetUTF8)
	c.response.WriteHeader(code)
	if _, err = c.response.Write([]byte(callback + "(")); err != nil {
		return
	}
	if err = c.echo.JSONSerializer.Serialize(c, i, ""); err != nil {
		return
	}
	if _, err = c.response.Write([]byte(");")); err != nil {
		return
	}
	return
}

func (c *Context) json(code int, i any, indent string) error {
	c.writeContentType(MIMEApplicationJSON)
	c.response.Status = code
	return c.echo.JSONSerializer.Serialize(c, i, indent)
}

// JSON sends a JSON response with status code.
func (c *Context) JSON(code int, i any) (err error) {
	return c.json(code, i, "")
}

// JSONPretty sends a pretty-print JSON with status code.
func (c *Context) JSONPretty(code int, i any, indent string) (err error) {
	return c.json(code, i, indent)
}

// JSONBlob sends a JSON blob response with status code.
func (c *Context) JSONBlob(code int, b []byte) (err error) {
	return c.Blob(code, MIMEApplicationJSON, b)
}

// JSONP sends a JSONP response with status code. It uses `callback` to construct
// the JSONP payload.
func (c *Context) JSONP(code int, callback string, i any) (err error) {
	return c.jsonPBlob(code, callback, i)
}

// JSONPBlob sends a JSONP blob response with status code. It uses `callback`
// to construct the JSONP payload.
func (c *Context) JSONPBlob(code int, callback string, b []byte) (err error) {
	c.writeContentType(MIMEApplicationJavaScriptCharsetUTF8)
	c.response.WriteHeader(code)
	if _, err = c.response.Write([]byte(callback + "(")); err != nil {
		return
	}
	if _, err = c.response.Write(b); err != nil {
		return
	}
	_, err = c.response.Write([]byte(");"))
	return
}

func (c *Context) xml(code int, i any, indent string) (err error) {
	c.writeContentType(MIMEApplicationXMLCharsetUTF8)
	c.response.WriteHeader(code)
	enc := xml.NewEncoder(c.response)
	if indent != "" {
		enc.Indent("", indent)
	}
	if _, err = c.response.Write([]byte(xml.Header)); err != nil {
		return
	}
	return enc.Encode(i)
}

// XML sends an XML response with status code.
func (c *Context) XML(code int, i any) (err error) {
	return c.xml(code, i, "")
}

// XMLPretty sends a pretty-print XML with status code.
func (c *Context) XMLPretty(code int, i any, indent string) (err error) {
	return c.xml(code, i, indent)
}

// XMLBlob sends an XML blob response with status code.
func (c *Context) XMLBlob(code int, b []byte) (err error) {
	c.writeContentType(MIMEApplicationXMLCharsetUTF8)
	c.response.WriteHeader(code)
	if _, err = c.response.Write([]byte(xml.Header)); err != nil {
		return
	}
	_, err = c.response.Write(b)
	return
}

// Blob sends a blob response with status code and content type.
func (c *Context) Blob(code int, contentType string, b []byte) (err error) {
	c.writeContentType(contentType)
	c.response.WriteHeader(code)
	_, err = c.response.Write(b)
	return
}

// Stream sends a streaming response with status code and content type.
func (c *Context) Stream(code int, contentType string, r io.Reader) (err error) {
	c.writeContentType(contentType)
	c.response.WriteHeader(code)
	_, err = io.Copy(c.response, r)
	return
}

// File sends a response with the content of the file.
func (c *Context) File(file string) error {
	return fsFile(c, file, c.echo.Filesystem)
}

// FileFS serves file from given file system.
//
// When dealing with `embed.FS` use `fs := echo.MustSubFS(fs, "rootDirectory") to create sub fs which uses necessary
// prefix for directory path. This is necessary as `//go:embed assets/images` embeds files with paths
// including `assets/images` as their prefix.
func (c *Context) FileFS(file string, filesystem fs.FS) error {
	return fsFile(c, file, filesystem)
}

func fsFile(c *Context, file string, filesystem fs.FS) error {
	f, err := filesystem.Open(file)
	if err != nil {
		return ErrNotFound
	}
	defer f.Close()

	fi, _ := f.Stat()
	if fi.IsDir() {
		file = filepath.ToSlash(filepath.Join(file, indexPage)) // ToSlash is necessary for Windows. fs.Open and os.Open are different in that aspect.
		f, err = filesystem.Open(file)
		if err != nil {
			return ErrNotFound
		}
		defer f.Close()
		if fi, err = f.Stat(); err != nil {
			return err
		}
	}
	ff, ok := f.(io.ReadSeeker)
	if !ok {
		return errors.New("file does not implement io.ReadSeeker")
	}
	http.ServeContent(c.Response(), c.Request(), fi.Name(), fi.ModTime(), ff)
	return nil
}

// Attachment sends a response as attachment, prompting client to save the file.
func (c *Context) Attachment(file, name string) error {
	return c.contentDisposition(file, name, "attachment")
}

// Inline sends a response as inline, opening the file in the browser.
func (c *Context) Inline(file, name string) error {
	return c.contentDisposition(file, name, "inline")
}

var quoteEscaper = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")

func (c *Context) contentDisposition(file, name, dispositionType string) error {
	c.response.Header().Set(HeaderContentDisposition, fmt.Sprintf(`%s; filename="%s"`, dispositionType, quoteEscaper.Replace(name)))
	return c.File(file)
}

// NoContent sends a response with no body and a status code.
func (c *Context) NoContent(code int) error {
	c.response.WriteHeader(code)
	return nil
}

// Redirect redirects the request to a provided URL with status code.
func (c *Context) Redirect(code int, url string) error {
	if code < 300 || code > 308 {
		return ErrInvalidRedirectCode
	}
	c.response.Header().Set(HeaderLocation, url)
	c.response.WriteHeader(code)
	return nil
}

// Logger returns logger in Context
func (c *Context) Logger() *slog.Logger {
	return c.logger
}

// Echo returns the `Echo` instance.
func (c *Context) Echo() *Echo {
	return c.echo
}
