// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

// Test fixture only; never deploy this mutable backend in production.
package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

func main() {
	var version int64 = 1
	var assetCalls int64
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			atomic.AddInt64(&version, 1)
			w.WriteHeader(204)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			asset(w, r, atomic.LoadInt64(&version), atomic.AddInt64(&assetCalls, 1))
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

func asset(w http.ResponseWriter, r *http.Request, version, calls int64) {
	query := r.URL.Query()
	w.Header().Set("Content-Type", "text/css")
	if strings.HasSuffix(r.URL.Path, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	}
	if query.Get("html") == "1" {
		w.Header().Set("Content-Type", "text/html")
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	if query.Get("expiry") == "1" {
		w.Header().Set("Cache-Control", "public, max-age=1")
	}
	if query.Get("validate") == "1" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if value := query.Get("privacy"); value != "" {
		if value == "cookie" {
			w.Header().Set("Set-Cookie", "fixture=test")
		} else {
			w.Header().Set("Cache-Control", value)
		}
	}
	w.Header().Set("Vary", "Accept-Encoding")
	switch query.Get("vary") {
	case "unsupported":
		w.Header().Add("Vary", "Accept-Language")
	case "star":
		w.Header().Set("Vary", "*")
	case "none":
		w.Header().Del("Vary")
	case "multiple":
		w.Header()["Vary"] = []string{"accept-encoding", "ACCEPT-ENCODING, Accept-Encoding"}
	}
	compressed := false
	for _, line := range r.Header.Values("Accept-Encoding") {
		for _, item := range strings.Split(line, ",") {
			parts := strings.Split(item, ";")
			if strings.TrimSpace(parts[0]) != "gzip" {
				continue
			}
			quality := 1.0
			for _, param := range parts[1:] {
				if strings.HasPrefix(strings.TrimSpace(param), "q=") {
					quality, _ = strconv.ParseFloat(strings.TrimPrefix(strings.TrimSpace(param), "q="), 64)
				}
			}
			compressed = quality > 0
		}
	}
	encoding := "identity"
	if compressed {
		encoding = "gzip"
		w.Header().Set("Content-Encoding", encoding)
	}
	tag := fmt.Sprintf("\"asset-%s-%d\"", encoding, version)
	w.Header().Set("ETag", tag)
	w.Header().Set("Last-Modified", "Mon, 26 Oct 2026 00:00:00 GMT")
	w.Header().Set("X-Origin-Calls", fmt.Sprint(calls))
	if r.Header.Get("If-None-Match") == tag {
		w.Header().Set("X-Origin-Conditional", "true")
		switch query.Get("change") {
		case "unsupported":
			w.Header().Add("Vary", "Accept-Language")
		case "star":
			w.Header().Set("Vary", "*")
		case "empty":
			w.Header().Set("Vary", "")
		case "added":
			w.Header().Set("Vary", "Accept-Encoding")
		case "encoding":
			w.Header().Set("Content-Encoding", "br")
		case "omitted":
			w.Header().Del("Vary")
		}
		w.WriteHeader(304)
		return
	}
	body := []byte(fmt.Sprintf("/* public fixture %d */ body { color: teal; }", version))
	if strings.HasSuffix(r.URL.Path, ".js") {
		body = []byte(fmt.Sprintf("/* public fixture %d */ console.log('cache');", version))
	}
	if compressed {
		var buffer bytes.Buffer
		zw := gzip.NewWriter(&buffer)
		_, _ = zw.Write(body)
		_ = zw.Close()
		body = buffer.Bytes()
	}
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	_, _ = w.Write(body)
}
