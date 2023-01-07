package middleware

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// MethodOverrideConfig defines the config for MethodOverride middleware.
type MethodOverrideConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper Skipper

	// Getter is a function that gets overridden method from the request.
	// Optional. Default values MethodFromHeader(echo.HeaderXHTTPMethodOverride).
	Getter MethodOverrideGetter
}

// MethodOverrideGetter is a function that gets overridden method from the request
type MethodOverrideGetter func(*echo.Context) string

// DefaultMethodOverrideConfig is the default MethodOverride middleware config.
var DefaultMethodOverrideConfig = MethodOverrideConfig{
	Skipper: DefaultSkipper,
	Getter:  MethodFromHeader(echo.HeaderXHTTPMethodOverride),
}

// MethodOverride returns a MethodOverride middleware.
// MethodOverride  middleware checks for the overridden method from the request and
// uses it instead of the original method.
//
// For security reasons, only `POST` method can be overridden.
func MethodOverride() echo.MiddlewareFunc {
	return MethodOverrideWithConfig(DefaultMethodOverrideConfig)
}

// MethodOverrideWithConfig returns a Method Override middleware with config or panics on invalid configuration.
func MethodOverrideWithConfig(config MethodOverrideConfig) echo.MiddlewareFunc {
	return toMiddlewareOrPanic(config)
}

// ToMiddleware converts MethodOverrideConfig to middleware or returns an error for invalid configuration
func (config MethodOverrideConfig) ToMiddleware() (echo.MiddlewareFunc, error) {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultMethodOverrideConfig.Skipper
	}
	if config.Getter == nil {
		config.Getter = DefaultMethodOverrideConfig.Getter
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			req := c.Request()
			if req.Method == http.MethodPost {
				m := config.Getter(c)
				if m != "" {
					req.Method = m
				}
			}
			return next(c)
		}
	}, nil
}

// MethodFromHeader is a `MethodOverrideGetter` that gets overridden method from
// the request header.
func MethodFromHeader(header string) MethodOverrideGetter {
	return func(c *echo.Context) string {
		return c.Request().Header.Get(header)
	}
}

// MethodFromForm is a `MethodOverrideGetter` that gets overridden method from the
// form parameter.
func MethodFromForm(param string) MethodOverrideGetter {
	return func(c *echo.Context) string {
		return c.FormValue(param)
	}
}

// MethodFromQuery is a `MethodOverrideGetter` that gets overridden method from
// the query parameter.
func MethodFromQuery(param string) MethodOverrideGetter {
	return func(c *echo.Context) string {
		return c.QueryParam(param)
	}
}
