package echo

import (
	stdContext "context"
	"crypto/tls"
	"fmt"
	"github.com/labstack/gommon/log"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"io/fs"
	stdLog "log"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	website = "https://echo.labstack.com"
	// http://patorjk.com/software/taag/#p=display&f=Small%20Slant&t=Echo
	banner = `
   ____    __
  / __/___/ /  ___
 / _// __/ _ \/ _ \
/___/\__/_//_/\___/ %s
High performance, minimalist Go web framework
%s
____________________________________O/_______
                                    O\
`
)

// ServerConfig TODO probably there is more suitable name. Fields should be private if methods are way to go.
type ServerConfig struct {
	// Address for the server to listen on (if not using custom listener)
	Address string
	// ListenerNetwork allows setting listener network (see net.Listen for allowed values)
	// Optional: defaults to "tcp"
	ListenerNetwork string

	// CertFilesystem is file system used to load certificates and keys (if certs/keys are given as paths)
	CertFilesystem fs.FS

	// DisableHTTP2 disables supports for HTTP2 in TLS server
	DisableHTTP2 bool
	// HideBanner does not log Echo banner on server startup
	HideBanner bool
	// HidePort does not log port on server startup
	HidePort bool

	// GracefulShutdown is configuration used for graceful shutdown
	GracefulShutdown GracefulShutdownConfig

	// ListenerAddrFunc allows getting listener address before server starts serving requests on listener.
	ListenerAddrFunc func(listener net.Addr)

	// BeforeServeFunc allows customizing/accessing server before server starts serving requests on listener.
	BeforeServeFunc func(server *http.Server) error
}

// GracefulShutdownConfig is graceful shutdown configuration which allows your server to finish serving ongoing active request within given timeout period
type GracefulShutdownConfig struct {
	// Context that completion signals graceful shutdown start
	Context stdContext.Context
	// Timeout is period which server allows listeners to finish serving ongoing requests. If this time is exceeded process is exited
	Timeout time.Duration
	// OnShutdownError allows to customize what happens when server.Shutdown returns an error.
	// Defaults to calling log.Fatal(... err)
	OnShutdownError func(err error)
}

// NewServerConfig creates new ServerConfig instance
func NewServerConfig() *ServerConfig {
	return &ServerConfig{}
}

// WithAddress sets address for ServerConfig
func (sc *ServerConfig) WithAddress(address string) *ServerConfig {
	sc.Address = address
	return sc
}

// WithGracefulShutdownContext sets graceful shutdown context and timeout for ServerConfig
func (sc *ServerConfig) WithGracefulShutdownContext(ctx stdContext.Context, timeout time.Duration) *ServerConfig {
	sc.GracefulShutdown = GracefulShutdownConfig{
		Context: ctx,
		Timeout: timeout,
	}
	return sc
}

// WithCertFilesystem sets filesystem used to load TLS certificate and key for ServerConfig
// Defaults to os.DirFS(".")
func (sc *ServerConfig) WithCertFilesystem(certFilesystem fs.FS) *ServerConfig {
	sc.CertFilesystem = certFilesystem
	return sc
}

// WithBeforeServeFunc is callback function that gives access to server before server.Serve(listener) is called.
func (sc *ServerConfig) WithBeforeServeFunc(before func(s *http.Server) error) *ServerConfig {
	sc.BeforeServeFunc = before
	return sc
}

// WithListenerAddrFunc is callback function that gives access to listener after it is created. Useful for getting port when address was ":0" (random port).
func (sc *ServerConfig) WithListenerAddrFunc(after func(listener net.Addr)) *ServerConfig {
	sc.ListenerAddrFunc = after
	return sc
}

// WithListenerNetwork sets network for listener
// Defaults to "tcp"
func (sc *ServerConfig) WithListenerNetwork(listenerNetwork string) *ServerConfig {
	sc.ListenerNetwork = listenerNetwork
	return sc
}

// WithDisableHTTP2 disables HTTP2 when server is started as TLS server
func (sc *ServerConfig) WithDisableHTTP2(disableHTTP2 bool) *ServerConfig {
	sc.DisableHTTP2 = disableHTTP2
	return sc
}

// WithHideBanner hides banner log message on server startup
func (sc *ServerConfig) WithHideBanner(hideBanner bool) *ServerConfig {
	sc.HideBanner = hideBanner
	return sc
}

// WithHidePort hides port log message on server startup
func (sc *ServerConfig) WithHidePort(hidePort bool) *ServerConfig {
	sc.HidePort = hidePort
	return sc
}

// Start starts a HTTP server.
func (sc *ServerConfig) Start(e *Echo) error {
	logger := e.Logger
	server := http.Server{
		Handler:  e,
		ErrorLog: stdLog.New(logger.Output(), logger.Prefix()+": ", 0),
	}

	listener, err := createListener(sc, nil)
	if err != nil {
		return err
	}
	return serve(sc, &server, listener, logger)
}

// StartTLS starts a HTTPS server.
// If `certFile` or `keyFile` is `string` the values are treated as file paths.
// If `certFile` or `keyFile` is `[]byte` the values are treated as the certificate or key as-is.
func (sc *ServerConfig) StartTLS(e *Echo, certFile, keyFile interface{}) error {
	logger := e.Logger
	s := http.Server{
		Handler:  e,
		ErrorLog: stdLog.New(logger.Output(), logger.Prefix()+": ", 0),
	}

	certFs := sc.CertFilesystem
	if certFs == nil {
		certFs = os.DirFS(".")
	}
	cert, err := filepathOrContent(certFile, certFs)
	if err != nil {
		return err
	}
	key, err := filepathOrContent(keyFile, certFs)
	if err != nil {
		return err
	}
	cer, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return err
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cer}}
	configureTLS(sc, tlsConfig)

	listener, err := createListener(sc, tlsConfig)
	if err != nil {
		return err
	}
	return serve(sc, &s, listener, logger)
}

// StartAutoTLS starts a HTTPS server using certificates automatically installed from https://letsencrypt.org.
func (sc *ServerConfig) StartAutoTLS(e *Echo) error {
	logger := e.Logger
	s := http.Server{
		Handler:  e,
		ErrorLog: stdLog.New(logger.Output(), logger.Prefix()+": ", 0),
	}

	autoTLSManager := autocert.Manager{
		Prompt: autocert.AcceptTOS,
	}
	tlsConfig := &tls.Config{
		GetCertificate: autoTLSManager.GetCertificate,
		NextProtos:     []string{acme.ALPNProto},
	}
	configureTLS(sc, tlsConfig)

	listener, err := createListener(sc, tlsConfig)
	if err != nil {
		return err
	}
	return serve(sc, &s, listener, logger)
}

// StartH2CServer starts a custom http/2 server with h2c (HTTP/2 Cleartext).
func (sc *ServerConfig) StartH2CServer(e *Echo, h2s *http2.Server) error {
	logger := e.Logger
	server := http.Server{
		Handler:  h2c.NewHandler(e, h2s),
		ErrorLog: stdLog.New(logger.Output(), logger.Prefix()+": ", 0),
	}

	listener, err := createListener(sc, nil)
	if err != nil {
		return err
	}
	return serve(sc, &server, listener, logger)
}

func serve(sc *ServerConfig, server *http.Server, listener net.Listener, logger Logger) error {
	if sc.BeforeServeFunc != nil {
		if err := sc.BeforeServeFunc(server); err != nil {
			return err
		}
	}
	startupGreetings(sc, logger, listener)

	if sc.GracefulShutdown.Context != nil {
		go gracefulShutdown(server, sc.GracefulShutdown)
	}
	return server.Serve(listener)
}

func configureTLS(sc *ServerConfig, tlsConfig *tls.Config) {
	if !sc.DisableHTTP2 {
		tlsConfig.NextProtos = append(tlsConfig.NextProtos, "h2")
	}
}

func createListener(sc *ServerConfig, tlsConfig *tls.Config) (net.Listener, error) {
	listenerNetwork := sc.ListenerNetwork
	if sc.ListenerNetwork == "" {
		listenerNetwork = "tcp"
	}

	var listener net.Listener
	var err error
	if tlsConfig != nil {
		listener, err = tls.Listen(listenerNetwork, sc.Address, tlsConfig)
	} else {
		listener, err = net.Listen(listenerNetwork, sc.Address)
	}
	if err != nil {
		return nil, err
	}

	if sc.ListenerAddrFunc != nil {
		sc.ListenerAddrFunc(listener.Addr())
	}
	return listener, nil
}

func startupGreetings(sc *ServerConfig, logger Logger, listener net.Listener) {
	if !sc.HideBanner {
		logger.Printf(banner, "v"+Version, website)
	}

	if !sc.HidePort {
		logger.Printf("⇨ http(s) server started on %s\n", listener.Addr())
	}
}

func filepathOrContent(fileOrContent interface{}, certFilesystem fs.FS) (content []byte, err error) {
	switch v := fileOrContent.(type) {
	case string:
		return fs.ReadFile(certFilesystem, v)
	case []byte:
		return v, nil
	default:
		return nil, ErrInvalidCertOrKeyType
	}
}

func gracefulShutdown(server *http.Server, config GracefulShutdownConfig) {
	<-config.Context.Done() // wait until shutdown context is closed.
	// note: is server if closed by other means this method is still run but is good as no-op

	ctx, cancel := stdContext.WithTimeout(stdContext.Background(), config.Timeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		// we end up here when listeners are not shut down within given timeout
		if config.OnShutdownError != nil {
			config.OnShutdownError(err)
			return
		}
		log.Fatal(fmt.Errorf("failed to shut down server within given timeout: %w", err))
	}
}
