package echo

import (
	"bytes"
	stdContext "context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/labstack/gommon/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func startOnRandomPort(ctx stdContext.Context, e *Echo) (string, error) {
	addrChan := make(chan string)
	errCh := make(chan error)

	go func() {
		errCh <- NewServerConfig().
			WithAddress(":0").
			WithGracefulShutdownContext(ctx, 100*time.Millisecond).
			WithListenerAddrFunc(func(listener net.Addr) {
				addrChan <- listener.String()
			}).
			Start(e)
	}()

	return waitForServerStart(addrChan, errCh)
}

func waitForServerStart(addrChan <-chan string, errCh <-chan error) (string, error) {
	waitCtx, cancel := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
	defer cancel()

	// wait for addr to arrive
	for {
		select {
		case <-waitCtx.Done():
			return "", waitCtx.Err()
		case addr := <-addrChan:
			return addr, nil
		case err := <-errCh:
			if err == http.ErrServerClosed { // was closed normally before listener callback was called. should not be possible
				return "", nil
			}
			// failed to start and we did not manage to get even listener part.
			return "", err
		}
	}
}

func doGet(url string) (int, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, "", err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, string(body), nil
}

func TestEchoStart(t *testing.T) {
	e := New()
	e.GET("/ok", func(c Context) error {
		return c.String(http.StatusOK, "OK")
	})

	addrChan := make(chan string)
	errCh := make(chan error)

	ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
	defer shutdown()
	go func() {
		errCh <- NewServerConfig().
			WithAddress(":0").
			WithGracefulShutdownContext(ctx, 100*time.Millisecond).
			WithListenerAddrFunc(func(listener net.Addr) {
				addrChan <- listener.String()
			}).
			Start(e)
	}()

	addr, err := waitForServerStart(addrChan, errCh)
	assert.NoError(t, err)

	// check if server is actually up
	code, body, err := doGet(fmt.Sprintf("http://%v/ok", addr))
	if err != nil {
		assert.NoError(t, err)
		return
	}
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "OK", body)

	shutdown()

	<-errCh // we will be blocking here until server returns from http.Serve

	// check if server was stopped
	code, body, err = doGet(fmt.Sprintf("http://%v/ok", addr))
	assert.True(t, strings.Contains(err.Error(), "connect: connection refused"))
	assert.Equal(t, 0, code)
	assert.Equal(t, "", body)
}

func TestEcho_StartTLS(t *testing.T) {
	var testCases = []struct {
		name        string
		addr        string
		certFile    string
		keyFile     string
		expectError string
	}{
		{
			name: "ok",
			addr: ":0",
		},
		{
			name:        "nok, invalid certFile",
			addr:        ":0",
			certFile:    "not existing",
			expectError: "open not existing: no such file or directory",
		},
		{
			name:        "nok, invalid keyFile",
			addr:        ":0",
			keyFile:     "not existing",
			expectError: "open not existing: no such file or directory",
		},
		{
			name:        "nok, failed to create cert out of certFile and keyFile",
			addr:        ":0",
			keyFile:     "_fixture/certs/cert.pem", // we are passing cert instead of key
			expectError: "tls: found a certificate rather than a key in the PEM for the private key",
		},
		{
			name:        "nok, invalid tls address",
			addr:        "nope",
			expectError: "listen tcp: address nope: missing port in address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := New()

			addrChan := make(chan string)
			errCh := make(chan error)

			ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
			defer shutdown()
			go func() {
				certFile := "_fixture/certs/cert.pem"
				if tc.certFile != "" {
					certFile = tc.certFile
				}
				keyFile := "_fixture/certs/key.pem"
				if tc.keyFile != "" {
					keyFile = tc.keyFile
				}

				errCh <- NewServerConfig().
					WithAddress(tc.addr).
					WithGracefulShutdownContext(ctx, 100*time.Millisecond).
					WithListenerAddrFunc(func(listener net.Addr) {
						addrChan <- listener.String()
					}).
					StartTLS(e, certFile, keyFile)
			}()

			_, err := waitForServerStart(addrChan, errCh)

			if tc.expectError != "" {
				if _, ok := err.(*os.PathError); ok {
					assert.Error(t, err) // error messages for unix and windows are different. so name only error type here
				} else {
					assert.EqualError(t, err, tc.expectError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEchoStartTLSAndStart(t *testing.T) {
	// We name if Echo and listeners work correctly when Echo is simultaneously attached to HTTP and HTTPS server
	e := New()
	e.GET("/", func(c Context) error {
		return c.String(http.StatusOK, "OK")
	})

	tlsCtx, tlsShutdown := stdContext.WithTimeout(stdContext.Background(), 100*time.Millisecond)
	defer tlsShutdown()
	addrTLSChan := make(chan string)
	errTLSChan := make(chan error)
	go func() {
		certFile := "_fixture/certs/cert.pem"
		keyFile := "_fixture/certs/key.pem"
		errTLSChan <- NewServerConfig().
			WithAddress(":0").
			WithGracefulShutdownContext(tlsCtx, 100*time.Millisecond).
			WithListenerAddrFunc(func(listener net.Addr) {
				addrTLSChan <- listener.String()
			}).
			StartTLS(e, certFile, keyFile)
	}()

	tlsAddr, err := waitForServerStart(addrTLSChan, errTLSChan)
	assert.NoError(t, err)

	// check if HTTPS works (note: we are using self signed certs so InsecureSkipVerify=true)
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	res, err := client.Get(fmt.Sprintf("https://%v", tlsAddr))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 100*time.Millisecond)
	defer shutdown()
	addrChan := make(chan string)
	errChan := make(chan error)
	go func() {
		errChan <- NewServerConfig().
			WithAddress(":0").
			WithGracefulShutdownContext(ctx, 100*time.Millisecond).
			WithListenerAddrFunc(func(listener net.Addr) {
				addrChan <- listener.String()
			}).
			Start(e)
	}()

	addr, err := waitForServerStart(addrChan, errChan)
	assert.NoError(t, err)

	// now we are serving both HTTPS and HTTP listeners. see if HTTP works in addition to HTTPS
	res, err = client.Get(fmt.Sprintf("http://%v", addr))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)

	// see if HTTPS works after HTTP listener is also added
	res, err = client.Get(fmt.Sprintf("https://%v", tlsAddr))
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, res.StatusCode)
}

func TestFilepathOrContent(t *testing.T) {
	cert, err := ioutil.ReadFile("_fixture/certs/cert.pem")
	require.NoError(t, err)
	key, err := ioutil.ReadFile("_fixture/certs/key.pem")
	require.NoError(t, err)

	testCases := []struct {
		name        string
		cert        interface{}
		key         interface{}
		expectedErr error
	}{
		{
			name:        `ValidCertAndKeyFilePath`,
			cert:        "_fixture/certs/cert.pem",
			key:         "_fixture/certs/key.pem",
			expectedErr: nil,
		},
		{
			name:        `ValidCertAndKeyByteString`,
			cert:        cert,
			key:         key,
			expectedErr: nil,
		},
		{
			name:        `InvalidKeyType`,
			cert:        cert,
			key:         1,
			expectedErr: ErrInvalidCertOrKeyType,
		},
		{
			name:        `InvalidCertType`,
			cert:        0,
			key:         key,
			expectedErr: ErrInvalidCertOrKeyType,
		},
		{
			name:        `InvalidCertAndKeyTypes`,
			cert:        0,
			key:         1,
			expectedErr: ErrInvalidCertOrKeyType,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := New()

			addrChan := make(chan string)
			errCh := make(chan error)

			ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
			defer shutdown()

			go func() {
				errCh <- NewServerConfig().
					WithAddress(":0").
					WithCertFilesystem(os.DirFS(".")).
					WithGracefulShutdownContext(ctx, 100*time.Millisecond).
					WithListenerAddrFunc(func(listener net.Addr) {
						addrChan <- listener.String()
					}).
					StartTLS(e, tc.cert, tc.key)
			}()

			_, err := waitForServerStart(addrChan, errCh)
			if tc.expectedErr != nil {
				assert.EqualError(t, err, tc.expectedErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEcho_StartAutoTLS(t *testing.T) {
	var testCases = []struct {
		name        string
		addr        string
		expectError string
	}{
		{
			name: "ok",
			addr: ":0",
		},
		{
			name:        "nok, invalid address",
			addr:        "nope",
			expectError: "listen tcp: address nope: missing port in address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := New()
			addrChan := make(chan string)
			errCh := make(chan error)

			ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
			defer shutdown()

			go func() {
				errCh <- NewServerConfig().
					WithAddress(tc.addr).
					WithGracefulShutdownContext(ctx, 100*time.Millisecond).
					WithListenerAddrFunc(func(listener net.Addr) {
						addrChan <- listener.String()
					}).
					StartAutoTLS(e)
			}()

			_, err := waitForServerStart(addrChan, errCh)
			if tc.expectError != "" {
				assert.EqualError(t, err, tc.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEcho_StartH2CServer(t *testing.T) {
	var testCases = []struct {
		name        string
		addr        string
		expectError string
	}{
		{
			name: "ok",
			addr: ":0",
		},
		{
			name:        "nok, invalid address",
			addr:        "nope",
			expectError: "listen tcp: address nope: missing port in address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := New()
			addrChan := make(chan string)
			errCh := make(chan error)

			ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
			defer shutdown()

			go func() {
				h2s := &http2.Server{}

				errCh <- NewServerConfig().
					WithAddress(tc.addr).
					WithGracefulShutdownContext(ctx, 100*time.Millisecond).
					WithListenerAddrFunc(func(listener net.Addr) {
						addrChan <- listener.String()
					}).
					StartH2CServer(e, h2s)
			}()

			_, err := waitForServerStart(addrChan, errCh)
			if tc.expectError != "" {
				assert.EqualError(t, err, tc.expectError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func supportsIPv6() bool {
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		// Check if any interface has local IPv6 assigned
		if strings.Contains(addr.String(), "::1") {
			return true
		}
	}
	return false
}

func TestWithListenerNetwork(t *testing.T) {
	testCases := []struct {
		name    string
		network string
		address string
	}{
		{
			name:    "tcp ipv4 address",
			network: "tcp",
			address: "127.0.0.1:1323",
		},
		{
			name:    "tcp ipv6 address",
			network: "tcp",
			address: "[::1]:1323",
		},
		{
			name:    "tcp4 ipv4 address",
			network: "tcp4",
			address: "127.0.0.1:1323",
		},
		{
			name:    "tcp6 ipv6 address",
			network: "tcp6",
			address: "[::1]:1323",
		},
	}

	hasIPv6 := supportsIPv6()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !hasIPv6 && strings.Contains(tc.address, "::") {
				t.Skip("Skipping testing IPv6 for " + tc.address + ", not available")
			}

			e := New()
			e.GET("/ok", func(c Context) error {
				return c.String(http.StatusOK, "OK")
			})

			addrChan := make(chan string)
			errCh := make(chan error)

			ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
			defer shutdown()

			go func() {
				errCh <- NewServerConfig().
					WithAddress(tc.address).
					WithListenerNetwork(tc.network).
					WithGracefulShutdownContext(ctx, 100*time.Millisecond).
					WithListenerAddrFunc(func(listener net.Addr) {
						addrChan <- listener.String()
					}).
					Start(e)
			}()

			_, err := waitForServerStart(addrChan, errCh)
			assert.NoError(t, err)

			code, body, err := doGet(fmt.Sprintf("http://%s/ok", tc.address))
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, code)
			assert.Equal(t, "OK", body)
		})
	}
}

func TestWithHideBanner(t *testing.T) {
	var testCases = []struct {
		name       string
		hideBanner bool
	}{
		{
			name:       "hide banner on startup",
			hideBanner: true,
		},
		{
			name:       "show banner on startup",
			hideBanner: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := New()

			var buf bytes.Buffer
			l := log.New("echo")
			l.SetOutput(&buf)
			e.Logger = l

			e.GET("/ok", func(c Context) error {
				return c.String(http.StatusOK, "OK")
			})

			addrChan := make(chan string)
			errCh := make(chan error)

			ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
			defer shutdown()

			go func() {
				_, err := waitForServerStart(addrChan, errCh)
				errCh <- err
				shutdown()
			}()

			err := NewServerConfig().
				WithAddress(":0").
				WithHideBanner(tc.hideBanner).
				WithGracefulShutdownContext(ctx, 100*time.Millisecond).
				WithListenerAddrFunc(func(listener net.Addr) {
					addrChan <- listener.String()
				}).
				Start(e)
			if err != http.ErrServerClosed {
				assert.NoError(t, err)
			}
			assert.NoError(t, <-errCh)

			contains := strings.Contains(buf.String(), "High performance, minimalist Go web framework")
			if tc.hideBanner {
				assert.False(t, contains)
			} else {
				assert.True(t, contains)
			}
		})
	}
}

func TestWithHidePort(t *testing.T) {
	var testCases = []struct {
		name     string
		hidePort bool
	}{
		{
			name:     "hide port on startup",
			hidePort: true,
		},
		{
			name:     "show port on startup",
			hidePort: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := New()

			var buf bytes.Buffer
			l := log.New("echo")
			l.SetOutput(&buf)
			e.Logger = l

			e.GET("/ok", func(c Context) error {
				return c.String(http.StatusOK, "OK")
			})

			addrChan := make(chan string)
			errCh := make(chan error, 1)

			ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)

			go func() {
				_, err := waitForServerStart(addrChan, errCh)
				errCh <- err
				shutdown()
			}()

			err := NewServerConfig().
				WithAddress(":0").
				WithHidePort(tc.hidePort).
				WithGracefulShutdownContext(ctx, 100*time.Millisecond).
				WithListenerAddrFunc(func(listener net.Addr) {
					addrChan <- listener.String()
				}).
				Start(e)
			if err != http.ErrServerClosed {
				assert.NoError(t, err)
			}
			assert.NoError(t, <-errCh)

			portMsg := fmt.Sprintf("http(s) server started on")
			contains := strings.Contains(buf.String(), portMsg)
			if tc.hidePort {
				assert.False(t, contains)
			} else {
				assert.True(t, contains)
			}
		})
	}
}

func TestWithBeforeServeFunc(t *testing.T) {
	e := New()

	e.GET("/ok", func(c Context) error {
		return c.String(http.StatusOK, "OK")
	})

	err := NewServerConfig().
		WithAddress(":0").
		WithBeforeServeFunc(func(s *http.Server) error {
			return errors.New("is called before serve")
		}).
		Start(e)
	assert.EqualError(t, err, "is called before serve")
}

func TestWithDisableHTTP2(t *testing.T) {
	var testCases = []struct {
		name         string
		disableHTTP2 bool
	}{
		{
			name:         "HTTP2 enabled",
			disableHTTP2: false,
		},
		{
			name:         "HTTP2 disabled",
			disableHTTP2: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			e := New()

			e.GET("/ok", func(c Context) error {
				return c.String(http.StatusOK, "OK")
			})

			addrChan := make(chan string)
			errCh := make(chan error, 1)

			ctx, shutdown := stdContext.WithTimeout(stdContext.Background(), 200*time.Millisecond)
			defer shutdown()

			go func() {
				certFile := "_fixture/certs/cert.pem"
				keyFile := "_fixture/certs/key.pem"

				errCh <- NewServerConfig().
					WithAddress(":0").
					WithDisableHTTP2(tc.disableHTTP2).
					WithGracefulShutdownContext(ctx, 100*time.Millisecond).
					WithListenerAddrFunc(func(listener net.Addr) {
						addrChan <- listener.String()
					}).
					StartTLS(e, certFile, keyFile)
			}()

			addr, err := waitForServerStart(addrChan, errCh)
			assert.NoError(t, err)

			url := fmt.Sprintf("https://%v/ok", addr)

			// do ordinary http(s) request
			client := &http.Client{Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}}
			res, err := client.Get(url)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusOK, res.StatusCode)

			// do HTTP2 request
			client.Transport = &http2.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			resp, err := client.Get(url)
			if err != nil {
				if tc.disableHTTP2 {
					assert.True(t, strings.Contains(err.Error(), `http2: unexpected ALPN protocol ""; want "h2"`))
					return
				}
				log.Fatalf("Failed get: %s", err)
			}

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Fatalf("Failed reading response body: %s", err)
			}
			assert.Equal(t, "OK", string(body))

		})
	}
}
