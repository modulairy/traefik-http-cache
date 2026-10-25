package traefik_http_cache

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
)

// capture buffers only bounded, non-streaming responses. Flush commits passthrough.
type capture struct {
	out      http.ResponseWriter
	header   http.Header
	frozen   http.Header
	status   int
	body     bytes.Buffer
	limit    int
	streamed bool
}

func newCapture(w http.ResponseWriter, limit int) *capture {
	return &capture{out: w, header: w.Header().Clone(), limit: limit}
}
func (c *capture) Header() http.Header {
	if c.streamed {
		return c.out.Header()
	}
	return c.header
}
func (c *capture) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		copyHeaders(c.out.Header(), c.header)
		c.out.WriteHeader(code)
		return
	}
	if c.status != 0 {
		return
	}
	c.status = code
	c.frozen = c.header.Clone()
	if c.streamed {
		c.out.WriteHeader(code)
	}
}
func (c *capture) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.WriteHeader(http.StatusOK)
	}
	if c.streamed {
		return c.out.Write(b)
	}
	if c.body.Len()+len(b) > c.limit {
		c.commit()
		return c.out.Write(b)
	}
	return c.body.Write(b)
}
func (c *capture) commit() {
	if c.streamed {
		return
	}
	if c.status == 0 {
		c.WriteHeader(200)
	}
	c.streamed = true
	copyHeaders(c.out.Header(), c.frozen)
	c.out.WriteHeader(c.status)
	_, _ = c.out.Write(c.body.Bytes())
	c.body.Reset()
}
func (c *capture) Flush() {
	c.commit()
	if f, ok := c.out.(http.Flusher); ok {
		f.Flush()
	}
}
func (c *capture) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := c.out.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacking unsupported")
	}
	c.streamed = true
	return h.Hijack()
}
func (c *capture) Unwrap() http.ResponseWriter { return c.out }
func copyHeaders(dst, src http.Header) {
	for k := range dst {
		delete(dst, k)
	}
	for k, v := range src {
		dst[k] = append([]string(nil), v...)
	}
}
func cleanHeaders(h http.Header) http.Header {
	n := h.Clone()
	for _, line := range h.Values("Connection") {
		for _, name := range splitComma(line) {
			n.Del(name)
		}
	}
	for _, name := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailer", "Transfer-Encoding", "Upgrade", "X-Cache-Status"} {
		n.Del(name)
	}
	return n
}
