// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package traefik_http_cache

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig() *Config {
	c := CreateConfig()
	c.KeyPrefix = "test"
	c.Rules = []Rule{{Name: "public", Request: RequestRule{Path: Match{Type: "prefix", Value: "/"}}, Cache: CacheConfig{TTL: "1m", Retention: "10m", MaxBodyBytes: 1024}}}
	return c
}
func handler(t *testing.T, c *Config, next http.HandlerFunc) *middleware {
	t.Helper()
	h, err := New(context.Background(), next, c, "test")
	if err != nil {
		t.Fatal(err)
	}
	return h.(*middleware)
}
func call(h http.Handler, method, path string, headers http.Header) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://example.test"+path, nil)
	if headers != nil {
		r.Header = headers
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestHitAndIsolation(t *testing.T) {
	var calls int
	h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, r.URL.RequestURI()) })
	if call(h, "GET", "/a?x=1", nil).Header().Get("X-Cache-Status") != "MISS" {
		t.Fatal("first request")
	}
	if call(h, "GET", "/a?x=1", nil).Header().Get("X-Cache-Status") != "HIT" {
		t.Fatal("cache hit")
	}
	call(h, "GET", "/a?x=2", nil)
	if calls != 2 {
		t.Fatal(calls)
	}
	r := httptest.NewRequest("GET", "http://other.test/a?x=1", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	if calls != 3 {
		t.Fatal("host collision")
	}
}
func TestPrivacyAndResponseConditions(t *testing.T) {
	for _, name := range []string{"Authorization", "Cookie", "Range", "If-None-Match"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "body") })
			for i := 0; i < 2; i++ {
				call(h, "GET", "/", http.Header{name: []string{"value"}})
			}
			if calls != 2 {
				t.Fatal("cached private or conditional request")
			}
		})
	}
	for _, headers := range []http.Header{{"Cache-Control": {"private"}}, {"Cache-Control": {"no-store"}}, {"Set-Cookie": {"session=1"}}, {"Vary": {"Accept-Language"}}, {"Content-Type": {"text/event-stream"}}, {"Cache-Control": {"max-age=1, max-age=100"}}} {
		t.Run(fmt.Sprint(headers), func(t *testing.T) {
			calls := 0
			h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) {
				calls++
				copyHeaders(w.Header(), headers)
				fmt.Fprint(w, "body")
			})
			call(h, "GET", "/", nil)
			call(h, "GET", "/", nil)
			if calls != 2 {
				t.Fatal("unsafe response cached")
			}
		})
	}
	c := testConfig()
	c.Rules[0].Response = ResponseRule{ContentTypes: []string{"application/*+json"}, Headers: []HeaderMatch{{Name: "X-Cache", Operator: "exact", Value: "yes"}}}
	calls := 0
	h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
		w.Header().Set("X-Cache", "yes")
		fmt.Fprint(w, "{}")
	})
	call(h, "GET", "/", nil)
	call(h, "GET", "/", nil)
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestRevalidation(t *testing.T) {
	for _, validator := range []string{"ETag", "Last-Modified"} {
		t.Run(validator, func(t *testing.T) {
			c := testConfig()
			c.Rules[0].Cache.Revalidation.Mode = "always"
			version := 1
			calls := 0
			h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
				calls++
				value := fmt.Sprintf("\"v%d\"", version)
				conditional := "If-None-Match"
				if validator == "Last-Modified" {
					value = time.Date(2026, 1, version, 0, 0, 0, 0, time.UTC).Format(http.TimeFormat)
					conditional = "If-Modified-Since"
				}
				w.Header().Set(validator, value)
				if r.Header.Get(conditional) == value {
					w.Header().Set("X-Validated", "yes")
					w.WriteHeader(304)
					return
				}
				fmt.Fprintf(w, "body%d", version)
			})
			call(h, "GET", "/", nil)
			w := call(h, "GET", "/", nil)
			if w.Code != 200 || w.Body.String() != "body1" || w.Header().Get("X-Cache-Status") != "REVALIDATED" || w.Header().Get("X-Validated") != "yes" {
				t.Fatal(w)
			}
			version = 2
			w = call(h, "GET", "/", nil)
			if w.Body.String() != "body2" || calls != 3 {
				t.Fatal(w, calls)
			}
		})
	}
}
func TestTTLAndNoCache(t *testing.T) {
	for _, cc := range []string{"no-cache", "max-age=0", "s-maxage=0, max-age=60"} {
		t.Run(cc, func(t *testing.T) {
			calls := 0
			h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Cache-Control", cc)
				fmt.Fprint(w, "fresh")
			})
			call(h, "GET", "/", nil)
			call(h, "GET", "/", nil)
			if calls != 2 {
				t.Fatal("freshness ignored")
			}
		})
	}
	c := testConfig()
	c.Rules[0].Cache.TTL = "1ms"
	calls := 0
	h := handler(t, c, func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "body") })
	call(h, "GET", "/", nil)
	time.Sleep(3 * time.Millisecond)
	call(h, "GET", "/", nil)
	if calls != 2 {
		t.Fatal("ttl ignored")
	}
}
func TestLargeAndStreaming(t *testing.T) {
	for _, flush := range []bool{false, true} {
		t.Run(fmt.Sprint(flush), func(t *testing.T) {
			c := testConfig()
			c.Rules[0].Cache.MaxBodyBytes = 8
			calls := 0
			h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
				calls++
				fmt.Fprint(w, "1234")
				if flush {
					w.(http.Flusher).Flush()
				}
				fmt.Fprint(w, "567890")
			})
			for i := 0; i < 2; i++ {
				w := call(h, "GET", "/", nil)
				if w.Body.String() != "1234567890" {
					t.Fatal("truncated response")
				}
			}
			if calls != 2 {
				t.Fatal("stream cached")
			}
		})
	}
}
func TestInvalidation(t *testing.T) {
	calls := 0
	h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, "body") })
	call(h, "GET", "/", nil)
	call(h, "GET", "/", nil)
	call(h, "PUT", "/", nil)
	call(h, "GET", "/", nil)
	if calls != 3 {
		t.Fatal(calls)
	}
}
func TestRulesAndHeaderKey(t *testing.T) {
	for _, kind := range []string{"exact", "prefix", "contains", "regex"} {
		m, err := compileMatch(kind, "/api", false)
		if err != nil || !m.matches("/api") {
			t.Fatal(kind, err)
		}
	}
	c := testConfig()
	c.Rules[0].Cache.KeyHeaders = []string{"Accept-Language"}
	calls := 0
	h := handler(t, c, func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, r.Header.Get("Accept-Language")) })
	for _, lang := range []string{"tr", "en", "tr"} {
		call(h, "GET", "/", http.Header{"Accept-Language": {lang}})
	}
	if calls != 2 {
		t.Fatal("header key collision")
	}
	c.Rules[0].Request.Path = Match{Type: "regex", Value: "["}
	if _, err := New(context.Background(), http.NotFoundHandler(), c, ""); err == nil {
		t.Fatal("invalid regex accepted")
	}
}
func TestConcurrent(t *testing.T) {
	var calls int64
	h := handler(t, testConfig(), func(w http.ResponseWriter, r *http.Request) { atomic.AddInt64(&calls, 1); fmt.Fprint(w, "body") })
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				call(h, "GET", "/", nil)
			}
		}()
	}
	wg.Wait()
}
func TestRedisFailure(t *testing.T) {
	c := testConfig()
	c.Store.Type = "redis"
	c.Store.Redis = RedisConfig{Address: "127.0.0.1:1", Timeout: "10ms"}
	h := handler(t, c, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "backend") })
	w := call(h, "GET", "/", nil)
	if w.Body.String() != "backend" || w.Header().Get("X-Cache-Status") != "BYPASS" {
		t.Fatal(w)
	}
}
func TestAgeAndBackendError(t *testing.T) {
	c := testConfig()
	calls := 0
	h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 1 {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Age", "120")
		fmt.Fprint(w, "old")
	})
	call(h, "GET", "/", nil)
	w := call(h, "GET", "/", nil)
	if w.Code != 503 || strings.Contains(w.Body.String(), "old") {
		t.Fatal("stale served on error")
	}
}
