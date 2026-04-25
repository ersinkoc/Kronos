package redis

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
)

// Client is a small RESP client used by the Redis driver.
type Client struct {
	rw     io.ReadWriter
	reader *Reader
	mu     sync.Mutex
}

// Dial connects to a Redis-compatible TCP endpoint.
func Dial(ctx context.Context, address string) (*Client, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	return NewClient(conn), nil
}

// NewClient wraps an existing RESP stream.
func NewClient(rw io.ReadWriter) *Client {
	return &Client{rw: rw, reader: NewReader(rw)}
}

// Do sends one command and returns its response.
func (c *Client) Do(ctx context.Context, args ...string) (Value, error) {
	if c == nil || c.rw == nil {
		return Value{}, fmt.Errorf("redis client is closed")
	}
	if err := ctx.Err(); err != nil {
		return Value{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.rw.Write(EncodeCommand(args...)); err != nil {
		return Value{}, err
	}
	value, err := c.reader.ReadValue()
	if err != nil {
		return Value{}, err
	}
	if value.Type == TypeError {
		return Value{}, RedisError(value.String)
	}
	return value, nil
}

// Auth authenticates with password-only or ACL username/password credentials.
func (c *Client) Auth(ctx context.Context, username string, password string) error {
	if password == "" {
		return nil
	}
	var value Value
	var err error
	if username == "" {
		value, err = c.Do(ctx, "AUTH", password)
	} else {
		value, err = c.Do(ctx, "AUTH", username, password)
	}
	if err != nil {
		return err
	}
	if value.Type != TypeSimpleString || value.String != "OK" {
		return fmt.Errorf("unexpected AUTH response: %#v", value)
	}
	return nil
}

// Hello selects RESP version and optionally authenticates in one round trip.
func (c *Client) Hello(ctx context.Context, version int, username string, password string) (Value, error) {
	if version != 2 && version != 3 {
		return Value{}, fmt.Errorf("unsupported RESP version %d", version)
	}
	args := []string{"HELLO", fmt.Sprintf("%d", version)}
	if password != "" {
		args = append(args, "AUTH")
		if username == "" {
			args = append(args, "default", password)
		} else {
			args = append(args, username, password)
		}
	}
	return c.Do(ctx, args...)
}

// RedisError is an error response returned by Redis.
type RedisError string

func (e RedisError) Error() string {
	return string(e)
}
