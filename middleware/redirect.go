package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// RedirectConfig defines the config for Redirect middleware.
type RedirectConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper

	// Status code to be used when redirecting the request.
	// Optional. Default value http.StatusMovedPermanently.
	Code int `yaml:"code"`
}

// redirectLogic represents a function that given a scheme, host and uri
// can both: 1) determine if redirect is needed (will set ok accordingly) and
// 2) return the appropriate redirect url.
type redirectLogic func(scheme, host, uri string) (ok bool, url string)

const www = "www."

// DefaultRedirectConfig is the default Redirect middleware config.
var DefaultRedirectConfig = RedirectConfig{
	Skipper: DefaultSkipper,
	Code:    http.StatusMovedPermanently,
}

// HTTPSRedirect redirects http requests to https.
// For example, http://labstack.com will be redirect to https://labstack.com.
//
// Usage `Echo#Pre(HTTPSRedirect())`
func HTTPSRedirect() echo.MiddlewareFunc {
	return MustHTTPSRedirectWithConfig(DefaultRedirectConfig)
}

// MustHTTPSRedirectWithConfig returns a HTTPS redirect middleware with config or panics on invalid configuration.
func MustHTTPSRedirectWithConfig(config RedirectConfig) echo.MiddlewareFunc {
	mw, err := HTTPSRedirectWithConfig(config)
	if err != nil {
		panic(err)
	}
	return mw
}

// HTTPSRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `HTTPSRedirect()`.
func HTTPSRedirectWithConfig(config RedirectConfig) (echo.MiddlewareFunc, error) {
	return redirect(config, func(scheme, host, uri string) (bool, string) {
		if scheme != "https" {
			return true, "https://" + host + uri
		}
		return false, ""
	})
}

// HTTPSWWWRedirect redirects http requests to https www.
// For example, http://labstack.com will be redirect to https://www.labstack.com.
//
// Usage `Echo#Pre(HTTPSWWWRedirect())`
func HTTPSWWWRedirect() echo.MiddlewareFunc {
	return MustHTTPSWWWRedirectWithConfig(DefaultRedirectConfig)
}

// MustHTTPSWWWRedirectWithConfig returns a HTTPS WWW redirect middleware with config or panics on invalid configuration.
func MustHTTPSWWWRedirectWithConfig(config RedirectConfig) echo.MiddlewareFunc {
	mw, err := HTTPSWWWRedirectWithConfig(config)
	if err != nil {
		panic(err)
	}
	return mw
}

// HTTPSWWWRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `HTTPSWWWRedirect()`.
func HTTPSWWWRedirectWithConfig(config RedirectConfig) (echo.MiddlewareFunc, error) {
	return redirect(config, func(scheme, host, uri string) (bool, string) {
		if scheme != "https" && !strings.HasPrefix(host, www) {
			return true, "https://www." + host + uri
		}
		return false, ""
	})
}

// HTTPSNonWWWRedirect redirects http requests to https non www.
// For example, http://www.labstack.com will be redirect to https://labstack.com.
//
// Usage `Echo#Pre(HTTPSNonWWWRedirect())`
func HTTPSNonWWWRedirect() echo.MiddlewareFunc {
	return MustHTTPSNonWWWRedirectWithConfig(DefaultRedirectConfig)
}

// MustHTTPSNonWWWRedirectWithConfig returns a HTTPS Non-WWW redirect middleware with config or panics on invalid configuration.
func MustHTTPSNonWWWRedirectWithConfig(config RedirectConfig) echo.MiddlewareFunc {
	mw, err := HTTPSNonWWWRedirectWithConfig(config)
	if err != nil {
		panic(err)
	}
	return mw
}

// HTTPSNonWWWRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `HTTPSNonWWWRedirect()`.
func HTTPSNonWWWRedirectWithConfig(config RedirectConfig) (echo.MiddlewareFunc, error) {
	return redirect(config, func(scheme, host, uri string) (ok bool, url string) {
		if scheme != "https" {
			host = strings.TrimPrefix(host, www)
			return true, "https://" + host + uri
		}
		return false, ""
	})
}

// WWWRedirect redirects non www requests to www.
// For example, http://labstack.com will be redirect to http://www.labstack.com.
//
// Usage `Echo#Pre(WWWRedirect())`
func WWWRedirect() echo.MiddlewareFunc {
	return MustWWWRedirectWithConfig(DefaultRedirectConfig)
}

// MustWWWRedirectWithConfig returns a WWW redirect middleware with config or panics on invalid configuration.
func MustWWWRedirectWithConfig(config RedirectConfig) echo.MiddlewareFunc {
	mw, err := WWWRedirectWithConfig(config)
	if err != nil {
		panic(err)
	}
	return mw
}

// WWWRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `WWWRedirect()`.
func WWWRedirectWithConfig(config RedirectConfig) (echo.MiddlewareFunc, error) {
	return redirect(config, func(scheme, host, uri string) (bool, string) {
		if !strings.HasPrefix(host, www) {
			return true, scheme + "://www." + host + uri
		}
		return false, ""
	})
}

// NonWWWRedirect redirects www requests to non www.
// For example, http://www.labstack.com will be redirect to http://labstack.com.
//
// Usage `Echo#Pre(NonWWWRedirect())`
func NonWWWRedirect() echo.MiddlewareFunc {
	return MustNonWWWRedirectWithConfig(DefaultRedirectConfig)
}

// MustNonWWWRedirectWithConfig returns a Non-WWW redirect middleware with config or panics on invalid configuration.
func MustNonWWWRedirectWithConfig(config RedirectConfig) echo.MiddlewareFunc {
	mw, err := NonWWWRedirectWithConfig(config)
	if err != nil {
		panic(err)
	}
	return mw
}

// NonWWWRedirectWithConfig returns an HTTPSRedirect middleware with config.
// See `NonWWWRedirect()`.
func NonWWWRedirectWithConfig(config RedirectConfig) (echo.MiddlewareFunc, error) {
	return redirect(config, func(scheme, host, uri string) (bool, string) {
		if strings.HasPrefix(host, www) {
			return true, scheme + "://" + host[4:] + uri
		}
		return false, ""
	})
}

func redirect(config RedirectConfig, cb redirectLogic) (echo.MiddlewareFunc, error) {
	if config.Skipper == nil {
		config.Skipper = DefaultRedirectConfig.Skipper
	}
	if config.Code == 0 {
		config.Code = DefaultRedirectConfig.Code
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			req, scheme := c.Request(), c.Scheme()
			host := req.Host
			if ok, url := cb(scheme, host, req.RequestURI); ok {
				return c.Redirect(config.Code, url)
			}

			return next(c)
		}
	}, nil
}
