// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

// Package traefik_http_cache implements a conservative shared HTTP cache for Traefik.
package traefik_http_cache

import (
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Config is the Traefik dynamic configuration.
type Config struct {
	KeyPrefix string      `json:"keyPrefix"`
	Store     StoreConfig `json:"store"`
	Rules     []Rule      `json:"rules"`
}

type StoreConfig struct {
	Type   string       `json:"type"`
	Memory MemoryConfig `json:"memory"`
	Redis  RedisConfig  `json:"redis"`
}
type MemoryConfig struct {
	MaxEntries int   `json:"maxEntries"`
	MaxBytes   int64 `json:"maxBytes"`
}
type RedisConfig struct {
	Address      string `json:"address"`
	Username     string `json:"username"`
	PasswordFile string `json:"passwordFile"`
	DB           int    `json:"db"`
	Timeout      string `json:"timeout"`
	TLS          bool   `json:"tls"`
	ServerName   string `json:"serverName"`
}
type Match struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}
type HeaderMatch struct {
	Name     string `json:"name"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}
type RequestRule struct {
	Path    Match         `json:"path"`
	Methods []string      `json:"methods"`
	Headers []HeaderMatch `json:"headers"`
}
type ResponseRule struct {
	ContentTypes []string      `json:"contentTypes"`
	Headers      []HeaderMatch `json:"headers"`
}
type Revalidation struct {
	Mode string `json:"mode"`
}
type CacheConfig struct {
	TTL          string       `json:"ttl"`
	Retention    string       `json:"retention"`
	MaxBodyBytes int          `json:"maxBodyBytes"`
	KeyHeaders   []string     `json:"keyHeaders"`
	Revalidation Revalidation `json:"revalidation"`
}
type Rule struct {
	Name     string       `json:"name"`
	Request  RequestRule  `json:"request"`
	Response ResponseRule `json:"response"`
	Cache    CacheConfig  `json:"cache"`
}
type compiledMatch struct {
	kind, value string
	re          *regexp.Regexp
}
type compiledHeader struct {
	name  string
	match compiledMatch
}
type compiledRule struct {
	config                          Rule
	path                            compiledMatch
	requestHeaders, responseHeaders []compiledHeader
	ttl, retention                  time.Duration
}

func CreateConfig() *Config {
	return &Config{Store: StoreConfig{Type: "memory", Memory: MemoryConfig{MaxEntries: 10000, MaxBytes: 64 << 20}, Redis: RedisConfig{Timeout: "200ms"}}}
}

func compileMatch(kind, value string, header bool) (compiledMatch, error) {
	m := compiledMatch{kind: kind, value: value}
	switch kind {
	case "exact", "prefix", "contains":
		if value == "" {
			return m, fmt.Errorf("empty match value")
		}
	case "exists":
		if !header {
			return m, fmt.Errorf("exists is only supported for headers")
		}
	case "regex":
		var err error
		m.re, err = regexp.Compile(value)
		if err != nil {
			return m, err
		}
	default:
		return m, fmt.Errorf("unknown match type %q", kind)
	}
	return m, nil
}
func (m compiledMatch) matches(value string) bool {
	switch m.kind {
	case "exact":
		return value == m.value
	case "prefix":
		return strings.HasPrefix(value, m.value)
	case "contains":
		return strings.Contains(value, m.value)
	case "regex":
		return m.re.MatchString(value)
	case "exists":
		return true
	}
	return false
}
func compileHeaders(headers []HeaderMatch) ([]compiledHeader, error) {
	var result []compiledHeader
	for _, h := range headers {
		if !validHeaderName(h.Name) {
			return nil, fmt.Errorf("invalid header name %q", h.Name)
		}
		m, err := compileMatch(h.Operator, h.Value, true)
		if err != nil {
			return nil, err
		}
		result = append(result, compiledHeader{name: http.CanonicalHeaderKey(h.Name), match: m})
	}
	return result, nil
}
func validHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}
func headersMatch(h http.Header, rules []compiledHeader) bool {
	for _, r := range rules {
		values, exists := h[r.name]
		if !exists || !r.match.matches(strings.Join(values, ", ")) {
			return false
		}
	}
	return true
}
func compileRules(rules []Rule) ([]compiledRule, error) {
	if len(rules) == 0 {
		return nil, fmt.Errorf("at least one rule is required")
	}
	seen := make(map[string]bool)
	var result []compiledRule
	for _, r := range rules {
		if r.Name == "" || seen[r.Name] {
			return nil, fmt.Errorf("rule names must be nonempty and unique")
		}
		seen[r.Name] = true
		if len(r.Request.Methods) == 0 {
			r.Request.Methods = []string{"GET"}
		}
		for _, method := range r.Request.Methods {
			if method != "GET" {
				return nil, fmt.Errorf("only GET caching is supported")
			}
		}
		if r.Cache.TTL == "" {
			r.Cache.TTL = "60s"
		}
		if r.Cache.Retention == "" {
			r.Cache.Retention = "10m"
		}
		if r.Cache.MaxBodyBytes == 0 {
			r.Cache.MaxBodyBytes = 1 << 20
		}
		if r.Cache.MaxBodyBytes < 1 || r.Cache.MaxBodyBytes > 64<<20 {
			return nil, fmt.Errorf("maxBodyBytes must be between 1 and 67108864")
		}
		if r.Cache.Revalidation.Mode == "" {
			r.Cache.Revalidation.Mode = "on-expiry"
		}
		if r.Cache.Revalidation.Mode != "on-expiry" && r.Cache.Revalidation.Mode != "always" {
			return nil, fmt.Errorf("invalid revalidation mode")
		}
		for _, h := range r.Cache.KeyHeaders {
			if !validHeaderName(h) {
				return nil, fmt.Errorf("invalid key header")
			}
		}
		for _, ct := range r.Response.ContentTypes {
			if ct != "application/*+json" {
				if _, _, err := mime.ParseMediaType(ct); err != nil || strings.Contains(ct, "*") {
					return nil, fmt.Errorf("invalid content type %q", ct)
				}
			}
		}
		p, err := compileMatch(r.Request.Path.Type, r.Request.Path.Value, false)
		if err != nil {
			return nil, err
		}
		rh, err := compileHeaders(r.Request.Headers)
		if err != nil {
			return nil, err
		}
		sh, err := compileHeaders(r.Response.Headers)
		if err != nil {
			return nil, err
		}
		ttl, err := time.ParseDuration(r.Cache.TTL)
		if err != nil || ttl <= 0 {
			return nil, fmt.Errorf("invalid ttl")
		}
		ret, err := time.ParseDuration(r.Cache.Retention)
		if err != nil || ret < ttl || ret > 24*time.Hour {
			return nil, fmt.Errorf("retention must be >= ttl and <= 24h")
		}
		result = append(result, compiledRule{config: r, path: p, requestHeaders: rh, responseHeaders: sh, ttl: ttl, retention: ret})
	}
	return result, nil
}
func (r compiledRule) responseMatches(h http.Header) bool {
	if !headersMatch(h, r.responseHeaders) {
		return false
	}
	if len(r.config.Response.ContentTypes) == 0 {
		return true
	}
	ct, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		return false
	}
	for _, allowed := range r.config.Response.ContentTypes {
		if strings.EqualFold(ct, allowed) || allowed == "application/*+json" && strings.HasPrefix(ct, "application/") && strings.HasSuffix(ct, "+json") {
			return true
		}
	}
	return false
}
