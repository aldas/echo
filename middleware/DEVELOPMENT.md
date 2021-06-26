# Development Guidelines for middlewares

## Future-proofing:

As middlewares tend to crow in configuration options it is important to future proof method signatures not to be blocked
by breaking changes in them. These suggestions could seem unnecessary at start but are here to help when new options are
introduced or there is need to add extra validation for configuration values and adding error as return value would be
the breaking change which we want to avoid.

Naming:

* `func MyMw() echo.MiddlewareFunc` - to create middleware with defaults (no arguments). This method should not be able
  to panic as defaults are known during compile time.
* `func MyMwWithConfig(conf MyMwConfig) (echo.MiddlewareFunc, error)` - if middleware creator function takes in
  configuration do not use `panic` to indicate configuration problems, return an error instead. Not everyone wants to
  deal with hidden panics and/or write recovery logic for them.
* `func MustMyMwWithConfig(conf MyMwConfig) echo.MiddlewareFunc` - for convenience of having variant without
  returned `error` prefix middleware creator function name with `Must` and panic if function returns an error.
  `Must` prefix makes it explicitly clear that function can panic.

## Best practices:

* Do not use `panic` in middleware creator functions in case of invalid configuration.
* In case of an error in middleware function handling request avoid using `c.Error()` and returning no error instead
  because previous middlewares up in call chain could have logic for dealing with returned errors.

