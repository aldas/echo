package echo

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"runtime"
)

// Route contains a handler and information for matching against requests.
// Method+Path pair uniquely identifies the Route.
type Route struct {
	Method      string           `json:"method"`
	Path        string           `json:"path"`
	Handler     HandlerFunc      `json:"-"`
	Middlewares []MiddlewareFunc `json:"-"`

	Name string `json:"name"`
}

// Routes is collection of Route instances with various helper methods.
type Routes []Route

// Reverse reverses route to URL string by replacing path parameters with given params values.
func (r Route) Reverse(params ...interface{}) string {
	uri := new(bytes.Buffer)
	ln := len(params)
	n := 0
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
	return uri.String()
}

// FIXME: is replaced with `x, err := e.Router().Routes().Reverse(echo.HandlerName(getUser), "1")`
//// URI generates a URI from handler.
//func (r Routes) URI(handler HandlerFunc, params ...interface{}) (string, error) {
//	name := HandlerName(handler)
//	return r.Reverse(name, params...)
//}

// HandlerName returns string name for given function.
func HandlerName(h HandlerFunc) string {
	// FIXME: check if function creates function (middlewarelike) would name be same but "contents" different??????
	// FIXME: handler := createHandlerFunc("each-time-different-1")
	// FIXME: handler2 := createHandlerFunc("each-time-different-2")
	t := reflect.ValueOf(h).Type()
	if t.Kind() == reflect.Func {
		return runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	}
	return t.String()
}

// Reverse reverses route to URL string by replacing path parameters with given params values.
func (r Routes) Reverse(name string, params ...interface{}) (string, error) {
	for _, rr := range r {
		if rr.Name == name {
			return rr.Reverse(params...), nil
		}
	}
	return "", errors.New("route not found")
}
