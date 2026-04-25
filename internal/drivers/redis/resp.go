package redis

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
)

// RESPType identifies a Redis serialization protocol value type.
type RESPType byte

const (
	TypeSimpleString RESPType = '+'
	TypeError        RESPType = '-'
	TypeInteger      RESPType = ':'
	TypeBulkString   RESPType = '$'
	TypeArray        RESPType = '*'
	TypeNull         RESPType = '_'
)

// Value is one RESP2/RESP3 value.
type Value struct {
	Type   RESPType
	String string
	Int    int64
	Array  []Value
	Null   bool
}

// EncodeCommand returns a RESP array of bulk string command arguments.
func EncodeCommand(args ...string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&buf, "$%d\r\n%s\r\n", len(arg), arg)
	}
	return buf.Bytes()
}

// Reader parses RESP values from a stream.
type Reader struct {
	r *bufio.Reader
}

// NewReader returns a RESP reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{r: bufio.NewReader(r)}
}

// ReadValue reads one RESP value.
func (r *Reader) ReadValue() (Value, error) {
	prefix, err := r.r.ReadByte()
	if err != nil {
		return Value{}, err
	}
	switch RESPType(prefix) {
	case TypeSimpleString:
		line, err := r.readLine()
		return Value{Type: TypeSimpleString, String: string(line)}, err
	case TypeError:
		line, err := r.readLine()
		return Value{Type: TypeError, String: string(line)}, err
	case TypeInteger:
		line, err := r.readLine()
		if err != nil {
			return Value{}, err
		}
		value, err := strconv.ParseInt(string(line), 10, 64)
		if err != nil {
			return Value{}, err
		}
		return Value{Type: TypeInteger, Int: value}, nil
	case TypeBulkString:
		return r.readBulk()
	case TypeArray:
		return r.readArray()
	case TypeNull:
		if _, err := r.readLine(); err != nil {
			return Value{}, err
		}
		return Value{Type: TypeNull, Null: true}, nil
	default:
		return Value{}, fmt.Errorf("unsupported RESP type %q", prefix)
	}
}

func (r *Reader) readBulk() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	size, err := strconv.Atoi(string(line))
	if err != nil {
		return Value{}, err
	}
	if size == -1 {
		return Value{Type: TypeBulkString, Null: true}, nil
	}
	if size < -1 {
		return Value{}, fmt.Errorf("invalid bulk string size %d", size)
	}
	data := make([]byte, size+2)
	if _, err := io.ReadFull(r.r, data); err != nil {
		return Value{}, err
	}
	if data[size] != '\r' || data[size+1] != '\n' {
		return Value{}, fmt.Errorf("bulk string missing CRLF terminator")
	}
	return Value{Type: TypeBulkString, String: string(data[:size])}, nil
}

func (r *Reader) readArray() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	count, err := strconv.Atoi(string(line))
	if err != nil {
		return Value{}, err
	}
	if count == -1 {
		return Value{Type: TypeArray, Null: true}, nil
	}
	if count < -1 {
		return Value{}, fmt.Errorf("invalid array size %d", count)
	}
	values := make([]Value, 0, count)
	for i := 0; i < count; i++ {
		value, err := r.ReadValue()
		if err != nil {
			return Value{}, err
		}
		values = append(values, value)
	}
	return Value{Type: TypeArray, Array: values}, nil
}

func (r *Reader) readLine() ([]byte, error) {
	line, err := r.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, fmt.Errorf("RESP line missing CRLF terminator")
	}
	return line[:len(line)-2], nil
}
