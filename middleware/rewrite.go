package middleware

import (
	"errors"
	"regexp"

	"github.com/labstack/echo/v4"
)

// RewriteConfig defines the config for Rewrite middleware.
type RewriteConfig struct {
	// Skipper defines a function to skip middleware.
	Skipper Skipper

	// Rules defines the URL path rewrite rules. The values captured in asterisk can be
	// retrieved by index e.g. $1, $2 and so on.
	// Example:
	// "/old":              "/new",
	// "/api/*":            "/$1",
	// "/js/*":             "/public/javascripts/$1",
	// "/users/*/orders/*": "/user/$1/order/$2",
	// Required.
	Rules map[string]string `yaml:"rules"`

	// RegexRules defines the URL path rewrite rules using regexp.Rexexp with captures
	// Every capture group in the values can be retrieved by index e.g. $1, $2 and so on.
	// Example:
	// "^/old/[0.9]+/":     "/new",
	// "^/api/.+?/(.*)":     "/v2/$1",
	RegexRules map[*regexp.Regexp]string `yaml:"regex_rules"`
}

// DefaultRewriteConfig is the default Rewrite middleware config.
var DefaultRewriteConfig = RewriteConfig{
	Skipper: DefaultSkipper,
}

// MustRewriteWithConfig returns a Rewrite middleware or panics on invalid configuration.
//
// Rewrite middleware rewrites the URL path based on the provided rules.
func MustRewriteWithConfig(config RewriteConfig) echo.MiddlewareFunc {
	mw, err := RewriteWithConfig(config)
	if err != nil {
		panic(err)
	}
	return mw
}

// RewriteWithConfig returns a Rewrite middleware with config.
func RewriteWithConfig(config RewriteConfig) (echo.MiddlewareFunc, error) {
	if config.Skipper == nil {
		config.Skipper = DefaultRewriteConfig.Skipper
	}
	if config.Rules == nil && config.RegexRules == nil {
		return nil, errors.New("echo rewrite middleware requires url path rewrite rules or regex rules")
	}

	if config.RegexRules == nil {
		config.RegexRules = make(map[*regexp.Regexp]string)
	}
	for k, v := range rewriteRulesRegex(config.Rules) {
		config.RegexRules[k] = v
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			if config.Skipper(c) {
				return next(c)
			}

			if err := rewriteURL(config.RegexRules, c.Request()); err != nil {
				return err
			}
			return next(c)
		}
	}, nil
}
