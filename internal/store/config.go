// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package store

// MemoryConfig limits the local adapter's serialized entries.
type MemoryConfig struct {
	MaxEntries int
	MaxBytes   int64
}

// RedisConfig configures a standalone Redis connection.
type RedisConfig struct {
	Address      string
	Username     string
	PasswordFile string
	DB           int
	Timeout      string
	TLS          bool
	ServerName   string
}
