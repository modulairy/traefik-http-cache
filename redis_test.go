// Copyright 2026 Modulairy contributors.
// SPDX-License-Identifier: Apache-2.0

package traefik_http_cache

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRESPRejectsMalformedAndOversizedReplies(t *testing.T) {
	for _, reply := range []string{"?bad\r\n", "$-2\r\n", "$1000000000\r\n", "*100\r\n", "$3\r\nabcxx", "+" + strings.Repeat("x", 8192) + "\r\n", "-ERR secret must not be echoed\r\n"} {
		_, err := readRESP(bufio.NewReader(strings.NewReader(reply)), 0)
		if err == nil {
			t.Fatalf("accepted malformed reply %q", reply[:2])
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("server error leaked")
		}
	}
}

func TestRedisAuthenticationAndDatabase(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	passwordPath := filepath.Join(t.TempDir(), "password")
	if err = os.WriteFile(passwordPath, []byte("test-password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commands := make(chan [][]string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			commands <- nil
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		var result [][]string
		for i := 0; i < 3; i++ {
			v, err := readRESP(reader, 0)
			if err != nil {
				commands <- nil
				return
			}
			a := v.([]interface{})
			var args []string
			for _, v := range a {
				args = append(args, v.(string))
			}
			result = append(result, args)
			_, _ = conn.Write([]byte("+OK\r\n"))
		}
		commands <- result
	}()
	s, err := newRedis(RedisConfig{Address: listener.Addr().String(), Username: "cache-user", PasswordFile: passwordPath, DB: 2, Timeout: "1s"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.command(context.Background(), "PING"); err != nil {
		t.Fatal(err)
	}
	got := <-commands
	if len(got) != 3 || strings.Join(got[0], " ") != "AUTH cache-user test-password" || strings.Join(got[1], " ") != "SELECT 2" || got[2][0] != "PING" {
		t.Fatal("wrong authentication/database sequence")
	}
}
