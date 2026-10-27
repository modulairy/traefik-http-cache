// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package traefik_http_cache

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func varyConfig() *Config {
	c := testConfig()
	c.Rules[0].Request.Path = Match{Type: "regex", Value: `^/assets/.*\.(css|js)$`}
	c.Rules[0].Response.ContentTypes = []string{"text/css", "text/javascript", "application/javascript"}
	c.Rules[0].Cache.VaryHeaders = []string{"Accept-Encoding"}
	return c
}

func gzipBody(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func varyStores(t *testing.T, run func(*testing.T, *Config)) {
	t.Helper()
	for _, kind := range []string{"memory", "redis"} {
		t.Run(kind, func(t *testing.T) {
			c := varyConfig()
			c.KeyPrefix = fmt.Sprintf("vary-%s-%d", t.Name(), time.Now().UnixNano())
			if kind == "redis" {
				address := os.Getenv("REDIS_ADDR")
				if address == "" {
					t.Skip("set REDIS_ADDR for real Redis variant tests")
				}
				c.Store.Type = "redis"
				c.Store.Redis = RedisConfig{Address: address, Timeout: "1s"}
			}
			run(t, c)
		})
	}
}

func TestVaryVariants(t *testing.T) {
	varyStores(t, func(t *testing.T, c *Config) {
		plain := []byte("body { color: teal; }")
		compressed := gzipBody(t, plain)
		calls := 0
		h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.Header()["vArY"] = []string{"accept-encoding", "ACCEPT-ENCODING, Accept-Encoding"}
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=60")
			w.Header().Set("Last-Modified", "Mon, 26 Oct 2026 00:00:00 GMT")
			body := plain
			if strings.Contains(strings.Join(r.Header.Values("Accept-Encoding"), ","), "gzip") {
				body = compressed
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("ETag", `"gzip"`)
			} else {
				w.Header().Set("ETag", `"identity"`)
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(body)))
			_, _ = w.Write(body)
		})
		cases := []http.Header{
			nil, {"Accept-Encoding": {""}}, {"Accept-Encoding": {"identity"}},
			{"Accept-Encoding": {"gzip"}}, {"Accept-Encoding": {"gzip;q=0.5, identity;q=1"}},
			{"Accept-Encoding": {"gzip;q=1, identity;q=0.5"}},
			{"Accept-Encoding": {"gzip", "identity;q=0.5"}},
			{"Accept-Encoding": {"gzip, identity;q=0.5"}},
		}
		for i, headers := range cases {
			first := call(h, "GET", "/assets/app.css?version=1", headers)
			second := call(h, "GET", "/assets/app.css?version=1", headers)
			if first.Header().Get("X-Cache-Status") != "MISS" || second.Header().Get("X-Cache-Status") != "HIT" || calls != i+1 {
				t.Fatalf("variant %d: calls=%d headers=%v/%v", i, calls, first.Header(), second.Header())
			}
			if !bytes.Equal(first.Body.Bytes(), second.Body.Bytes()) {
				t.Fatal("body changed")
			}
			for _, name := range []string{"Content-Encoding", "Vary", "ETag", "Last-Modified", "Content-Length"} {
				if strings.Join(first.Header().Values(name), "|") != strings.Join(second.Header().Values(name), "|") {
					t.Fatalf("lost %s", name)
				}
			}
			if second.Header().Get("Content-Encoding") == "gzip" {
				if !bytes.Equal(second.Body.Bytes(), compressed) {
					t.Fatal("compressed bytes changed")
				}
				reader, err := gzip.NewReader(bytes.NewReader(second.Body.Bytes()))
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := io.ReadAll(reader)
				_ = reader.Close()
				if err != nil || !bytes.Equal(decoded, plain) {
					t.Fatal("invalid gzip")
				}
			} else if !bytes.Equal(second.Body.Bytes(), plain) {
				t.Fatal("identity changed")
			}
		}
		lower := call(h, "GET", "/assets/app.css?version=1", http.Header{"accept-encoding": {"gzip"}})
		if lower.Header().Get("X-Cache-Status") != "HIT" {
			t.Fatal("case-sensitive key")
		}
		for i := 0; i < 2; i++ {
			call(h, "GET", "/assets/app.css?version=1", http.Header{"Accept-Encoding": {"gzip"}, "accept-encoding": {"identity"}})
		}
		if calls != len(cases)+2 {
			t.Fatal("ambiguous aliases cached")
		}
	})
}

func TestVaryAdmissionAndPrivacy(t *testing.T) {
	for _, tc := range []struct {
		name              string
		request, response http.Header
		disabled          bool
		path              string
	}{
		{name: "default-off", response: http.Header{"Vary": {"Accept-Encoding"}}, disabled: true},
		{name: "wildcard", response: http.Header{"Vary": {"*"}}},
		{name: "wildcard-list", response: http.Header{"Vary": {"Accept-Encoding, *"}}},
		{name: "unsupported", response: http.Header{"Vary": {"Accept-Encoding", "Accept-Language"}}},
		{name: "empty-vary", response: http.Header{"Vary": {""}}},
		{name: "invalid-vary", response: http.Header{"Vary": {"Accept Encoding"}}},
		{name: "cookie", request: http.Header{"cookie": {""}}},
		{name: "authorization", request: http.Header{"authorization": {"Bearer test"}}},
		{name: "no-store-request", request: http.Header{"cache-control": {"no-store"}}},
		{name: "private", response: http.Header{"cache-control": {"private"}}},
		{name: "no-store", response: http.Header{"Cache-Control": {"no-store"}}},
		{name: "set-cookie", response: http.Header{"set-cookie": {"session=test"}}},
		{name: "html-mime", response: http.Header{"Content-Type": {"text/html"}}},
		{name: "html-path", path: "/index.html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := varyConfig()
			if tc.disabled {
				c.Rules[0].Cache.VaryHeaders = nil
				c.Rules[0].Cache.KeyHeaders = []string{"Accept-Encoding"}
			}
			calls := 0
			h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "text/css")
				w.Header().Set("Vary", "Accept-Encoding")
				for k, v := range tc.response {
					w.Header().Del(http.CanonicalHeaderKey(k))
					w.Header()[k] = v
				}
				fmt.Fprint(w, "body")
			})
			path := tc.path
			if path == "" {
				path = "/assets/app.css"
			}
			for i := 0; i < 2; i++ {
				call(h, "GET", path, tc.request)
			}
			if calls != 2 {
				t.Fatal("unsafe response/request cached")
			}
		})
	}
	for _, list := range [][]string{{"*"}, {"Cookie"}, {"Authorization"}, {"Accept-Language"}, {""}} {
		c := varyConfig()
		c.Rules[0].Cache.VaryHeaders = list
		if _, err := New(context.Background(), http.NotFoundHandler(), c, ""); err == nil {
			t.Fatalf("accepted %v", list)
		}
	}
	c := varyConfig()
	c.Rules[0].Cache.VaryHeaders = []string{"accept-encoding", "ACCEPT-ENCODING"}
	c.Rules[0].Cache.KeyHeaders = []string{"Accept-Language", "accept-encoding"}
	h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {})
	if len(h.rules[0].keyHeaders) != 2 || len(h.rules[0].varyHeaders) != 1 {
		t.Fatal("duplicate effective headers")
	}
}

func TestVaryCanonicalDownstreamAndKeyHeaders(t *testing.T) {
	c := varyConfig()
	c.Rules[0].Cache.KeyHeaders = []string{"Accept-Language"}
	h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Vary", "Accept-Encoding")
		fmt.Fprintf(w, "%s/%s", r.Header.Get("Accept-Encoding"), r.Header.Get("Accept-Language"))
	})
	first := call(h, "GET", "/assets/app.css", http.Header{"accept-encoding": {"gzip"}, "accept-language": {"tr"}})
	second := call(h, "GET", "/assets/app.css", http.Header{"Accept-Encoding": {"gzip"}, "Accept-Language": {"tr"}})
	other := call(h, "GET", "/assets/app.css", http.Header{"Accept-Encoding": {"gzip"}, "Accept-Language": {"en"}})
	if first.Body.String() != "gzip/tr" || second.Body.String() != "gzip/tr" || second.Header().Get("X-Cache-Status") != "HIT" || other.Body.String() != "gzip/en" || other.Header().Get("X-Cache-Status") != "MISS" {
		t.Fatal("downstream/key selectors differ", first, second, other)
	}
}

func TestVaryRevalidation(t *testing.T) {
	varyStores(t, func(t *testing.T, c *Config) {
		for _, mode := range []string{"same", "omitted", "added", "unsupported", "star", "empty", "encoding", "etag", "last-modified"} {
			t.Run(mode, func(t *testing.T) {
				cfg := *c
				cfg.Rules = append([]Rule(nil), c.Rules...)
				cfg.KeyPrefix += "-" + mode
				cfg.Rules[0].Cache.Revalidation.Mode = "always"
				calls := 0
				h := handler(t, &cfg, func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("Content-Type", "text/css")
					if mode != "added" || r.Header.Get("If-None-Match") != "" {
						w.Header().Set("Vary", "Accept-Encoding")
					}
					w.Header().Set("ETag", `"v1"`)
					w.Header().Set("Last-Modified", "Mon, 26 Oct 2026 00:00:00 GMT")
					conditional := r.Header.Get("If-None-Match") != ""
					if mode == "last-modified" {
						w.Header().Del("ETag")
						conditional = r.Header.Get("If-Modified-Since") != ""
					}
					if calls == 2 && conditional {
						switch mode {
						case "omitted":
							w.Header().Del("Vary")
						case "unsupported":
							w.Header().Add("Vary", "Accept-Language")
						case "star":
							w.Header().Set("Vary", "*")
						case "empty":
							w.Header().Set("Vary", "")
						case "encoding":
							w.Header().Set("Content-Encoding", "gzip")
						case "etag":
							w.Header().Set("ETag", `"other"`)
						}
						w.WriteHeader(304)
						return
					}
					if calls == 3 && (r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "") {
						t.Error("fallback is conditional")
					}
					fmt.Fprintf(w, "body-%d", calls)
				})
				call(h, "GET", "/assets/app.css", nil)
				out := call(h, "GET", "/assets/app.css", nil)
				unchanged := mode == "same" || mode == "omitted" || mode == "last-modified"
				if unchanged {
					if calls != 2 || out.Body.String() != "body-1" || out.Header().Get("X-Cache-Status") != "REVALIDATED" {
						t.Fatal(calls, out)
					}
				} else {
					if calls != 3 || out.Body.String() != "body-3" || out.Header().Get("Content-Encoding") != "" {
						t.Fatal("reused changed representation", calls, out)
					}
					next := call(h, "GET", "/assets/app.css", nil)
					if next.Header().Get("X-Cache-Status") != "MISS" {
						t.Fatal("old generation retained")
					}
				}
			})
		}
	})
}

func TestVaryIsolationAndInvalidation(t *testing.T) {
	varyStores(t, func(t *testing.T, c *Config) {
		calls := 0
		h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method == "PUT" {
				w.WriteHeader(204)
				return
			}
			w.Header().Set("Content-Type", "text/css")
			w.Header().Set("Vary", "Accept-Encoding")
			fmt.Fprint(w, calls)
		})
		get := func(url, encoding string, secure bool) *httptest.ResponseRecorder {
			r := httptest.NewRequest("GET", url, nil)
			r.Header.Set("Accept-Encoding", encoding)
			if secure {
				r.TLS = &tls.ConnectionState{}
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}
		urls := []string{"http://one.test/assets/a.css?a=1&b=2", "http://two.test/assets/a.css?a=1&b=2", "http://one.test/assets/a.css?b=2&a=1", "http://one.test/assets/a.css?a=2&b=2", "http://one.test/assets/%61.css?a=1&b=2"}
		for _, url := range urls {
			for _, encoding := range []string{"gzip", "identity"} {
				if get(url, encoding, false).Header().Get("X-Cache-Status") != "MISS" || get(url, encoding, false).Header().Get("X-Cache-Status") != "HIT" {
					t.Fatal("identity collision", url, encoding)
				}
			}
		}
		if get(urls[0], "gzip", true).Header().Get("X-Cache-Status") != "MISS" {
			t.Fatal("scheme collision")
		}
		r := httptest.NewRequest("PUT", urls[0], nil)
		h.ServeHTTP(httptest.NewRecorder(), r)
		for _, encoding := range []string{"gzip", "identity"} {
			if get(urls[0], encoding, false).Header().Get("X-Cache-Status") != "MISS" {
				t.Fatal("variant survived invalidation")
			}
		}
		if get(urls[1], "gzip", false).Header().Get("X-Cache-Status") != "HIT" {
			t.Fatal("other host invalidated")
		}
		if get(urls[2], "identity", false).Header().Get("X-Cache-Status") != "HIT" {
			t.Fatal("other query invalidated")
		}
		if get(urls[0], "gzip", true).Header().Get("X-Cache-Status") != "HIT" {
			t.Fatal("other scheme invalidated")
		}
	})
}

func TestVaryPolicyAndExpiry(t *testing.T) {
	varyStores(t, func(t *testing.T, c *Config) {
		c.Rules[0].Cache.TTL = "1ms"
		origin := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/css")
			w.Header().Set("Vary", "Accept-Encoding")
			fmt.Fprint(w, "body")
		}
		h := handler(t, c, origin)
		call(h, "GET", "/assets/a.css", nil)
		time.Sleep(5 * time.Millisecond)
		if call(h, "GET", "/assets/a.css", nil).Header().Get("X-Cache-Status") != "MISS" {
			t.Fatal("expiry ignored")
		}
		r := httptest.NewRequest("GET", "http://example.test/assets/a.css", nil)
		oldKey := h.key(r, h.rules[0])
		c.Rules[0].Cache.VaryHeaders = nil
		other := handler(t, c, origin)
		other.store = h.store
		if oldKey == other.key(r, other.rules[0]) {
			t.Fatal("policy collision")
		}
		if call(other, "GET", "/assets/a.css", nil).Header().Get("X-Cache-Status") == "HIT" {
			t.Fatal("old policy used")
		}
		// Even a corrupted/incompatible entry at the current key is not served.
		bad := entry{Header: http.Header{"Vary": {"*"}}, Body: []byte("unsafe"), Stored: time.Now(), FreshFor: 60}
		data, _ := json.Marshal(bad)
		scope := h.scope(r)
		_, gen, err := h.store.Read(context.Background(), scope, oldKey, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err = h.store.Write(context.Background(), scope, oldKey, gen, data, time.Minute); err != nil {
			t.Fatal(err)
		}
		if call(h, "GET", "/assets/a.css", nil).Body.String() == "unsafe" {
			t.Fatal("invalid stored Vary used")
		}
	})
}

func TestVaryKeyBeforeResponseAndInflightInvalidation(t *testing.T) {
	varyStores(t, func(t *testing.T, c *Config) {
		entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var first bool
		h := handler(t, c, func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "PUT" {
				w.WriteHeader(204)
				return
			}
			w.Header().Set("Content-Type", "text/css")
			// Deliberately omit Vary: configured selectors still belong to the key.
			if !first {
				first = true
				close(entered)
				<-release
				fmt.Fprint(w, "old")
				return
			}
			fmt.Fprint(w, r.Header.Get("Accept-Encoding"))
		})
		gzipHeader := http.Header{"Accept-Encoding": {"gzip"}}
		go func() { call(h, "GET", "/assets/app.css", gzipHeader); close(finished) }()
		<-entered
		call(h, "PUT", "/assets/app.css", nil)
		close(release)
		<-finished
		for _, encoding := range []string{"gzip", "identity"} {
			headers := http.Header{"Accept-Encoding": {encoding}}
			out := call(h, "GET", "/assets/app.css", headers)
			if out.Header().Get("X-Cache-Status") != "MISS" || out.Body.String() != encoding {
				t.Fatal("late fill/selector collision", out)
			}
			if call(h, "GET", "/assets/app.css", headers).Header().Get("X-Cache-Status") != "HIT" {
				t.Fatal("no-Vary keyed response not cached")
			}
		}
	})
}
