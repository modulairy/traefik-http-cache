// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

// Test fixture only; never deploy this mutable backend in production.
package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

func main() {
	var version int64 = 1
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			atomic.AddInt64(&version, 1)
			w.WriteHeader(204)
			return
		}
		tag := fmt.Sprintf("\"v%d\"", atomic.LoadInt64(&version))
		w.Header().Set("ETag", tag)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Header().Set("X-Cache-Enabled", "true")
		if r.URL.Path == "/private" {
			w.Header().Set("Cache-Control", "private")
		}
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(304)
			return
		}
		fmt.Fprintf(w, "{\"version\":%d,\"path\":%q}", atomic.LoadInt64(&version), r.URL.Path)
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
