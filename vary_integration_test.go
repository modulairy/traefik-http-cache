// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package traefik_http_cache

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTraefikVaryIntegration(t *testing.T) {
	base := os.Getenv("CACHE_TEST_URL")
	if base == "" {
		t.Skip("set CACHE_TEST_URL for real Traefik/Yaegi variants")
	}
	transport := &http.Transport{DisableCompression: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	for _, host := range []string{"memory.localhost", "redis.localhost"} {
		t.Run(host, func(t *testing.T) {
			request := func(method, path string, headers http.Header) (http.Header, []byte) {
				t.Helper()
				r, _ := http.NewRequest(method, base+path, nil)
				r.Host = host
				if headers != nil {
					r.Header = headers.Clone()
				}
				resp, err := client.Do(r)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != 200 && resp.StatusCode != 204 {
					t.Fatalf("status %d: %s", resp.StatusCode, body)
				}
				if resp.Uncompressed {
					t.Fatal("client decompressed response")
				}
				return resp.Header, body
			}
			path := fmt.Sprintf("/assets/app.css?test=%d", time.Now().UnixNano())
			cases := []http.Header{nil, {"Accept-Encoding": {""}}, {"Accept-Encoding": {"identity"}}, {"Accept-Encoding": {"gzip"}}, {"Accept-Encoding": {"gzip;q=0, identity;q=1"}}, {"Accept-Encoding": {"gzip;q=0.5, identity;q=1"}}, {"Accept-Encoding": {"gzip;q=1, identity;q=0.5"}}, {"Accept-Encoding": {"gzip", "identity;q=0.5"}}}
			originIDs := make(map[string]bool)
			for _, headers := range cases {
				first, body := request("GET", path, headers)
				second, cached := request("GET", path, headers)
				if first.Get("X-Cache-Status") != "MISS" || second.Get("X-Cache-Status") != "HIT" || !bytes.Equal(body, cached) {
					t.Fatalf("variant failed %v: %v / %v", headers, first, second)
				}
				if originIDs[first.Get("X-Origin-Calls")] {
					t.Fatal("different variants shared origin response")
				}
				originIDs[first.Get("X-Origin-Calls")] = true
				for _, name := range []string{"Content-Encoding", "Vary", "ETag", "Last-Modified", "Content-Length", "X-Origin-Calls"} {
					if strings.Join(first.Values(name), "|") != strings.Join(second.Values(name), "|") {
						t.Fatalf("changed %s", name)
					}
				}
				if second.Get("Content-Encoding") == "gzip" {
					reader, err := gzip.NewReader(bytes.NewReader(cached))
					if err != nil {
						t.Fatal(err)
					}
					decoded, err := io.ReadAll(reader)
					_ = reader.Close()
					if err != nil || !bytes.Contains(decoded, []byte("color: teal")) {
						t.Fatal("gzip payload invalid")
					}
				} else if !bytes.Contains(cached, []byte("color: teal")) {
					t.Fatal("identity payload invalid")
				}
			}
			request("PUT", path, nil)
			for _, encoding := range []string{"gzip", "identity"} {
				headers := http.Header{"Accept-Encoding": {encoding}}
				h, _ := request("GET", path, headers)
				if h.Get("X-Cache-Status") != "MISS" {
					t.Fatal("variant not invalidated")
				}
			}
			for _, suffix := range []string{"&other=1", "&other=2"} {
				h, _ := request("GET", path+suffix, nil)
				if h.Get("X-Cache-Status") != "MISS" {
					t.Fatal("query mixed")
				}
			}
			validatePath := path + "&validate=1&variants=1"
			for _, encoding := range []string{"gzip", "identity"} {
				headers := http.Header{"Accept-Encoding": {encoding}}
				first, body := request("GET", validatePath, headers)
				second, cached := request("GET", validatePath, headers)
				if first.Get("X-Cache-Status") != "MISS" || second.Get("X-Cache-Status") != "REVALIDATED" || first.Get("ETag") != second.Get("ETag") || !bytes.Equal(body, cached) {
					t.Fatal("variant validator/body mixed", encoding, first, second)
				}
			}
			for i := 0; i < 2; i++ {
				h, _ := request("GET", strings.Replace(path, "/assets/", "/assets/default/", 1), nil)
				if h.Get("X-Cache-Status") != "MISS" {
					t.Fatal("default Vary exclusion changed", h)
				}
			}
			expiryPath := path + "&expiry=1"
			identityHeader := http.Header{"Accept-Encoding": {"identity"}}
			request("GET", expiryPath, identityHeader)
			time.Sleep(2100 * time.Millisecond)
			expired, _ := request("GET", expiryPath, identityHeader)
			if expired.Get("X-Cache-Status") != "REVALIDATED" {
				t.Fatal("expired variant was not validated", expired)
			}
			for _, change := range []string{"same", "omitted", "unsupported", "star", "empty", "encoding", "added"} {
				p := path + "&validate=1&change=" + change
				if change == "added" {
					p += "&vary=none"
				}
				headers := http.Header{"Accept-Encoding": {"gzip"}}
				_, original := request("GET", p, headers)
				h, body := request("GET", p, headers)
				if !bytes.Equal(original, body) || h.Get("Content-Encoding") != "gzip" {
					t.Fatal("304 damaged compressed representation", change, h)
				}
				if change == "same" || change == "omitted" {
					if h.Get("X-Cache-Status") != "REVALIDATED" || h.Get("X-Origin-Conditional") != "true" {
						t.Fatal("304 not reused", h)
					}
				} else {
					if h.Get("X-Origin-Conditional") != "" || h.Get("X-Cache-Status") == "REVALIDATED" {
						t.Fatal("unsafe 304 body reused", change, h)
					}
				}
			}
			for _, query := range []string{"vary=star", "vary=unsupported", "privacy=private", "privacy=no-store", "privacy=cookie", "html=1"} {
				for i := 0; i < 2; i++ {
					h, _ := request("GET", path+"&"+query, nil)
					if h.Get("X-Cache-Status") == "HIT" {
						t.Fatal("unsafe response cached", query)
					}
				}
			}
			for _, headers := range []http.Header{{"Cookie": {"session=test"}}, {"Authorization": {"Bearer test"}}} {
				for i := 0; i < 2; i++ {
					h, _ := request("GET", path, headers)
					if h.Get("X-Cache-Status") == "HIT" {
						t.Fatal("private request hit")
					}
				}
			}
			js := strings.Replace(path, "app.css", "app.js", 1) + "&vary=multiple"
			headers := http.Header{"Accept-Encoding": {"gzip"}}
			request("GET", js, headers)
			h, _ := request("GET", js, headers)
			if h.Get("X-Cache-Status") != "HIT" || h.Get("Content-Type") != "application/javascript" {
				t.Fatal("JS/multiline Vary failed", h)
			}
		})
	}
}
