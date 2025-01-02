// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package echo

import (
	stdContext "context"
	"crypto/tls"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	banner = "Echo (v%s). High performance, minimalist Go web framework https://echo.labstack.com"
)

// StartConfig is for creating configured http.Server instance to start serve http(s) requests with given Echo instance
type StartConfig struct {
	Address string

	HideBanner bool
	HidePort   bool

	CertFilesystem fs.FS
	TLSConfig      *tls.Config

	ListenerNetwork  string
	ListenerAddrFunc func(addr net.Addr)

	GracefulContext stdContext.Context
	GracefulTimeout time.Duration

	BeforeServeFunc func(s *http.Server) error
	OnShutdownError func(err error)
}

// Start starts a HTTP(s) server.
func (sc StartConfig) Start(e *Echo) error {
	return sc.start(e)
}

// StartTLS starts a HTTPS server.
// If `certFile` or `keyFile` is `string` the values are treated as file paths.
// If `certFile` or `keyFile` is `[]byte` the values are treated as the certificate or key as-is.
func (sc StartConfig) StartTLS(e *Echo, certFile, keyFile any) error {
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
	if sc.TLSConfig == nil {
		sc.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{"h2"},
			//NextProtos: []string{"http/1.1"}, // Disallow "h2", allow http
		}
	}
	sc.TLSConfig.Certificates = []tls.Certificate{cer}
	return sc.start(e)
}

// start starts a HTTP(s) server.
func (sc StartConfig) start(e *Echo) error {
	logger := e.Logger
	server := http.Server{
		Handler:  e,
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
		// defaults for GoSec rule G112 // https://github.com/securego/gosec
		// G112 (CWE-400): Potential Slowloris Attack because ReadHeaderTimeout is not configured in the http.Server
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	listenerNetwork := sc.ListenerNetwork
	if listenerNetwork == "" {
		listenerNetwork = "tcp"
	}
	var listener net.Listener
	var err error
	if sc.TLSConfig != nil {
		listener, err = tls.Listen(listenerNetwork, sc.Address, sc.TLSConfig)
	} else {
		listener, err = net.Listen(listenerNetwork, sc.Address)
	}
	if err != nil {
		return err
	}
	if sc.ListenerAddrFunc != nil {
		sc.ListenerAddrFunc(listener.Addr())
	}

	if sc.BeforeServeFunc != nil {
		if err := sc.BeforeServeFunc(&server); err != nil {
			return err
		}
	}
	if !sc.HideBanner {
		bannerText := fmt.Sprintf(banner, Version)
		logger.Info(bannerText)
	}
	if !sc.HidePort {
		logger.Info("http(s) server started", "address", listener.Addr())
	}

	if sc.GracefulContext != nil {
		ctx, cancel := stdContext.WithCancel(sc.GracefulContext)
		defer cancel() // make sure this graceful coroutine will end when serve returns by some other means
		go gracefulShutdown(ctx, &sc, &server, logger)
	}
	return server.Serve(listener)
}

func filepathOrContent(fileOrContent any, certFilesystem fs.FS) (content []byte, err error) {
	switch v := fileOrContent.(type) {
	case string:
		return fs.ReadFile(certFilesystem, v)
	case []byte:
		return v, nil
	default:
		return nil, ErrInvalidCertOrKeyType
	}
}

func gracefulShutdown(gracefulCtx stdContext.Context, sc *StartConfig, server *http.Server, logger *slog.Logger) {
	<-gracefulCtx.Done() // wait until shutdown context is closed.
	// note: is server if closed by other means this method is still run but is good as no-op

	timeout := sc.GracefulTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	shutdownCtx, cancel := stdContext.WithTimeout(stdContext.Background(), timeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		// we end up here when listeners are not shut down within given timeout
		if sc.OnShutdownError != nil {
			sc.OnShutdownError(err)
			return
		}
		logger.Error("failed to shut down server within given timeout", "error", err)
	}
}
