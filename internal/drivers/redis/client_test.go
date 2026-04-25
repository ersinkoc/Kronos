package redis

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestClientDo(t *testing.T) {
	t.Parallel()

	conn := &scriptedConn{responses: bytes.NewBufferString("+PONG\r\n")}
	client := NewClient(conn)
	value, err := client.Do(context.Background(), "PING")
	if err != nil {
		t.Fatalf("Do(PING) error = %v", err)
	}
	if value.Type != TypeSimpleString || value.String != "PONG" {
		t.Fatalf("Do(PING) = %#v", value)
	}
	if got := conn.writes.String(); got != "*1\r\n$4\r\nPING\r\n" {
		t.Fatalf("written command = %q", got)
	}
}

func TestClientAuthAndHello(t *testing.T) {
	t.Parallel()

	conn := &scriptedConn{responses: bytes.NewBufferString("+OK\r\n*2\r\n$6\r\nserver\r\n$5\r\nredis\r\n")}
	client := NewClient(conn)
	if err := client.Auth(context.Background(), "alice", "secret"); err != nil {
		t.Fatalf("Auth() error = %v", err)
	}
	value, err := client.Hello(context.Background(), 3, "", "secret")
	if err != nil {
		t.Fatalf("Hello() error = %v", err)
	}
	if value.Type != TypeArray || len(value.Array) != 2 {
		t.Fatalf("Hello() = %#v", value)
	}
	writes := conn.writes.String()
	if !strings.Contains(writes, "$4\r\nAUTH\r\n$5\r\nalice\r\n$6\r\nsecret\r\n") {
		t.Fatalf("AUTH write missing in %q", writes)
	}
	if !strings.Contains(writes, "$5\r\nHELLO\r\n$1\r\n3\r\n$4\r\nAUTH\r\n$7\r\ndefault\r\n$6\r\nsecret\r\n") {
		t.Fatalf("HELLO write missing in %q", writes)
	}
}

func TestClientReturnsRedisError(t *testing.T) {
	t.Parallel()

	conn := &scriptedConn{responses: bytes.NewBufferString("-NOAUTH Authentication required\r\n")}
	client := NewClient(conn)
	if _, err := client.Do(context.Background(), "PING"); err == nil {
		t.Fatal("Do() error = nil, want RedisError")
	} else {
		var redisErr RedisError
		if !errors.As(err, &redisErr) {
			t.Fatalf("Do() error = %T, want RedisError", err)
		}
	}
}

func TestClientRejectsInvalidStateAndHelloVersion(t *testing.T) {
	t.Parallel()

	var client *Client
	if _, err := client.Do(context.Background(), "PING"); err == nil {
		t.Fatal("Do(nil client) error = nil, want error")
	}
	client = NewClient(&scriptedConn{})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Do(canceled, "PING"); err == nil {
		t.Fatal("Do(canceled) error = nil, want error")
	}
	if _, err := client.Hello(context.Background(), 4, "", ""); err == nil {
		t.Fatal("Hello(version 4) error = nil, want error")
	}
	if got := RedisError("NOAUTH").Error(); got != "NOAUTH" {
		t.Fatalf("RedisError.Error() = %q", got)
	}
}

func TestDialReturnsConnectError(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Dial(ctx, "127.0.0.1:1"); err == nil {
		t.Fatal("Dial(canceled) error = nil, want error")
	}
}

type scriptedConn struct {
	responses *bytes.Buffer
	writes    bytes.Buffer
}

func (c *scriptedConn) Read(p []byte) (int, error) {
	if c.responses == nil {
		return 0, io.EOF
	}
	return c.responses.Read(p)
}

func (c *scriptedConn) Write(p []byte) (int, error) {
	return c.writes.Write(p)
}
