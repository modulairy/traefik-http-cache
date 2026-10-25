// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package traefik_http_cache

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestTraefikIntegration(t *testing.T) {
	base := os.Getenv("CACHE_TEST_URL")
	if base == "" {
		t.Skip("set CACHE_TEST_URL for real Traefik/Yaegi tests")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, host := range []string{"memory.localhost", "redis.localhost"} {
		t.Run(host, func(t *testing.T) {
			request := func(method, path string, headers http.Header) (http.Header, string, int) {
				t.Helper()
				r, _ := http.NewRequest(method, base+path, nil)
				r.Host = host
				if headers != nil {
					r.Header = headers
				}
				resp, err := client.Do(r)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				b, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				return resp.Header, string(b), resp.StatusCode
			}
			path := fmt.Sprintf("/public?test=%d", time.Now().UnixNano())
			h, b, code := request("GET", path, nil)
			if code != 200 || h.Get("X-Cache-Status") != "MISS" {
				t.Fatalf("first: %d %v %s", code, h, b)
			}
			h, b2, _ := request("GET", path, nil)
			if h.Get("X-Cache-Status") != "HIT" || b != b2 {
				t.Fatal("hit failed", h, b2)
			}
			request("PUT", path, nil)
			h, b2, _ = request("GET", path, nil)
			if h.Get("X-Cache-Status") != "MISS" || b == b2 {
				t.Fatal("invalidation failed", h, b2)
			}
			path = fmt.Sprintf("/validate?test=%d", time.Now().UnixNano())
			request("GET", path, nil)
			h, _, code = request("GET", path, nil)
			if code != 200 || h.Get("X-Cache-Status") != "REVALIDATED" {
				t.Fatal("304 validation failed", code, h)
			}
			for i := 0; i < 2; i++ {
				h, _, _ = request("GET", "/private", nil)
				if h.Get("X-Cache-Status") == "HIT" {
					t.Fatal("private cached")
				}
			}
			h, _, _ = request("GET", "/auth", http.Header{"Authorization": {"Bearer test-only"}})
			if h.Get("X-Cache-Status") == "HIT" {
				t.Fatal("auth cached")
			}
		})
	}
}
