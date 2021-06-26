package middleware

import (
	"github.com/labstack/echo/v4"
)

// RequestIDConfig defines the config for RequestID middleware.
type RequestIDConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper Skipper

	// Generator defines a function to generate an ID.
	// Optional. Default value random.String(32).
	Generator func() string
}

// RequestID returns a X-Request-ID middleware.
func RequestID() echo.MiddlewareFunc {
	mw, err := RequestIDWithConfig(RequestIDConfig{})
	if err != nil {
		panic(err)
	}
	return mw
}

// MustRequestIDWithConfig returns a X-Request-ID middleware with config or panics on invalid configuration.
func MustRequestIDWithConfig(config RequestIDConfig) echo.MiddlewareFunc {
	mw, err := RequestIDWithConfig(config)
	if err != nil {
		panic(err)
	}
	return mw
}

// RequestIDWithConfig returns a X-Request-ID middleware with config.
func RequestIDWithConfig(config RequestIDConfig) (echo.MiddlewareFunc, error) {
	if config.Skipper == nil {
		config.Skipper = DefaultSkipper
	}
	if config.Generator == nil {
		config.Generator = createRandomStringGenerator(32)
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			req := c.Request()
			res := c.Response()
			rid := req.Header.Get(echo.HeaderXRequestID)
			if rid == "" {
				rid = config.Generator()
			}
			res.Header().Set(echo.HeaderXRequestID, rid)

			return next(c)
		}
	}, nil
}
