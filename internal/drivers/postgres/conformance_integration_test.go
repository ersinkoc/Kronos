//go:build integration

package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/drivers"
)

func TestPostgresDriverConformanceBackupRestore(t *testing.T) {
	sourceDSN := strings.TrimSpace(os.Getenv("KRONOS_POSTGRES_TEST_DSN"))
	if sourceDSN == "" {
		t.Skip("KRONOS_POSTGRES_TEST_DSN is not set")
	}
	requireCommand(t, "pg_dump")
	requireCommand(t, "pg_dumpall")
	requireCommand(t, "psql")

	timeoutSeconds := envInt("KRONOS_POSTGRES_TEST_TIMEOUT_SECONDS", 30)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	bulkRows := envInt("KRONOS_POSTGRES_BULK_ROWS", 2500)
	bulkChecksum := bulkRows * (bulkRows + 1) / 2
	suffix := randomHex(t, 4)
	sourceSchema := "kronos_src_" + suffix
	restoreSchema := "kronos_restore_" + suffix
	failureSchema := "kronos_fail_" + suffix
	roleName := "kronos_role_" + suffix
	globalRestoreRoleName := "kronos_global_restore_" + suffix
	restoreDSN := firstNonEmpty(strings.TrimSpace(os.Getenv("KRONOS_POSTGRES_RESTORE_DSN")), sourceDSN)
	sourceAdminRole := queryScalar(t, ctx, sourceDSN, "select current_user;")
	cleanupSchema(t, ctx, sourceDSN, sourceSchema)
	cleanupSchema(t, ctx, restoreDSN, restoreSchema)
	cleanupSchema(t, ctx, restoreDSN, failureSchema)
	cleanupRole(t, ctx, sourceDSN, roleName)
	cleanupRole(t, ctx, restoreDSN, globalRestoreRoleName)
	if fullGlobalRestoreEnabled() && restoreDSN != sourceDSN {
		cleanupSchema(t, ctx, restoreDSN, sourceSchema)
		cleanupRole(t, ctx, restoreDSN, roleName)
		if sourceAdminRole != "" {
			cleanupRole(t, ctx, restoreDSN, sourceAdminRole)
		}
	}
	defer cleanupSchema(t, context.Background(), sourceDSN, sourceSchema)
	defer cleanupSchema(t, context.Background(), restoreDSN, restoreSchema)
	defer cleanupSchema(t, context.Background(), restoreDSN, failureSchema)
	defer cleanupRole(t, context.Background(), sourceDSN, roleName)
	defer cleanupRole(t, context.Background(), restoreDSN, globalRestoreRoleName)
	if fullGlobalRestoreEnabled() && restoreDSN != sourceDSN {
		defer cleanupSchema(t, context.Background(), restoreDSN, sourceSchema)
		defer cleanupRole(t, context.Background(), restoreDSN, roleName)
		if sourceAdminRole != "" {
			defer cleanupRole(t, context.Background(), restoreDSN, sourceAdminRole)
		}
	}

	seedSQL := fmt.Sprintf(`
create extension if not exists pgcrypto;
create schema %s;
create table %s.users(id integer primary key, name text not null);
create table %s.documents(id integer primary key, public_id uuid not null default gen_random_uuid(), payload_oid oid not null);
create table %s.bulk_items(id integer primary key, label text not null, payload jsonb not null, created_at timestamptz not null default now());
insert into %s.users(id, name) values (1, 'Ada'), (2, 'Grace');
insert into %s.documents(id, payload_oid) values (1, lo_from_bytea(0, convert_to('kronos-large-object-%s', 'UTF8')));
insert into %s.bulk_items(id, label, payload, created_at)
select g, 'item-' || g, jsonb_build_object('rank', g, 'bucket', g %% 17, 'tag', 'kronos-%s'), '2026-04-27T00:00:00Z'::timestamptz + (g || ' seconds')::interval
from generate_series(1, %d) as g;
create index bulk_items_label_idx on %s.bulk_items(label);
`, sourceSchema, sourceSchema, sourceSchema, sourceSchema, sourceSchema, sourceSchema, suffix, sourceSchema, suffix, bulkRows, sourceSchema)
	runPSQL(t, ctx, sourceDSN, seedSQL)
	runPSQL(t, ctx, sourceDSN, fmt.Sprintf("create role %s login password 'kronos-secret-%s';", roleName, suffix))

	driver := NewDriver()
	var backup drivers.MemoryRecordStream
	rp, err := driver.BackupFull(ctx, drivers.Target{
		Connection: map[string]string{"dsn": sourceDSN},
		Options:    map[string]string{"include_globals": "true"},
	}, &backup)
	if err != nil {
		t.Fatalf("BackupFull() error = %v", err)
	}
	records := backup.Records()
	if rp.Position != "pg_dumpall:globals+pg_dump:plain" {
		t.Fatalf("ResumePoint.Position = %q", rp.Position)
	}
	if len(records) < 3 || records[0].Object.Kind != globalsObjectKind || records[2].Object.Kind != databaseObjectKind {
		t.Fatalf("backup records do not include globals then database streams: %#v", records)
	}
	globalsSQL := string(records[0].Payload)
	if !strings.Contains(globalsSQL, roleName) {
		t.Fatalf("globals backup does not contain role %q", roleName)
	}
	if strings.Contains(strings.ToLower(globalsSQL), "kronos-secret") || strings.Contains(strings.ToLower(globalsSQL), "password '") {
		t.Fatalf("globals backup leaked role password material")
	}
	if !strings.Contains(string(records[2].Payload), sourceSchema) || !strings.Contains(string(records[2].Payload), "Ada") {
		t.Fatalf("backup records do not contain expected source data: %#v", records)
	}
	if fullGlobalRestoreEnabled() && restoreDSN != sourceDSN {
		var fullRestore drivers.MemoryRecordStream
		writeRecords(t, &fullRestore, records)
		if err := driver.Restore(ctx, drivers.Target{Connection: map[string]string{"dsn": restoreDSN}}, &fullRestore, drivers.RestoreOptions{ReplaceExisting: true}); err != nil {
			t.Fatalf("Restore(full cluster globals + database) error = %v", err)
		}
		restoredSourceRole := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select exists(select 1 from pg_roles where rolname = '%s');", roleName))
		if restoredSourceRole != "t" {
			t.Fatalf("full restore role presence = %q, want t", restoredSourceRole)
		}
		if sourceAdminRole != "" {
			restoredAdminRole := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select exists(select 1 from pg_roles where rolname = '%s');", sourceAdminRole))
			if restoredAdminRole != "t" {
				t.Fatalf("full restore source admin role presence = %q, want t", restoredAdminRole)
			}
		}
		fullRestoreCount := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select count(*) from %s.bulk_items;", sourceSchema))
		if fullRestoreCount != strconv.Itoa(bulkRows) {
			t.Fatalf("full restore bulk row count = %q, want %d", fullRestoreCount, bulkRows)
		}
		fullRestoreIndexPresent := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select to_regclass('%s.bulk_items_label_idx') is not null;", sourceSchema))
		if fullRestoreIndexPresent != "t" {
			t.Fatalf("full restore bulk index presence = %q, want t", fullRestoreIndexPresent)
		}
		runPSQL(t, ctx, restoreDSN, fmt.Sprintf("select lo_unlink(payload_oid) from %s.documents; drop schema %s cascade;", sourceSchema, sourceSchema))
	}
	var globalRestore drivers.MemoryRecordStream
	globalRestoreSQL := fmt.Sprintf("create role %s; comment on role %s is 'kronos global restore drill';", globalRestoreRoleName, globalRestoreRoleName)
	if err := globalRestore.WriteRecord(drivers.ObjectRef{Name: "globals", Kind: globalsObjectKind}, []byte(globalRestoreSQL)); err != nil {
		t.Fatalf("WriteRecord(globals restore) error = %v", err)
	}
	if err := driver.Restore(ctx, drivers.Target{Connection: map[string]string{"dsn": restoreDSN}}, &globalRestore, drivers.RestoreOptions{ReplaceExisting: true}); err != nil {
		t.Fatalf("Restore(globals) error = %v", err)
	}
	restoredRole := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select exists(select 1 from pg_roles where rolname = '%s');", globalRestoreRoleName))
	if restoredRole != "t" {
		t.Fatalf("restored global role presence = %q, want t", restoredRole)
	}
	restoredRoleComment := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select coalesce(shobj_description((select oid from pg_roles where rolname = '%s'), 'pg_authid'), '');", globalRestoreRoleName))
	if restoredRoleComment != "kronos global restore drill" {
		t.Fatalf("restored global role comment = %q, want drill comment", restoredRoleComment)
	}
	runPSQL(t, ctx, sourceDSN, fmt.Sprintf("select lo_unlink(payload_oid) from %s.documents; drop schema %s cascade;", sourceSchema, sourceSchema))

	var restore drivers.MemoryRecordStream
	rewrite := strings.ReplaceAll(string(records[2].Payload), sourceSchema, restoreSchema)
	if err := restore.WriteRecord(drivers.ObjectRef{Name: restoreSchema, Kind: databaseObjectKind}, []byte(rewrite)); err != nil {
		t.Fatalf("WriteRecord(restore) error = %v", err)
	}
	if err := driver.Restore(ctx, drivers.Target{Connection: map[string]string{"dsn": restoreDSN}}, &restore, drivers.RestoreOptions{ReplaceExisting: true}); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	count := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select count(*) from %s.users;", restoreSchema))
	if count != "2" {
		t.Fatalf("restored row count = %q, want 2", count)
	}
	name := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select name from %s.users where id = 1;", restoreSchema))
	if name != "Ada" {
		t.Fatalf("restored id=1 name = %q, want Ada", name)
	}
	largeObject := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select convert_from(lo_get(payload_oid), 'UTF8') from %s.documents where id = 1;", restoreSchema))
	if largeObject != "kronos-large-object-"+suffix {
		t.Fatalf("restored large object = %q, want %q", largeObject, "kronos-large-object-"+suffix)
	}
	publicID := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select public_id::text <> '' from %s.documents where id = 1;", restoreSchema))
	if publicID != "t" {
		t.Fatalf("restored extension-backed uuid presence = %q, want t", publicID)
	}
	bulkCount := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select count(*) from %s.bulk_items;", restoreSchema))
	if bulkCount != strconv.Itoa(bulkRows) {
		t.Fatalf("restored bulk row count = %q, want %d", bulkCount, bulkRows)
	}
	restoredBulkChecksum := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select sum(id)::text || ':' || sum((payload->>'rank')::integer)::text from %s.bulk_items;", restoreSchema))
	wantBulkChecksum := fmt.Sprintf("%d:%d", bulkChecksum, bulkChecksum)
	if restoredBulkChecksum != wantBulkChecksum {
		t.Fatalf("restored bulk checksum = %q, want %s", restoredBulkChecksum, wantBulkChecksum)
	}
	bulkIndexPresent := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select to_regclass('%s.bulk_items_label_idx') is not null;", restoreSchema))
	if bulkIndexPresent != "t" {
		t.Fatalf("restored bulk index presence = %q, want t", bulkIndexPresent)
	}

	var failedRestore drivers.MemoryRecordStream
	failingSQL := fmt.Sprintf(`
create schema %s;
create table %s.created_before_error(id integer primary key);
select kronos_missing_restore_function();
`, failureSchema, failureSchema)
	if err := failedRestore.WriteRecord(drivers.ObjectRef{Name: failureSchema, Kind: databaseObjectKind}, []byte(failingSQL)); err != nil {
		t.Fatalf("WriteRecord(failed restore) error = %v", err)
	}
	if err := driver.Restore(ctx, drivers.Target{Connection: map[string]string{"dsn": restoreDSN}}, &failedRestore, drivers.RestoreOptions{ReplaceExisting: true}); err == nil {
		t.Fatal("Restore(failing SQL) error = nil, want error")
	}
	rolledBack := queryScalar(t, ctx, restoreDSN, fmt.Sprintf("select to_regclass('%s.created_before_error') is null;", failureSchema))
	if rolledBack != "t" {
		t.Fatalf("failing restore rollback = %q, want t", rolledBack)
	}
}

func TestPostgresNativeProtocolBackupRestore(t *testing.T) {
	sourceDSN := strings.TrimSpace(os.Getenv("KRONOS_POSTGRES_TEST_DSN"))
	if sourceDSN == "" {
		t.Skip("KRONOS_POSTGRES_TEST_DSN is not set")
	}
	requireCommand(t, "psql")

	timeoutSeconds := envInt("KRONOS_POSTGRES_TEST_TIMEOUT_SECONDS", 30)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	suffix := randomHex(t, 4)
	schema := "kronos_native_" + suffix
	cleanupSchema(t, ctx, sourceDSN, schema)
	defer cleanupSchema(t, context.Background(), sourceDSN, schema)

	seedSQL := fmt.Sprintf(`
create schema %s;
create type %s.user_status as enum ('active', 'blocked');
create domain %s.email_address as text not null check (value like '%%@%%');
create table %s.users(
  id serial primary key,
  status %s.user_status not null default 'active',
  email %s.email_address not null,
  name text not null,
  updated_at timestamptz
);
create function %s.touch_updated_at() returns trigger
language plpgsql
as $$ begin new.updated_at = now(); return new; end $$;
create trigger users_touch_updated_at before update on %s.users
for each row execute function %s.touch_updated_at();
create index users_name_idx on %s.users(name);
create view %s.active_users as select id, name from %s.users where status = 'active';
create materialized view %s.user_counts as select status, count(*)::integer as count from %s.users group by status;
insert into %s.users(email, name) values ('ada@example.com', 'Ada'), ('grace@example.com', 'Grace');
`, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema)
	runPSQL(t, ctx, sourceDSN, seedSQL)

	driver := NewDriver()
	var backup drivers.MemoryRecordStream
	if _, err := driver.BackupFull(ctx, drivers.Target{
		Connection: map[string]string{"dsn": sourceDSN},
		Options:    map[string]string{"protocol": "native"},
	}, &backup); err != nil {
		t.Fatalf("BackupFull(native) error = %v", err)
	}
	records := backup.Records()
	if len(records) != 2 || records[0].Object.Kind != databaseObjectKind || !records[1].Done {
		t.Fatalf("native backup records = %#v", records)
	}
	payload := string(records[0].Payload)
	for _, want := range []string{
		fmt.Sprintf("CREATE TYPE \"%s\".\"user_status\" AS ENUM", schema),
		fmt.Sprintf("CREATE DOMAIN \"%s\".\"email_address\"", schema),
		fmt.Sprintf("CREATE SEQUENCE \"%s\".\"users_id_seq\"", schema),
		fmt.Sprintf("CREATE TABLE \"%s\".\"users\"", schema),
		fmt.Sprintf("CREATE FUNCTION %s.touch_updated_at()", schema),
		"CREATE TRIGGER users_touch_updated_at",
		fmt.Sprintf("CREATE VIEW \"%s\".\"active_users\"", schema),
		fmt.Sprintf("CREATE MATERIALIZED VIEW \"%s\".\"user_counts\"", schema),
	} {
		if !strings.Contains(payload, want) {
			t.Fatalf("native backup payload missing %q:\n%s", want, payload)
		}
	}

	cleanupSchema(t, ctx, sourceDSN, schema)
	var restore drivers.MemoryRecordStream
	writeRecords(t, &restore, records)
	if err := driver.Restore(ctx, drivers.Target{
		Connection: map[string]string{"dsn": sourceDSN},
		Options:    map[string]string{"protocol": "native"},
	}, &restore, drivers.RestoreOptions{ReplaceExisting: true}); err != nil {
		t.Fatalf("Restore(native) error = %v", err)
	}

	count := queryScalar(t, ctx, sourceDSN, fmt.Sprintf("select count(*) from %s.users;", schema))
	if count != "2" {
		t.Fatalf("native restored row count = %q, want 2", count)
	}
	viewCount := queryScalar(t, ctx, sourceDSN, fmt.Sprintf("select count(*) from %s.active_users;", schema))
	if viewCount != "2" {
		t.Fatalf("native restored view count = %q, want 2", viewCount)
	}
	materializedPopulated := queryScalar(t, ctx, sourceDSN, fmt.Sprintf("select ispopulated from pg_matviews where schemaname = '%s' and matviewname = 'user_counts';", schema))
	if materializedPopulated != "f" {
		t.Fatalf("native restored matview populated = %q, want f", materializedPopulated)
	}
	runPSQL(t, ctx, sourceDSN, fmt.Sprintf("update %s.users set name = 'Ada Lovelace' where id = 1;", schema))
	triggerSetTimestamp := queryScalar(t, ctx, sourceDSN, fmt.Sprintf("select updated_at is not null from %s.users where id = 1;", schema))
	if triggerSetTimestamp != "t" {
		t.Fatalf("native restored trigger updated_at = %q, want t", triggerSetTimestamp)
	}
	runPSQL(t, ctx, sourceDSN, fmt.Sprintf("insert into %s.users(email, name) values ('katherine@example.com', 'Katherine');", schema))
	nextID := queryScalar(t, ctx, sourceDSN, fmt.Sprintf("select max(id)::text from %s.users;", schema))
	if nextID != "3" {
		t.Fatalf("native restored sequence max id = %q, want 3", nextID)
	}
}

func requireCommand(t *testing.T, name string) {
	t.Helper()

	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is not installed: %v", name, err)
	}
}

func randomHex(t *testing.T, n int) string {
	t.Helper()

	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return hex.EncodeToString(buf)
}

func envInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func cleanupSchema(t *testing.T, ctx context.Context, dsn string, schema string) {
	t.Helper()

	cmd := exec.CommandContext(ctx, "psql", "--set", "ON_ERROR_STOP=1", "--dbname", dsn, "--command", fmt.Sprintf("drop schema if exists %s cascade;", schema))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup schema %s error = %v output=%s", schema, err, strings.TrimSpace(string(out)))
	}
}

func cleanupRole(t *testing.T, ctx context.Context, dsn string, roleName string) {
	t.Helper()

	cmd := exec.CommandContext(ctx, "psql", "--set", "ON_ERROR_STOP=1", "--dbname", dsn, "--command", fmt.Sprintf("drop role if exists %s;", roleName))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup role %s error = %v output=%s", roleName, err, strings.TrimSpace(string(out)))
	}
}

func runPSQL(t *testing.T, ctx context.Context, dsn string, sql string) {
	t.Helper()

	cmd := exec.CommandContext(ctx, "psql", "--set", "ON_ERROR_STOP=1", "--dbname", dsn)
	cmd.Stdin = strings.NewReader(sql)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("psql error = %v output=%s", err, strings.TrimSpace(string(out)))
	}
}

func queryScalar(t *testing.T, ctx context.Context, dsn string, sql string) string {
	t.Helper()

	cmd := exec.CommandContext(ctx, "psql", "--no-align", "--tuples-only", "--set", "ON_ERROR_STOP=1", "--dbname", dsn, "--command", sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("query %q error = %v output=%s", sql, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out))
}

func fullGlobalRestoreEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KRONOS_POSTGRES_FULL_GLOBAL_RESTORE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeRecords(t *testing.T, stream *drivers.MemoryRecordStream, records []drivers.Record) {
	t.Helper()

	for _, record := range records {
		if record.Done {
			if err := stream.FinishObject(record.Object, record.Rows); err != nil {
				t.Fatalf("FinishObject(%s) error = %v", record.Object.Name, err)
			}
			continue
		}
		if err := stream.WriteRecord(record.Object, record.Payload); err != nil {
			t.Fatalf("WriteRecord(%s) error = %v", record.Object.Name, err)
		}
	}
}
