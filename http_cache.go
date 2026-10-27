// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package traefik_http_cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modulairy/traefik-http-cache/internal/store"
)

const Version = "0.1.3"

type entry struct {
	Header     http.Header
	Body       []byte
	Stored     time.Time
	InitialAge float64
	FreshFor   float64
}
type middleware struct {
	next           http.Handler
	store          store.Store
	rules          []compiledRule
	prefix, policy string
	retention      time.Duration
	mu             sync.RWMutex
	disabledUntil  time.Time
}

func New(_ context.Context, next http.Handler, c *Config, name string) (http.Handler, error) {
	if c == nil || next == nil {
		return nil, fmt.Errorf("config and next handler are required")
	}
	if c.KeyPrefix == "" {
		return nil, fmt.Errorf("keyPrefix must identify the application/cache policy")
	}
	rules, err := compileRules(c.Rules)
	if err != nil {
		return nil, err
	}
	var storage store.Store
	switch c.Store.Type {
	case "memory":
		memory, memoryErr := store.NewMemory(store.MemoryConfig{MaxEntries: c.Store.Memory.MaxEntries, MaxBytes: c.Store.Memory.MaxBytes})
		if memoryErr != nil {
			return nil, memoryErr
		}
		storage = memory
	case "redis":
		cfg := c.Store.Redis
		redis, redisErr := store.NewRedis(store.RedisConfig{Address: cfg.Address, Username: cfg.Username, PasswordFile: cfg.PasswordFile, DB: cfg.DB, Timeout: cfg.Timeout, TLS: cfg.TLS, ServerName: cfg.ServerName}, "thc:"+digest(c.KeyPrefix))
		if redisErr != nil {
			return nil, redisErr
		}
		storage = redis
	default:
		return nil, fmt.Errorf("unknown store type %q", c.Store.Type)
	}
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(c.Rules)
	m := &middleware{next: next, store: storage, rules: rules, prefix: c.KeyPrefix, policy: digest(string(b))}
	for _, r := range rules {
		if r.retention > m.retention {
			m.retention = r.retention
		}
	}
	return m, nil
}
func digest(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
func directives(h http.Header) map[string]string {
	m := make(map[string]string)
	for _, line := range headerValues(h, "Cache-Control") {
		for _, part := range splitComma(line) {
			kv := strings.SplitN(part, "=", 2)
			key := strings.ToLower(strings.TrimSpace(kv[0]))
			value := ""
			if len(kv) == 2 {
				value = strings.Trim(strings.TrimSpace(kv[1]), "\"")
			}
			if _, ok := m[key]; ok {
				m["invalid"] = ""
			}
			m[key] = value
		}
	}
	return m
}
func has(m map[string]string, key string) bool { _, ok := m[key]; return ok }
func present(h http.Header, key string) bool {
	for name := range h {
		if strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}

// Preserve each field value and its order; never normalize encoding qweights.
func headerValues(h http.Header, key string) []string {
	var names []string
	for name := range h {
		if strings.EqualFold(name, key) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var values []string
	for _, name := range names {
		values = append(values, h[name]...)
	}
	return values
}

func ambiguousHeaders(h http.Header, names []string) bool {
	for _, name := range names {
		count := 0
		for field := range h {
			if strings.EqualFold(field, name) {
				count++
			}
		}
		if count > 1 {
			return true
		}
	}
	return false
}

// An empty opt-in preserves the original rejection of every Vary field.
func varyFields(h http.Header, rule compiledRule) ([]string, bool) {
	if !present(h, "Vary") {
		return nil, true
	}
	if len(rule.varyHeaders) == 0 {
		return nil, false
	}
	seen := make(map[string]bool)
	for _, line := range headerValues(h, "Vary") {
		for _, field := range splitComma(line) {
			if !validHeaderName(field) || field == "*" {
				return nil, false
			}
			name := http.CanonicalHeaderKey(field)
			allowed := false
			for _, h := range rule.varyHeaders {
				allowed = allowed || name == h
			}
			if !allowed {
				return nil, false
			}
			seen[name] = true
		}
	}
	if len(seen) == 0 {
		return nil, false
	}
	var fields []string
	for name := range seen {
		fields = append(fields, name)
	}
	sort.Strings(fields)
	return fields, true
}

func reusable304(old, update http.Header, rule compiledRule) bool {
	before, ok := varyFields(old, rule)
	if !ok {
		return false
	}
	if present(update, "Vary") {
		after, valid := varyFields(update, rule)
		if !valid || strings.Join(before, ",") != strings.Join(after, ",") {
			return false
		}
	}
	if present(update, "Content-Encoding") && strings.Join(headerValues(old, "Content-Encoding"), "\n") != strings.Join(headerValues(update, "Content-Encoding"), "\n") {
		return false
	}
	return true
}
func bypass(r *http.Request) bool {
	for _, h := range []string{"Authorization", "Cookie", "Range", "If-Match", "If-None-Match", "If-Modified-Since", "If-Unmodified-Since", "If-Range", "Upgrade"} {
		if present(r.Header, h) {
			return true
		}
	}
	return r.Method != "GET" || has(directives(r.Header), "no-store") || r.Header.Get("Pragma") != ""
}
func (m *middleware) scope(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return digest(m.prefix + "\n" + scheme + "\n" + r.Host + "\n" + r.URL.RequestURI())
}
func (m *middleware) key(r *http.Request, rule compiledRule) string {
	type headerKey struct {
		Name    string
		Present bool
		Values  []string
	}
	values := struct {
		Policy, Rule, Method string
		Headers              []headerKey
	}{Policy: m.policy, Rule: rule.config.Name, Method: r.Method}
	for _, h := range rule.keyHeaders {
		values.Headers = append(values.Headers, headerKey{Name: http.CanonicalHeaderKey(h), Present: present(r.Header, h), Values: headerValues(r.Header, h)})
	}
	b, _ := json.Marshal(values)
	if len(rule.varyHeaders) > 0 {
		// Isolate opt-in entries from pre-support policy/key formats.
		return digest("vary-v1\n" + string(b))
	}
	return digest(string(b))
}
func (m *middleware) disabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Now().Before(m.disabledUntil)
}
func (m *middleware) invalidate(ctx context.Context, scope string) {
	if err := m.store.Invalidate(ctx, scope, m.retention); err != nil {
		m.mu.Lock()
		m.disabledUntil = time.Now().Add(m.retention)
		m.mu.Unlock()
	}
}
func unsafeMethod(method string) bool {
	return method != "GET" && method != "HEAD" && method != "OPTIONS" && method != "TRACE"
}
func (m *middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	scope := m.scope(r)
	onlyCached := has(directives(r.Header), "only-if-cached")
	forward := func() {
		if onlyCached {
			http.Error(w, "no usable cached response", http.StatusGatewayTimeout)
		} else {
			m.next.ServeHTTP(w, r)
		}
	}
	if unsafeMethod(r.Method) {
		c := newCapture(w, 0)
		m.next.ServeHTTP(c, r)
		if c.status == 0 {
			c.WriteHeader(200)
		}
		if c.status >= 200 && c.status < 400 {
			m.invalidate(r.Context(), scope)
		}
		c.commit()
		return
	}
	if bypass(r) || m.disabled() {
		forward()
		return
	}
	var rule *compiledRule
	for i := range m.rules {
		candidate := &m.rules[i]
		if candidate.path.matches(r.URL.Path) && headersMatch(r.Header, candidate.requestHeaders) {
			rule = candidate
			break
		}
	}
	if rule == nil {
		forward()
		return
	}
	if ambiguousHeaders(r.Header, rule.keyHeaders) {
		forward()
		return
	}
	key := m.key(r, *rule)
	data, gen, err := m.store.Read(r.Context(), scope, key, rule.retention)
	if err != nil {
		w.Header().Set("X-Cache-Status", "BYPASS")
		forward()
		return
	}
	var cached *entry
	if len(data) > 0 {
		var e entry
		if json.Unmarshal(data, &e) == nil && !e.Stored.IsZero() {
			if _, allowed := varyFields(e.Header, *rule); allowed {
				cached = &e
			} else {
				m.invalidate(r.Context(), scope)
			}
		}
	}
	if cached != nil && fresh(cached, r, rule) {
		serve(w, cached, "HIT")
		return
	}
	if onlyCached {
		http.Error(w, "no usable cached response", http.StatusGatewayTimeout)
		return
	}
	origin := r.Clone(r.Context())
	origin.Header = canonicalHeaders(r.Header)
	request := origin.Clone(r.Context())
	conditional := false
	if cached != nil {
		if tag := cached.Header.Get("ETag"); tag != "" {
			request.Header.Set("If-None-Match", tag)
			conditional = true
		}
		if lm := cached.Header.Get("Last-Modified"); lm != "" {
			request.Header.Set("If-Modified-Since", lm)
			conditional = true
		}
	}
	c := newCapture(w, rule.config.Cache.MaxBodyBytes)
	started := time.Now()
	m.next.ServeHTTP(c, request)
	if c.status == 0 {
		c.WriteHeader(200)
	}
	if c.streamed {
		if cached != nil {
			m.invalidate(r.Context(), scope)
		}
		return
	}
	c.header = c.frozen
	if c.status == 304 && cached != nil && conditional {
		if !reusable304(cached.Header, c.header, *rule) {
			m.invalidate(r.Context(), scope)
			m.next.ServeHTTP(w, origin)
			return
		}
		// A contradictory validator must never refresh the stored representation.
		if tag := c.header.Get("ETag"); tag != "" && cached.Header.Get("ETag") != "" && strings.TrimPrefix(tag, "W/") != strings.TrimPrefix(cached.Header.Get("ETag"), "W/") {
			m.invalidate(r.Context(), scope)
			m.next.ServeHTTP(w, origin)
			return
		}
		h := cached.Header.Clone()
		for k, v := range cleanHeaders(c.header) {
			if k != "Content-Length" {
				h[k] = v
			}
		}
		// A 304 without Date/Age starts a new validated age, not the previous age.
		if !present(c.header, "Date") {
			h.Set("Date", time.Now().UTC().Format(http.TimeFormat))
		}
		if !present(c.header, "Age") {
			h.Del("Age")
		}
		e, ok := makeEntry(h, cached.Body, *rule, started)
		if ok {
			m.save(r.Context(), scope, key, gen, e, rule.retention)
		} else {
			m.invalidate(r.Context(), scope)
		}
		if e == nil {
			e = &entry{Header: cleanHeaders(h), Body: cached.Body, Stored: time.Now()}
		}
		serve(w, e, "REVALIDATED")
		return
	}
	if c.status == 200 {
		if e, ok := makeEntry(c.header, c.body.Bytes(), *rule, started); ok {
			m.save(r.Context(), scope, key, gen, e, rule.retention)
		} else if cached != nil {
			m.invalidate(r.Context(), scope)
		}
	} else if cached != nil && c.status < 500 {
		m.invalidate(r.Context(), scope)
	}
	c.header.Set("X-Cache-Status", "MISS")
	c.commit()
}
func (m *middleware) save(ctx context.Context, scope, key, gen string, e *entry, ret time.Duration) {
	b, err := json.Marshal(e)
	if err == nil {
		_ = m.store.Write(ctx, scope, key, gen, b, ret)
	}
}
func age(e *entry) float64 {
	a := e.InitialAge + time.Since(e.Stored).Seconds()
	if a < 0 {
		return 0
	}
	return a
}
func fresh(e *entry, r *http.Request, rule *compiledRule) bool {
	if rule.config.Cache.Revalidation.Mode == "always" {
		return false
	}
	d := directives(r.Header)
	if has(d, "no-cache") || has(d, "invalid") {
		return false
	}
	a := age(e)
	limit := e.FreshFor
	if v, ok := d["max-age"]; ok {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return false
		}
		if float64(n) < limit {
			limit = float64(n)
		}
	}
	if v, ok := d["min-fresh"]; ok {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return false
		}
		a += float64(n)
	}
	return a < limit
}
func makeEntry(h http.Header, body []byte, rule compiledRule, started time.Time) (*entry, bool) {
	d := directives(h)
	_, varyOK := varyFields(h, rule)
	if has(d, "private") || has(d, "no-store") || has(d, "invalid") || has(d, "must-understand") || present(h, "Set-Cookie") || !varyOK || present(h, "Trailer") || !rule.responseMatches(h) {
		return nil, false
	}
	if ct := strings.ToLower(h.Get("Content-Type")); strings.HasPrefix(ct, "text/event-stream") || strings.HasPrefix(ct, "application/grpc") {
		return nil, false
	}
	if len(body) > rule.config.Cache.MaxBodyBytes {
		return nil, false
	}
	if cl := h.Get("Content-Length"); cl != "" {
		n, err := strconv.Atoi(cl)
		if err != nil || n != len(body) {
			return nil, false
		}
	}
	now := time.Now()
	ttl := rule.ttl.Seconds()
	date := now
	if value := h.Get("Date"); value != "" {
		parsed, err := http.ParseTime(value)
		if err != nil {
			return nil, false
		}
		date = parsed
	}
	if v, ok := d["s-maxage"]; ok {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, false
		}
		if float64(n) < ttl {
			ttl = float64(n)
		}
	} else if v, ok := d["max-age"]; ok {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, false
		}
		if float64(n) < ttl {
			ttl = float64(n)
		}
	} else if v := h.Get("Expires"); v != "" {
		t, err := http.ParseTime(v)
		if err != nil {
			return nil, false
		}
		if seconds := t.Sub(date).Seconds(); seconds < ttl {
			ttl = seconds
		}
	}
	if has(d, "no-cache") {
		ttl = 0
	}
	initial := now.Sub(date).Seconds()
	if initial < 0 {
		initial = 0
	}
	a := 0.0
	if v := h.Get("Age"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, false
		}
		a = float64(n)
	}
	a += now.Sub(started).Seconds()
	if a > initial {
		initial = a
	}
	n := cleanHeaders(h)
	if n.Get("Date") == "" {
		n.Set("Date", now.UTC().Format(http.TimeFormat))
	}
	return &entry{Header: n, Body: append([]byte(nil), body...), Stored: now, InitialAge: initial, FreshFor: ttl}, true
}
func serve(w http.ResponseWriter, e *entry, status string) {
	copyHeaders(w.Header(), e.Header)
	w.Header().Set("Age", strconv.FormatInt(int64(age(e)), 10))
	w.Header().Set("X-Cache-Status", status)
	w.WriteHeader(200)
	_, _ = w.Write(e.Body)
}
