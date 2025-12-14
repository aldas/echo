// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package echo

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConcurrentRouter_Remove(t *testing.T) {
	router := NewConcurrentRouter(NewRouter(RouterConfig{}))

	_, err := router.Add(Route{
		Method:  http.MethodGet,
		Path:    "/initial1",
		Handler: handlerFunc,
	})
	assert.NoError(t, err)
	assert.Equal(t, len(router.Routes()), 1)

	err = router.Remove(http.MethodGet, "/initial1")
	assert.NoError(t, err)
	assert.Equal(t, len(router.Routes()), 0)
}

func TestConcurrentRouter_ConcurrentReads(t *testing.T) {
	router := NewConcurrentRouter(NewRouter(RouterConfig{}))

	testPaths := []string{"/route1", "/route2", "/route3", "/route4", "/route5"}
	for _, path := range testPaths {
		_, err := router.Add(Route{
			Method:  http.MethodGet,
			Path:    path,
			Handler: handlerFunc,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Launch 10 goroutines for concurrent reads
	var wg sync.WaitGroup
	var routeCallCount atomic.Int64
	var routesCallCount atomic.Int64

	numGoroutines := 10
	routeCallsPerGoroutine := 50
	routesCallsPerGoroutine := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			// Call Route() 50 times
			for j := 0; j < routeCallsPerGoroutine; j++ {
				path := testPaths[j%len(testPaths)]
				req := httptest.NewRequest(http.MethodGet, path, nil)
				rec := httptest.NewRecorder()
				c := NewContext(req, rec, nil)

				handler := router.Route(c)
				if handler != nil {
					routeCallCount.Add(1)
				}
			}

			// Call Routes() 20 times
			for j := 0; j < routesCallsPerGoroutine; j++ {
				routes := router.Routes()
				if len(routes) == 5 {
					routesCallCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify all operations succeeded
	expectedRouteCalls := int64(numGoroutines * routeCallsPerGoroutine)
	expectedRoutesCalls := int64(numGoroutines * routesCallsPerGoroutine)

	assert.Equal(t, expectedRouteCalls, routeCallCount.Load(), "all Route() calls should succeed")
	assert.Equal(t, expectedRoutesCalls, routesCallCount.Load(), "all Routes() calls should succeed")
}

func TestConcurrentRouter_ConcurrentWrites(t *testing.T) {
	router := NewConcurrentRouter(NewRouter(RouterConfig{}))

	_, _ = router.Add(Route{Method: http.MethodGet, Path: "/initial1", Handler: handlerFunc})
	_, _ = router.Add(Route{Method: http.MethodGet, Path: "/initial2", Handler: handlerFunc})

	// Launch 5 goroutines, each adds 10 unique routes
	var wg sync.WaitGroup
	var addCount atomic.Int64

	numGoroutines := 5
	addsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < addsPerGoroutine; j++ {
				path := fmt.Sprintf("/route-g%d-n%d", goroutineID, j)
				_, err := router.Add(Route{
					Method:  http.MethodGet,
					Path:    path,
					Handler: handlerFunc,
				})
				if err == nil {
					addCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify final route count
	expectedAdds := int64(numGoroutines * addsPerGoroutine)
	assert.Equal(t, expectedAdds, addCount.Load(), "all Add() calls should succeed")

	expectedTotal := 2 + int(expectedAdds) // 2 initial + 50 added
	assert.Len(t, router.Routes(), expectedTotal, "route count mismatch")

	// Verify all routes are accessible
	allRoutes := router.Routes()
	assert.Len(t, allRoutes, expectedTotal)
}

func TestConcurrentRouter_ConcurrentReadWrite(t *testing.T) {
	router := NewConcurrentRouter(NewRouter(RouterConfig{}))

	initialPaths := []string{"/read1", "/read2", "/read3"}
	for _, path := range initialPaths {
		_, err := router.Add(Route{Method: http.MethodGet, Path: path, Handler: handlerFunc})
		if err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	var routeCallCount atomic.Int64
	var addCallCount atomic.Int64
	var routesCallCount atomic.Int64

	// Launch 4 reader goroutines: call Route() 100 times each
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				path := initialPaths[j%len(initialPaths)]

				req := httptest.NewRequest(http.MethodGet, path, nil)
				rec := httptest.NewRecorder()
				c := NewContext(req, rec, nil)

				handler := router.Route(c)
				if handler != nil {
					routeCallCount.Add(1)
				}
			}
		}()
	}

	// Launch 2 writer goroutines: call Add() 20 times each
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				path := fmt.Sprintf("/write-g%d-n%d", goroutineID, j)
				_, err := router.Add(Route{
					Method:  http.MethodGet,
					Path:    path,
					Handler: handlerFunc,
				})
				if err == nil {
					addCallCount.Add(1)
				}
			}
		}(i)
	}

	// Launch 2 inspector goroutines: call Routes() 50 times each
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				routes := router.Routes()
				if routes != nil {
					routesCallCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	// Verify all operations succeeded
	assert.Equal(t, int64(400), routeCallCount.Load(), "all Route() calls should succeed")
	assert.Equal(t, int64(40), addCallCount.Load(), "all Add() calls should succeed")
	assert.Equal(t, int64(100), routesCallCount.Load(), "all Routes() calls should succeed")

	// Verify final route count
	expectedTotal := 3 + 40 // 3 initial + 40 added
	assert.Len(t, router.Routes(), expectedTotal, "route count mismatch")
}
