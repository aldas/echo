package echo

import (
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
)

var myNamedHandler = func(c Context) error {
	return nil
}

type NameStruct struct {
}

func (n *NameStruct) getUsers(c Context) error {
	return nil
}

func TestHandlerName(t *testing.T) {
	myNameFuncVar := func(c Context) error {
		return nil
	}

	tmp := NameStruct{}

	var testCases = []struct {
		name            string
		whenHandlerFunc HandlerFunc
		expect          string
	}{
		{
			name: "ok, func as anonymous func",
			whenHandlerFunc: func(c Context) error {
				return nil
			},
			expect: "github.com/labstack/echo/v4.TestHandlerName.func2",
		},
		{
			name:            "ok, func as named package variable",
			whenHandlerFunc: myNamedHandler,
			expect:          "github.com/labstack/echo/v4.glob..func3",
		},
		{
			name:            "ok, func as named function variable",
			whenHandlerFunc: myNameFuncVar,
			expect:          "github.com/labstack/echo/v4.TestHandlerName.func1",
		},
		{
			name:            "ok, func as struct method",
			whenHandlerFunc: tmp.getUsers,
			expect:          "github.com/labstack/echo/v4.(*NameStruct).getUsers-fm",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name := HandlerName(tc.whenHandlerFunc)
			assert.Equal(t, tc.expect, name)
		})
	}
}

func TestHandlerName_differentFuncSameName(t *testing.T) {
	handlerCreator := func(name string) HandlerFunc {
		return func(c Context) error {
			return c.String(http.StatusTeapot, name)
		}
	}
	h1 := handlerCreator("name1")
	assert.Equal(t, "github.com/labstack/echo/v4.TestHandlerName_differentFuncSameName.func1.1", HandlerName(h1))

	h2 := handlerCreator("name2")
	assert.Equal(t, "github.com/labstack/echo/v4.TestHandlerName_differentFuncSameName.func1.1", HandlerName(h2))
}
