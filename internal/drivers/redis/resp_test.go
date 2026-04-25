package redis

import (
	"bytes"
	"reflect"
	"testing"
)

func TestEncodeCommand(t *testing.T) {
	t.Parallel()

	got := EncodeCommand("HELLO", "3")
	want := "*2\r\n$5\r\nHELLO\r\n$1\r\n3\r\n"
	if string(got) != want {
		t.Fatalf("EncodeCommand() = %q, want %q", got, want)
	}
}

func TestReadRESPValues(t *testing.T) {
	t.Parallel()

	input := "+OK\r\n:42\r\n$5\r\nhello\r\n*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n_\r\n"
	reader := NewReader(bytes.NewBufferString(input))
	cases := []Value{
		{Type: TypeSimpleString, String: "OK"},
		{Type: TypeInteger, Int: 42},
		{Type: TypeBulkString, String: "hello"},
		{Type: TypeArray, Array: []Value{
			{Type: TypeBulkString, String: "GET"},
			{Type: TypeBulkString, String: "key"},
		}},
		{Type: TypeNull, Null: true},
	}
	for i, want := range cases {
		got, err := reader.ReadValue()
		if err != nil {
			t.Fatalf("ReadValue(%d) error = %v", i, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("ReadValue(%d) = %#v, want %#v", i, got, want)
		}
	}
}

func TestReadRESPRejectsBadBulkTerminator(t *testing.T) {
	t.Parallel()

	reader := NewReader(bytes.NewBufferString("$3\r\nabcxx"))
	if _, err := reader.ReadValue(); err == nil {
		t.Fatal("ReadValue() error = nil, want terminator error")
	}
}
