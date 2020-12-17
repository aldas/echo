package middleware

import (
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
)

type (
	// JWTConfig defines the config for JWT middleware.
	JWTConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper Skipper

		// BeforeFunc defines a function which is executed just before the middleware.
		BeforeFunc BeforeFunc

		// SuccessHandler defines a function which is executed for a valid token.
		SuccessHandler JWTSuccessHandler

		// ErrorHandler defines a function which is executed for an invalid token.
		// It may be used to define a custom JWT error.
		ErrorHandler JWTErrorHandler

		// ErrorHandlerWithContext is almost identical to ErrorHandler, but it's passed the current context.
		ErrorHandlerWithContext JWTErrorHandlerWithContext

		// Signing key to validate token. Used as fallback if SigningKeys has length 0.
		// Required. This or SigningKeys.
		SigningKey interface{}

		// Map of signing keys to validate token with kid field usage.
		// Required. This or SigningKey.
		SigningKeys map[string]interface{}

		// Signing method, used to check token signing method.
		// Optional. Default value HS256.
		SigningMethod string

		// KeyFunc is custom method to return signing key during token validation by TokenParser
		// Optional. If not set middleware will use default implementation that checks if token algorithm matches singing
		//           method and returns matching singing key for kid from SigningKeys
		// Both `alg` (https://tools.ietf.org/html/rfc7515#section-4.1.1)
		// and `kid` (https://tools.ietf.org/html/rfc7515#section-4.1.4) are JWT header parameter values
		KeyFunc func(alg string, kid interface{}) (interface{}, error)

		// Context key to store user information from the token into context.
		// Optional. Default value "user".
		ContextKey string

		// TokenLookup is a string in the form of "<source>:<name>" that is used
		// to extract token from the request.
		// Optional. Default value "header:Authorization".
		// Possible values:
		// - "header:<name>"
		// - "query:<name>"
		// - "param:<name>"
		// - "cookie:<name>"
		// - "form:<name>"
		TokenLookup string

		// AuthScheme to be used in the Authorization header.
		// Optional. Default value "Bearer".
		AuthScheme string

		// TokenParser is wrapper interface for different JWT token parsing implementations.
		// Required.
		TokenParser JwtTokenParser
	}

	// JWTSuccessHandler defines a function which is executed for a valid token.
	JWTSuccessHandler func(echo.Context)

	// JWTErrorHandler defines a function which is executed for an invalid token.
	JWTErrorHandler func(error) error

	// JWTErrorHandlerWithContext is almost identical to JWTErrorHandler, but it's passed the current context.
	JWTErrorHandlerWithContext func(error, echo.Context) error

	jwtExtractor func(echo.Context) (string, error)
)

// Algorithms
const (
	AlgorithmHS256 = "HS256"
)

// Errors
var (
	ErrJWTMissing = echo.NewHTTPError(http.StatusBadRequest, "missing or malformed jwt")
	ErrJWTInvalid = echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired jwt")
)

var (
	// DefaultJWTConfig is the default JWT auth middleware config.
	DefaultJWTConfig = JWTConfig{
		Skipper:       DefaultSkipper,
		SigningMethod: AlgorithmHS256,
		ContextKey:    "user",
		TokenLookup:   "header:" + echo.HeaderAuthorization,
		AuthScheme:    "Bearer",
	}
)

// JWT returns a JSON Web Token (JWT) auth middleware.
//
// For valid token, it sets the user in context and calls next handler.
// For invalid token, it returns "401 - Unauthorized" error.
// For missing token, it returns "400 - Bad Request" error.
//
// See: https://jwt.io/introduction
// See `JWTConfig.TokenLookup`
func JWT(key interface{}) echo.MiddlewareFunc {
	c := DefaultJWTConfig
	c.SigningKey = key
	return JWTWithConfig(c)
}

// JWTWithConfig returns a JWT auth middleware with config.
// See: `JWT()`.
func JWTWithConfig(config JWTConfig) echo.MiddlewareFunc {
	if config.SigningKey == nil && len(config.SigningKeys) == 0 {
		panic("echo: jwt middleware requires signing key")
	}
	if config.TokenParser == nil {
		panic("echo: jwt middleware requires token parser instance")
	}
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultJWTConfig.Skipper
	}
	if config.SigningMethod == "" {
		config.SigningMethod = DefaultJWTConfig.SigningMethod
	}
	if config.ContextKey == "" {
		config.ContextKey = DefaultJWTConfig.ContextKey
	}
	if config.TokenLookup == "" {
		config.TokenLookup = DefaultJWTConfig.TokenLookup
	}
	if config.AuthScheme == "" {
		config.AuthScheme = DefaultJWTConfig.AuthScheme
	}
	if config.KeyFunc == nil {
		config.KeyFunc = func(alg string, kid interface{}) (interface{}, error) {
			// signature algorithm used for token must match our signing method
			if alg != config.SigningMethod {
				return nil, fmt.Errorf("unexpected jwt signing method=%s", alg)
			}
			if len(config.SigningKeys) == 0 {
				return config.SigningKey, nil
			}
			kidStr, ok := kid.(string)
			if !ok {
				return nil, errors.New("failed to cast jwt key id as string")
			}
			if key, ok := config.SigningKeys[kidStr]; ok {
				return key, nil
			}
			return nil, fmt.Errorf("unexpected jwt key id=%v", kid)
		}
	}

	// Initialize
	parts := strings.Split(config.TokenLookup, ":")
	extractor := jwtFromHeader(parts[1], config.AuthScheme)
	switch parts[0] {
	case "query":
		extractor = jwtFromQuery(parts[1])
	case "param":
		extractor = jwtFromParam(parts[1])
	case "cookie":
		extractor = jwtFromCookie(parts[1])
	case "form":
		extractor = jwtFromForm(parts[1])
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			if config.BeforeFunc != nil {
				config.BeforeFunc(c)
			}

			auth, err := extractor(c)
			if err != nil {
				if config.ErrorHandler != nil {
					return config.ErrorHandler(err)
				}

				if config.ErrorHandlerWithContext != nil {
					return config.ErrorHandlerWithContext(err, c)
				}
				return err
			}
			token, err := config.TokenParser.Parse(auth, config)
			if err == nil {
				// Store user information from token into context.
				c.Set(config.ContextKey, token)
				if config.SuccessHandler != nil {
					config.SuccessHandler(c)
				}
				return next(c)
			}
			if config.ErrorHandler != nil {
				return config.ErrorHandler(err)
			}
			if config.ErrorHandlerWithContext != nil {
				return config.ErrorHandlerWithContext(err, c)
			}
			return &echo.HTTPError{
				Code:     ErrJWTInvalid.Code,
				Message:  ErrJWTInvalid.Message,
				Internal: err,
			}
		}
	}
}

// JwtTokenParser is wrapper interface for different JWT token parsing implementations.
type JwtTokenParser interface {
	// Parse parses token string to token instance that is set to echo.Context under JWTConfig.ContextKey
	// Must return error when parsing failed, token is not valid or otherwise incorrect
	Parse(tokenString string, config JWTConfig) (interface{}, error)
}

// jwtFromHeader returns a `jwtExtractor` that extracts token from the request header.
func jwtFromHeader(header string, authScheme string) jwtExtractor {
	return func(c echo.Context) (string, error) {
		auth := c.Request().Header.Get(header)
		l := len(authScheme)
		if len(auth) > l+1 && auth[:l] == authScheme {
			return auth[l+1:], nil
		}
		return "", ErrJWTMissing
	}
}

// jwtFromQuery returns a `jwtExtractor` that extracts token from the query string.
func jwtFromQuery(param string) jwtExtractor {
	return func(c echo.Context) (string, error) {
		token := c.QueryParam(param)
		if token == "" {
			return "", ErrJWTMissing
		}
		return token, nil
	}
}

// jwtFromParam returns a `jwtExtractor` that extracts token from the url param string.
func jwtFromParam(param string) jwtExtractor {
	return func(c echo.Context) (string, error) {
		token := c.Param(param)
		if token == "" {
			return "", ErrJWTMissing
		}
		return token, nil
	}
}

// jwtFromCookie returns a `jwtExtractor` that extracts token from the named cookie.
func jwtFromCookie(name string) jwtExtractor {
	return func(c echo.Context) (string, error) {
		cookie, err := c.Cookie(name)
		if err != nil {
			return "", ErrJWTMissing
		}
		return cookie.Value, nil
	}
}

// jwtFromForm returns a `jwtExtractor` that extracts token from the form field.
func jwtFromForm(name string) jwtExtractor {
	return func(c echo.Context) (string, error) {
		field := c.FormValue(name)
		if field == "" {
			return "", ErrJWTMissing
		}
		return field, nil
	}
}
