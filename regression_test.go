// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package traefik_http_cache

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestQAHeaderBoundaryCollision(t *testing.T) {
	c := testConfig()
	c.Rules[0].Cache.KeyHeaders = []string{"A", "B"}
	h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%q/%q", r.Header.Values("A"), r.Header.Values("B"))
	})
	first := call(h, "GET", "/", http.Header{"A": {"x", "B", "present"}, "B": {"y"}})
	second := call(h, "GET", "/", http.Header{"A": {"x"}, "B": {"B", "present", "y"}})
	if first.Body.String() == second.Body.String() {
		t.Fatal("different variants shared response", second.Header())
	}
}
func TestQAOnlyIfCached(t *testing.T) {
	calls := 0
	h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "body") })
	w := call(h, "GET", "/", http.Header{"Cache-Control": {"only-if-cached"}})
	if calls != 0 || w.Code != 504 {
		t.Fatalf("backend called %d times, status %d", calls, w.Code)
	}
	call(h, "GET", "/", nil)
	w = call(h, "GET", "/", http.Header{"Cache-Control": {"only-if-cached"}})
	if calls != 1 || w.Code != 200 {
		t.Fatal("cache-only did not use fresh response")
	}
}
func TestQAHeadersAfterWriteHeader(t *testing.T) {
	h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "body")
	})
	w := call(h, "GET", "/", nil)
	if w.Header().Get("Content-Type") != "text/plain" {
		t.Fatal("post-WriteHeader mutation changed sent headers", w.Header())
	}
}
func TestQA304PrivacyEvicts(t *testing.T) {
	for _, name := range []string{"Cache-Control", "Set-Cookie", "Vary"} {
		t.Run(name, func(t *testing.T) {
			c := testConfig()
			c.Rules[0].Cache.Revalidation.Mode = "always"
			calls := 0
			h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("ETag", "\"v1\"")
				if calls == 2 {
					w.Header().Set(name, "no-store")
					w.WriteHeader(304)
					return
				}
				if calls == 3 && r.Header.Get("If-None-Match") != "" {
					t.Error("privacy-disallowed entry survived 304")
				}
				fmt.Fprint(w, "body")
			})
			call(h, "GET", "/", nil)
			w := call(h, "GET", "/", nil)
			if w.Code != 200 || w.Body.String() != "body" {
				t.Fatal("304 lost representation")
			}
			call(h, "GET", "/", nil)
		})
	}
}
func TestQAInflightReadCannotResurrectAfterWrite(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	var reads int32
	h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(204)
			return
		}
		if atomic.AddInt32(&reads, 1) == 1 {
			close(entered)
			<-release
			fmt.Fprint(w, "old")
			return
		}
		fmt.Fprint(w, "new")
	})
	go func() { call(h, "GET", "/", nil); close(finished) }()
	<-entered
	call(h, "PUT", "/", nil)
	close(release)
	<-finished
	w := call(h, "GET", "/", nil)
	if w.Body.String() != "new" {
		t.Fatal("inflight old response resurrected")
	}
}

func TestQAFlushBeforeWritePreservesHeaders(t *testing.T) {
	h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "data: hello\n\n")
	})
	out := call(h, "GET", "/", nil)
	if got := out.Result().Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("lost header on first flush: %q", got)
	}
	if got := out.Result().Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("lost cache policy on first flush: %q", got)
	}
}
