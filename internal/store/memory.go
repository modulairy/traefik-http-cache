// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type memoryValue struct {
	data    []byte
	expires time.Time
}
type memoryScope struct {
	generation string
	expires    time.Time
	values     map[string]memoryValue
}
type memoryStore struct {
	mu         sync.Mutex
	scopes     map[string]*memoryScope
	maxEntries int
	maxBytes   int64
	bytes      int64
	entries    int
}

func NewMemory(c MemoryConfig) (*memoryStore, error) {
	if c.MaxEntries <= 0 || c.MaxBytes <= 0 {
		return nil, fmt.Errorf("memory limits must be positive")
	}
	return &memoryStore{scopes: make(map[string]*memoryScope), maxEntries: c.MaxEntries, maxBytes: c.MaxBytes}, nil
}
func (m *memoryStore) remove(scope string) {
	s := m.scopes[scope]
	if s == nil {
		return
	}
	for _, v := range s.values {
		m.bytes -= int64(len(v.data))
		m.entries--
	}
	delete(m.scopes, scope)
}
func (m *memoryStore) clean(now time.Time) {
	for k, s := range m.scopes {
		if !now.Before(s.expires) {
			m.remove(k)
			continue
		}
		for key, v := range s.values {
			if !now.Before(v.expires) {
				delete(s.values, key)
				m.bytes -= int64(len(v.data))
				m.entries--
			}
		}
	}
}
func (m *memoryStore) Read(ctx context.Context, scope, key string, retention time.Duration) ([]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.clean(now)
	s := m.scopes[scope]
	if s == nil {
		for len(m.scopes) >= m.maxEntries {
			for k := range m.scopes {
				m.remove(k)
				break
			}
		}
		gen, err := token()
		if err != nil {
			return nil, "", err
		}
		s = &memoryScope{generation: gen, expires: now.Add(retention), values: make(map[string]memoryValue)}
		m.scopes[scope] = s
	}
	if until := now.Add(retention); until.After(s.expires) {
		s.expires = until
	}
	v := s.values[key]
	return append([]byte(nil), v.data...), s.generation, nil
}
func (m *memoryStore) Write(ctx context.Context, scope, key, generation string, data []byte, retention time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clean(time.Now())
	s := m.scopes[scope]
	if s == nil || s.generation != generation {
		return nil
	}
	if int64(len(data)) > m.maxBytes {
		return nil
	}
	if old, ok := s.values[key]; ok {
		m.bytes -= int64(len(old.data))
		m.entries--
		delete(s.values, key)
	}
	// At capacity, skip new writes rather than evicting active generations.
	if m.entries >= m.maxEntries || m.bytes+int64(len(data)) > m.maxBytes {
		return nil
	}
	s.values[key] = memoryValue{data: append([]byte(nil), data...), expires: time.Now().Add(retention)}
	s.expires = time.Now().Add(retention)
	m.bytes += int64(len(data))
	m.entries++
	return nil
}
func (m *memoryStore) Invalidate(ctx context.Context, scope string, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remove(scope)
	return nil
}
