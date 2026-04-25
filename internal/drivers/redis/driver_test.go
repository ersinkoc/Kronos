package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/drivers"
	"github.com/kronos/kronos/internal/manifest"
)

func TestDriverBackupFullScanDump(t *testing.T) {
	t.Parallel()

	fake := &fakeCommander{
		responses: []Value{
			{Type: TypeArray},
			{Type: TypeArray, Array: []Value{{Type: TypeBulkString, String: "user default on nopass ~* &* +@all"}}},
			{Type: TypeArray, Array: []Value{
				{Type: TypeBulkString, String: "0"},
				{Type: TypeArray, Array: []Value{{Type: TypeBulkString, String: "user:1"}}},
			}},
			{Type: TypeSimpleString, String: "string"},
			{Type: TypeInteger, Int: 1234},
			{Type: TypeBulkString, String: "dump-bytes"},
		},
	}
	driver := &Driver{dial: func(context.Context, drivers.Target) (commander, error) { return fake, nil }}
	var stream drivers.MemoryRecordStream
	rp, err := driver.BackupFull(context.Background(), drivers.Target{Name: "redis"}, &stream)
	if err != nil {
		t.Fatalf("BackupFull() error = %v", err)
	}
	if rp.Driver != "redis" {
		t.Fatalf("ResumePoint = %#v", rp)
	}
	records := stream.Records()
	if len(records) != 4 || records[0].Object.Kind != "acl" || !records[1].Done || records[2].Object.Name != "user:1" || !records[3].Done {
		t.Fatalf("records = %#v", records)
	}
	var acl aclRecord
	if err := json.Unmarshal(records[0].Payload, &acl); err != nil {
		t.Fatalf("Unmarshal(acl) error = %v", err)
	}
	if len(acl.Users) != 1 || acl.Users[0] != "user default on nopass ~* &* +@all" {
		t.Fatalf("acl = %#v", acl)
	}
	var record keyRecord
	if err := json.Unmarshal(records[2].Payload, &record); err != nil {
		t.Fatalf("Unmarshal(record) error = %v", err)
	}
	if record.Key != "user:1" || record.Type != "string" || record.TTLMillis != 1234 || string(record.Dump) != "dump-bytes" {
		t.Fatalf("record = %#v", record)
	}
}

func TestDriverNameVersionTestAndUnsupportedIncremental(t *testing.T) {
	t.Parallel()

	driver := NewDriver()
	if driver.Name() != "redis" {
		t.Fatalf("Name() = %q", driver.Name())
	}
	fake := &fakeCommander{
		responses: []Value{
			{Type: TypeArray},
			{Type: TypeBulkString, String: "# Server\r\nredis_version:7.2.4\r\n"},
			{Type: TypeArray},
			{Type: TypeSimpleString, String: "PONG"},
		},
	}
	driver.dial = func(context.Context, drivers.Target) (commander, error) { return fake, nil }
	version, err := driver.Version(context.Background(), drivers.Target{Name: "redis"})
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	if version != "7.2.4" {
		t.Fatalf("Version() = %q", version)
	}
	if err := driver.Test(context.Background(), drivers.Target{Name: "redis"}); err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if _, err := driver.BackupIncremental(context.Background(), drivers.Target{}, manifest.Manifest{}, nil); !errors.Is(err, drivers.ErrIncrementalUnsupported) {
		t.Fatalf("BackupIncremental() error = %v", err)
	}
}

func TestDriverConnectFallsBackToAuth(t *testing.T) {
	t.Parallel()

	fake := &fakeCommander{helloErr: fmt.Errorf("hello disabled")}
	driver := &Driver{dial: func(context.Context, drivers.Target) (commander, error) { return fake, nil }}
	if _, err := driver.connect(context.Background(), drivers.Target{Connection: map[string]string{"username": "backup", "password": "secret"}}); err != nil {
		t.Fatalf("connect() error = %v", err)
	}
	want := [][]string{{"HELLO"}, {"AUTH", "backup", "secret"}}
	if fmt.Sprint(fake.commands) != fmt.Sprint(want) {
		t.Fatalf("commands = %v, want %v", fake.commands, want)
	}
}

func TestDriverConnectReturnsDialAndAuthErrors(t *testing.T) {
	t.Parallel()

	dialErr := fmt.Errorf("dial failed")
	driver := &Driver{dial: func(context.Context, drivers.Target) (commander, error) { return nil, dialErr }}
	if _, err := driver.connect(context.Background(), drivers.Target{}); !errors.Is(err, dialErr) {
		t.Fatalf("connect(dial) error = %v, want %v", err, dialErr)
	}
	authErr := fmt.Errorf("auth failed")
	driver = &Driver{dial: func(context.Context, drivers.Target) (commander, error) {
		return &fakeCommander{helloErr: fmt.Errorf("hello disabled"), authErr: authErr}, nil
	}}
	if _, err := driver.connect(context.Background(), drivers.Target{}); !errors.Is(err, authErr) {
		t.Fatalf("connect(auth) error = %v, want %v", err, authErr)
	}
}

func TestDialTargetUsesDefaultAddressAndContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dialTarget(ctx, drivers.Target{Connection: map[string]string{}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("dialTarget(canceled default) error = %v, want context.Canceled", err)
	}
	if _, err := dialTarget(ctx, drivers.Target{Connection: map[string]string{"host": "127.0.0.1", "port": "6380"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("dialTarget(canceled host/port) error = %v, want context.Canceled", err)
	}
}

func TestDriverStreamWaitsForContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewDriver().Stream(ctx, drivers.Target{}, drivers.ResumePoint{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Stream(canceled) error = %v", err)
	}
}

func TestDriverRestore(t *testing.T) {
	t.Parallel()

	fake := &fakeCommander{
		responses: []Value{
			{Type: TypeArray},
			{Type: TypeSimpleString, String: "OK"},
		},
	}
	driver := &Driver{dial: func(context.Context, drivers.Target) (commander, error) { return fake, nil }}
	var stream drivers.MemoryRecordStream
	payload, err := json.Marshal(keyRecord{Key: "user:1", Type: "string", TTLMillis: 0, Dump: []byte("dump")})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := stream.WriteRecord(drivers.ObjectRef{Name: "user:1"}, payload); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if err := driver.Restore(context.Background(), drivers.Target{Name: "redis"}, &stream, drivers.RestoreOptions{ReplaceExisting: true}); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	last := fake.commands[len(fake.commands)-1]
	want := []string{"RESTORE", "user:1", "0", "dump", "REPLACE"}
	if len(last) != len(want) {
		t.Fatalf("RESTORE command = %v", last)
	}
	for i := range want {
		if last[i] != want[i] {
			t.Fatalf("RESTORE command = %v, want %v", last, want)
		}
	}
}

func TestDriverRestoreACL(t *testing.T) {
	t.Parallel()

	fake := &fakeCommander{
		responses: []Value{
			{Type: TypeArray},
			{Type: TypeSimpleString, String: "OK"},
		},
	}
	driver := &Driver{dial: func(context.Context, drivers.Target) (commander, error) { return fake, nil }}
	var stream drivers.MemoryRecordStream
	payload, err := json.Marshal(aclRecord{Users: []string{"user backup on >secret ~* +@read"}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := stream.WriteRecord(drivers.ObjectRef{Name: "acl", Kind: "acl"}, payload); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if err := driver.Restore(context.Background(), drivers.Target{Name: "redis"}, &stream, drivers.RestoreOptions{}); err != nil {
		t.Fatalf("Restore(acl) error = %v", err)
	}
	last := fake.commands[len(fake.commands)-1]
	want := []string{"ACL", "SETUSER", "backup", "on", ">secret", "~*", "+@read"}
	if len(last) != len(want) {
		t.Fatalf("ACL SETUSER command = %v", last)
	}
	for i := range want {
		if last[i] != want[i] {
			t.Fatalf("ACL SETUSER command = %v, want %v", last, want)
		}
	}
}

func TestDriverReplayStreamCommands(t *testing.T) {
	t.Parallel()

	fake := &fakeCommander{
		responses: []Value{
			{Type: TypeArray},
			{Type: TypeSimpleString, String: "OK"},
			{Type: TypeSimpleString, String: "OK"},
		},
	}
	driver := &Driver{dial: func(context.Context, drivers.Target) (commander, error) { return fake, nil }}
	reader := &fakeStreamReader{records: []drivers.StreamRecord{
		{ResumePoint: drivers.ResumePoint{Position: "1"}, Payload: []byte(`["SET","user:1","Ada"]`)},
		{ResumePoint: drivers.ResumePoint{Position: "2"}, Payload: []byte(`{"command":["DEL"],"args":["user:2"]}`)},
		{ResumePoint: drivers.ResumePoint{Position: "3"}, Payload: []byte(`["SET","user:3","Grace"]`)},
	}}
	if err := driver.ReplayStream(context.Background(), drivers.Target{Name: "redis"}, reader, drivers.ReplayTarget{Position: "2"}); err != nil {
		t.Fatalf("ReplayStream() error = %v", err)
	}
	wantCommands := [][]string{
		{"HELLO"},
		{"SET", "user:1", "Ada"},
		{"DEL", "user:2"},
	}
	if len(fake.commands) != len(wantCommands) {
		t.Fatalf("commands = %v, want %v", fake.commands, wantCommands)
	}
	for i := range wantCommands {
		if len(fake.commands[i]) != len(wantCommands[i]) {
			t.Fatalf("command %d = %v, want %v", i, fake.commands[i], wantCommands[i])
		}
		for j := range wantCommands[i] {
			if fake.commands[i][j] != wantCommands[i][j] {
				t.Fatalf("command %d = %v, want %v", i, fake.commands[i], wantCommands[i])
			}
		}
	}
}

func TestParseInfoFieldAndIndexByte(t *testing.T) {
	t.Parallel()

	info := "# Server\r\nredis_version:7.2.4\r\nrole:master"
	if got := parseInfoField(info, "redis_version"); got != "7.2.4" {
		t.Fatalf("parseInfoField(redis_version) = %q", got)
	}
	if got := parseInfoField(info, "missing"); got != "" {
		t.Fatalf("parseInfoField(missing) = %q", got)
	}
	if got := indexByte("abc", 'b'); got != 1 {
		t.Fatalf("indexByte() = %d", got)
	}
	if got := indexByte("abc", 'z'); got != -1 {
		t.Fatalf("indexByte(missing) = %d", got)
	}
}

func TestReplayStreamStopsAtTime(t *testing.T) {
	t.Parallel()

	fake := &fakeCommander{responses: []Value{{Type: TypeArray}}}
	driver := &Driver{dial: func(context.Context, drivers.Target) (commander, error) { return fake, nil }}
	cutoff := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	reader := &fakeStreamReader{records: []drivers.StreamRecord{
		{ResumePoint: drivers.ResumePoint{Time: cutoff.Add(time.Second)}, Payload: []byte(`["SET","late","1"]`)},
	}}
	if err := driver.ReplayStream(context.Background(), drivers.Target{Name: "redis"}, reader, drivers.ReplayTarget{Time: cutoff}); err != nil {
		t.Fatalf("ReplayStream(time cutoff) error = %v", err)
	}
	if len(fake.commands) != 1 {
		t.Fatalf("commands = %v, want only HELLO", fake.commands)
	}
}

type fakeCommander struct {
	responses []Value
	commands  [][]string
	helloErr  error
	authErr   error
}

func (c *fakeCommander) Do(ctx context.Context, args ...string) (Value, error) {
	c.commands = append(c.commands, append([]string(nil), args...))
	if len(c.responses) == 0 {
		return Value{Type: TypeSimpleString, String: "OK"}, nil
	}
	value := c.responses[0]
	c.responses = c.responses[1:]
	return value, nil
}

func (c *fakeCommander) Auth(ctx context.Context, username string, password string) error {
	c.commands = append(c.commands, []string{"AUTH", username, password})
	return c.authErr
}

func (c *fakeCommander) Hello(ctx context.Context, version int, username string, password string) (Value, error) {
	c.commands = append(c.commands, []string{"HELLO"})
	if c.helloErr != nil {
		return Value{}, c.helloErr
	}
	if len(c.responses) == 0 {
		return Value{Type: TypeArray}, nil
	}
	value := c.responses[0]
	c.responses = c.responses[1:]
	return value, nil
}

type fakeStreamReader struct {
	records []drivers.StreamRecord
	index   int
}

func (r *fakeStreamReader) NextStream() (drivers.StreamRecord, error) {
	if r.index >= len(r.records) {
		return drivers.StreamRecord{}, io.EOF
	}
	record := r.records[r.index]
	r.index++
	return record, nil
}
