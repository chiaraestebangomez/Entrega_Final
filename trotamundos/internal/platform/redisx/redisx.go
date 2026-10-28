package redisx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrNil = errors.New("redis: nil")

type Client struct {
	addr     string
	password string
	db       int
	timeout  time.Duration

	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

func New(addr, password string, db int) *Client {
	return &Client{addr: addr, password: password, db: db, timeout: 3 * time.Second}
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Do(ctx, "PING")
	return err
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	v, err := c.Do(ctx, "GET", key)
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", ErrNil
	}
	return v.(string), nil
}

func (c *Client) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	args := []any{"SET", key, value}
	if ttl > 0 {
		args = append(args, "PX", strconv.FormatInt(ttl.Milliseconds(), 10))
	}
	_, err := c.Do(ctx, args...)
	return err
}

func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	args := []any{"DEL"}
	for _, k := range keys {
		args = append(args, k)
	}
	_, err := c.Do(ctx, args...)
	return err
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn, c.r = nil, nil
	return err
}

func (c *Client) Do(ctx context.Context, args ...any) (any, error) {
	if len(args) == 0 {
		return nil, errors.New("redis: empty command")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := c.ensureConn(deadline); err != nil {
		return nil, err
	}
	_ = c.conn.SetDeadline(deadline)
	if err := writeCommand(c.conn, args); err != nil {
		c.reset()
		return nil, err
	}
	v, err := readReply(c.r)
	if err != nil {
		c.reset()
		return nil, err
	}
	return v, nil
}

func (c *Client) ensureConn(deadline time.Time) error {
	if c.conn != nil {
		return nil
	}
	d := net.Dialer{Deadline: deadline}
	conn, err := d.Dial("tcp", c.addr)
	if err != nil {
		return err
	}
	c.conn = conn
	c.r = bufio.NewReader(conn)
	if c.password != "" {
		if err := writeCommand(conn, []any{"AUTH", c.password}); err != nil {
			c.reset()
			return err
		}
		if _, err := readReply(c.r); err != nil {
			c.reset()
			return err
		}
	}
	if c.db != 0 {
		if err := writeCommand(conn, []any{"SELECT", strconv.Itoa(c.db)}); err != nil {
			c.reset()
			return err
		}
		if _, err := readReply(c.r); err != nil {
			c.reset()
			return err
		}
	}
	return nil
}

func (c *Client) reset() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn, c.r = nil, nil
}

func writeCommand(w io.Writer, args []any) error {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		s := fmt.Sprint(a)
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(s), s)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func readReply(r *bufio.Reader) (any, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == "" {
		return nil, errors.New("redis: empty reply")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, errors.New(line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n == -1 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n == -1 {
			return nil, nil
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			v, err := readReply(r)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("redis: unexpected reply %q", line)
	}
}
