// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package store

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// RESP2 is deliberately limited to the commands used by this standalone adapter.
// Connections have bounded lifetimes; no goroutine or pool survives a reload.
type redisStore struct {
	config   RedisConfig
	timeout  time.Duration
	password string
	prefix   string
}

func NewRedis(c RedisConfig, prefix string) (*redisStore, error) {
	if c.Address == "" || c.DB < 0 {
		return nil, fmt.Errorf("redis address and nonnegative db required")
	}
	if c.Timeout == "" {
		c.Timeout = "200ms"
	}
	d, err := time.ParseDuration(c.Timeout)
	if err != nil || d <= 0 || d > 10*time.Second {
		return nil, fmt.Errorf("redis timeout must be >0 and <=10s")
	}
	password := ""
	if c.PasswordFile != "" {
		b, err := os.ReadFile(c.PasswordFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read redis password file")
		}
		password = strings.TrimRight(string(b), "\r\n")
	}
	return &redisStore{config: c, timeout: d, password: password, prefix: prefix}, nil
}
func writeCommand(w io.Writer, args []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
func readRESP(r *bufio.Reader, depth int) (interface{}, error) {
	if depth > 2 {
		return nil, fmt.Errorf("redis response nesting exceeded")
	}
	lineBytes, err := r.ReadSlice('\n')
	if err != nil {
		return nil, err
	}
	line := string(lineBytes)
	if len(line) < 3 || len(line) > 4096 || !strings.HasSuffix(line, "\r\n") {
		return nil, fmt.Errorf("invalid redis response")
	}
	s := line[1 : len(line)-2]
	switch line[0] {
	case '+':
		return s, nil
	case '-':
		return nil, fmt.Errorf("redis command failed")
	case ':':
		return strconv.ParseInt(s, 10, 64)
	case '$':
		n, err := strconv.Atoi(s)
		if err != nil || n < -1 || n > 100<<20 {
			return nil, fmt.Errorf("invalid redis bulk length")
		}
		if n == -1 {
			return nil, nil
		}
		b := make([]byte, n+2)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, err
		}
		if b[n] != '\r' || b[n+1] != '\n' {
			return nil, fmt.Errorf("invalid redis terminator")
		}
		return string(b[:n]), nil
	case '*':
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n > 8 {
			return nil, fmt.Errorf("invalid redis array")
		}
		a := make([]interface{}, n)
		for i := range a {
			a[i], err = readRESP(r, depth+1)
			if err != nil {
				return nil, err
			}
		}
		return a, nil
	}
	return nil, fmt.Errorf("unsupported redis response")
}
func (s *redisStore) command(ctx context.Context, args ...string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	d := &net.Dialer{Timeout: s.timeout}
	conn, err := d.DialContext(ctx, "tcp", s.config.Address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	if s.config.TLS {
		host, _, e := net.SplitHostPort(s.config.Address)
		if e != nil {
			return nil, e
		}
		if s.config.ServerName != "" {
			host = s.config.ServerName
		}
		t := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host})
		if err = t.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = t
	}
	r := bufio.NewReader(conn)
	if s.password != "" {
		auth := []string{"AUTH", s.password}
		if s.config.Username != "" {
			auth = []string{"AUTH", s.config.Username, s.password}
		}
		if err = writeCommand(conn, auth); err != nil {
			return nil, err
		}
		if _, err = readRESP(r, 0); err != nil {
			return nil, err
		}
	}
	if s.config.DB != 0 {
		if err = writeCommand(conn, []string{"SELECT", strconv.Itoa(s.config.DB)}); err != nil {
			return nil, err
		}
		if _, err = readRESP(r, 0); err != nil {
			return nil, err
		}
	}
	if err = writeCommand(conn, args); err != nil {
		return nil, err
	}
	return readRESP(r, 0)
}

const redisRead = `local g=redis.call('GET',KEYS[1]); if not g then g=ARGV[1]; redis.call('SET',KEYS[1],g,'PX',ARGV[2]); elseif redis.call('PTTL',KEYS[1]) < tonumber(ARGV[2]) then redis.call('PEXPIRE',KEYS[1],ARGV[2]); end; return {g,redis.call('GET',KEYS[2]..':'..g) or ''}`
const redisWrite = `if redis.call('GET',KEYS[1])~=ARGV[1] then return 0 end; redis.call('SET',KEYS[2]..':'..ARGV[1],ARGV[2],'PX',ARGV[3]); if redis.call('PTTL',KEYS[1]) < tonumber(ARGV[3]) then redis.call('PEXPIRE',KEYS[1],ARGV[3]); end; return 1`

func (s *redisStore) keys(scope, key string) (string, string) {
	base := s.prefix + ":" + scope
	return base + ":generation", base + ":" + key
}
func (s *redisStore) Read(ctx context.Context, scope, key string, ret time.Duration) ([]byte, string, error) {
	gen, err := token()
	if err != nil {
		return nil, "", err
	}
	g, k := s.keys(scope, key)
	v, err := s.command(ctx, "EVAL", redisRead, "2", g, k, gen, strconv.FormatInt(ret.Milliseconds(), 10))
	if err != nil {
		return nil, "", err
	}
	a, ok := v.([]interface{})
	if !ok || len(a) != 2 {
		return nil, "", fmt.Errorf("invalid cache response")
	}
	generation, ok := a[0].(string)
	if !ok {
		return nil, "", fmt.Errorf("invalid generation")
	}
	data, ok := a[1].(string)
	if !ok {
		return nil, "", fmt.Errorf("invalid cache data")
	}
	return []byte(data), generation, nil
}
func (s *redisStore) Write(ctx context.Context, scope, key, gen string, data []byte, ret time.Duration) error {
	g, k := s.keys(scope, key)
	_, err := s.command(ctx, "EVAL", redisWrite, "2", g, k, gen, string(data), strconv.FormatInt(ret.Milliseconds(), 10))
	return err
}
func (s *redisStore) Invalidate(ctx context.Context, scope string, ret time.Duration) error {
	gen, err := token()
	if err != nil {
		return err
	}
	g, _ := s.keys(scope, "")
	_, err = s.command(ctx, "SET", g, gen, "PX", strconv.FormatInt(ret.Milliseconds(), 10))
	return err
}
