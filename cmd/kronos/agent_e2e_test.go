//go:build e2e

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
	control "github.com/kronos/kronos/internal/server"
)

func TestE2EWorkerBacksUpRedisThroughControlPlane(t *testing.T) {
	redisEndpoint := startE2ERedisServer(t)
	repoDir := t.TempDir()
	db, err := kvstore.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	stores, err := newAPIStores(db)
	if err != nil {
		t.Fatalf("newAPIStores() error = %v", err)
	}
	if err := stores.targets.Save(core.Target{
		ID:        "target-redis",
		Name:      "redis-e2e",
		Driver:    core.TargetDriverRedis,
		Endpoint:  redisEndpoint,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Save(target) error = %v", err)
	}
	if err := stores.storages.Save(core.Storage{
		ID:        "storage-local",
		Name:      "local-e2e",
		Kind:      core.StorageKindLocal,
		URI:       "file://" + repoDir,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Save(storage) error = %v", err)
	}

	server := httptest.NewServer(newServerHandlerWithStores(nil, nil, stores))
	defer server.Close()

	job := enqueueE2EBackup(t, server, "target-redis", "storage-local")
	_, privateKey, err := ed25519.GenerateKey(bytes.NewReader(bytes.Repeat([]byte{3}, 64)))
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runAgentWorkerWithToken(ctx, server.Client(), server.URL, control.AgentHeartbeat{ID: "agent-e2e", Capacity: 1}, 5*time.Millisecond, "", agentWorkerOptions{
			ManifestPrivateKeyHex: hex.EncodeToString(privateKey),
			ChunkKeyHex:           hex.EncodeToString(bytes.Repeat([]byte{8}, 32)),
			ChunkAlgorithm:        "aes-256-gcm",
			Compression:           "none",
			KeyID:                 "e2e-key",
		})
	}()

	backup := waitForE2EBackup(t, stores.backups, stores.jobs, done, job.ID)
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("worker error = %v", err)
	}
	finished, ok, err := stores.jobs.Get(job.ID)
	if err != nil {
		t.Fatalf("Get(job) error = %v", err)
	}
	if !ok || finished.Status != core.JobStatusSucceeded {
		t.Fatalf("finished job ok=%v job=%#v", ok, finished)
	}
	if backup.TargetID != "target-redis" || backup.StorageID != "storage-local" || backup.ManifestID.IsZero() || backup.ChunkCount == 0 {
		t.Fatalf("backup = %#v", backup)
	}
}

func enqueueE2EBackup(t *testing.T, server *httptest.Server, targetID, storageID core.ID) core.Job {
	t.Helper()

	payload := fmt.Sprintf(`{"target_id":%q,"storage_id":%q,"type":"full"}`, targetID, storageID)
	resp, err := server.Client().Post(server.URL+"/api/v1/backups/now", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST /api/v1/backups/now error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /api/v1/backups/now status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var job core.Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatalf("Decode(job) error = %v", err)
	}
	if job.ID.IsZero() {
		t.Fatal("queued job id is empty")
	}
	return job
}

func waitForE2EBackup(t *testing.T, backups *control.BackupStore, jobs *control.JobStore, done <-chan error, jobID core.ID) core.Backup {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		items, err := backups.List()
		if err != nil {
			t.Fatalf("List(backups) error = %v", err)
		}
		for _, backup := range items {
			if backup.JobID == jobID {
				return backup
			}
		}
		select {
		case err := <-done:
			job, ok, getErr := jobs.Get(jobID)
			if getErr != nil {
				t.Fatalf("worker exited with %v; Get(job) error = %v", err, getErr)
			}
			t.Fatalf("worker exited before backup was saved: err=%v job_ok=%v job=%#v", err, ok, job)
		case <-time.After(10 * time.Millisecond):
		}
	}
	job, ok, err := jobs.Get(jobID)
	if err != nil {
		t.Fatalf("timed out waiting for backup for job %s; Get(job) error = %v", jobID, err)
	}
	t.Fatalf("timed out waiting for backup for job %s; job_ok=%v job=%#v", jobID, ok, job)
	return core.Backup{}
}

func startE2ERedisServer(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go handleE2ERedisConn(t, conn)
		}
	}()
	return listener.Addr().String()
}

func handleE2ERedisConn(t *testing.T, conn net.Conn) {
	t.Helper()
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for {
		command, err := readE2ERedisCommand(reader)
		if err != nil {
			if err != io.EOF {
				t.Errorf("readE2ERedisCommand() error = %v", err)
			}
			return
		}
		if len(command) == 0 {
			continue
		}
		switch strings.ToUpper(command[0]) {
		case "HELLO":
			writeE2ERedisRaw(t, conn, "*2\r\n$6\r\nserver\r\n$5\r\nredis\r\n")
		case "ACL":
			writeE2ERedisArray(t, conn, "user default on nopass ~* &* +@all")
		case "SCAN":
			writeE2ERedisRaw(t, conn, "*2\r\n$1\r\n0\r\n*1\r\n$6\r\nuser:1\r\n")
		case "TYPE":
			writeE2ERedisRaw(t, conn, "+string\r\n")
		case "PTTL":
			writeE2ERedisRaw(t, conn, ":0\r\n")
		case "DUMP":
			writeE2ERedisBulk(t, conn, "dump-user-1")
		case "INFO":
			writeE2ERedisBulk(t, conn, "# Server\r\nredis_version:7.2.0\r\n")
		default:
			writeE2ERedisRaw(t, conn, fmt.Sprintf("-ERR unsupported command %s\r\n", command[0]))
			return
		}
	}
}

func readE2ERedisCommand(reader *bufio.Reader) ([]string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\r\n")
	if !strings.HasPrefix(line, "*") {
		return nil, fmt.Errorf("RESP array expected, got %q", line)
	}
	count, err := strconv.Atoi(strings.TrimPrefix(line, "*"))
	if err != nil {
		return nil, err
	}
	command := make([]string, 0, count)
	for i := 0; i < count; i++ {
		header, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		header = strings.TrimSuffix(header, "\r\n")
		if !strings.HasPrefix(header, "$") {
			return nil, fmt.Errorf("RESP bulk expected, got %q", header)
		}
		size, err := strconv.Atoi(strings.TrimPrefix(header, "$"))
		if err != nil {
			return nil, err
		}
		data := make([]byte, size+2)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		if string(data[size:]) != "\r\n" {
			return nil, fmt.Errorf("RESP bulk missing CRLF terminator")
		}
		command = append(command, string(data[:size]))
	}
	return command, nil
}

func writeE2ERedisBulk(t *testing.T, conn net.Conn, value string) {
	t.Helper()
	writeE2ERedisRaw(t, conn, fmt.Sprintf("$%d\r\n%s\r\n", len(value), value))
}

func writeE2ERedisArray(t *testing.T, conn net.Conn, values ...string) {
	t.Helper()
	writeE2ERedisRaw(t, conn, fmt.Sprintf("*%d\r\n", len(values)))
	for _, value := range values {
		writeE2ERedisBulk(t, conn, value)
	}
}

func writeE2ERedisRaw(t *testing.T, conn net.Conn, value string) {
	t.Helper()
	if _, err := fmt.Fprint(conn, value); err != nil {
		t.Errorf("Write(%q) error = %v", value, err)
	}
}
