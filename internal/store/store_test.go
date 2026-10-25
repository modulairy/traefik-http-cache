// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func storeContract(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()
	scope := fmt.Sprintf("test-%d", time.Now().UnixNano())
	ret := time.Second
	_, g, err := s.Read(ctx, scope, "k", ret)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Write(ctx, scope, "k", g, []byte("body"), ret); err != nil {
		t.Fatal(err)
	}
	b, _, err := s.Read(ctx, scope, "k", ret)
	if err != nil || string(b) != "body" {
		t.Fatal(string(b), err)
	}
	if err = s.Invalidate(ctx, scope, ret); err != nil {
		t.Fatal(err)
	}
	if err = s.Write(ctx, scope, "k", g, []byte("stale"), ret); err != nil {
		t.Fatal(err)
	}
	b, g, err = s.Read(ctx, scope, "k", ret)
	if err != nil || len(b) != 0 {
		t.Fatal("stale writer resurrected cache", err)
	}
	if err = s.Write(ctx, scope, "k", g, []byte("new"), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	b, _, err = s.Read(ctx, scope, "k", ret)
	if err != nil || len(b) != 0 {
		t.Fatal("expired data", err)
	}
}
func TestMemoryStore(t *testing.T) {
	s, err := NewMemory(MemoryConfig{MaxEntries: 10, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	storeContract(t, s)
}
func TestMemoryBounds(t *testing.T) {
	s, _ := NewMemory(MemoryConfig{MaxEntries: 2, MaxBytes: 4})
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		scope := fmt.Sprint(i)
		_, g, _ := s.Read(ctx, scope, "k", time.Minute)
		_ = s.Write(ctx, scope, "k", g, []byte("1234"), time.Minute)
	}
	if s.bytes > 4 || s.entries > 2 || len(s.scopes) > 2 {
		t.Fatal("unbounded memory")
	}
}
func TestRedisStore(t *testing.T) {
	address := os.Getenv("REDIS_ADDR")
	if address == "" {
		t.Skip("set REDIS_ADDR for real Redis contract test")
	}
	s, err := NewRedis(RedisConfig{Address: address, Timeout: "1s"}, "test-contract")
	if err != nil {
		t.Fatal(err)
	}
	storeContract(t, s)
}
