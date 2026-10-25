// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

// Package store provides opaque cache storage with URI generation checks.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Store rejects writes racing with invalidation. Read returns a generation even
// on a miss. Returned data belongs to the caller. HTTP policy is not stored here.
type Store interface {
	Read(context.Context, string, string, time.Duration) ([]byte, string, error)
	Write(context.Context, string, string, string, []byte, time.Duration) error
	Invalidate(context.Context, string, time.Duration) error
}

func token() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
