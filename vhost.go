// SPDX-License-Identifier: MIT
// SPDX-FileCopyrightText: © 2015 LabStack LLC and Echo contributors

package echo

import "net/http"

// NewVirtualHostHandler creates instance of Echo that routes requests to given virtual hosts
// when hosts in request does not exists in given map the request is served by returned Echo instance.
func NewVirtualHostHandler(vhosts map[string]*Echo) *Echo {
	e := New()
	e.serveHTTPFunc = func(w http.ResponseWriter, r *http.Request) {
		if e, ok := vhosts[r.Host]; ok {
			e.ServeHTTP(w, r)
			return
		}
		e.serveHTTP(w, r)
	}
	return e
}
