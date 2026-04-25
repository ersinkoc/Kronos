package server

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
	sched "github.com/kronos/kronos/internal/schedule"
)

var (
	targetsBucket   = []byte("targets")
	storagesBucket  = []byte("storages")
	schedulesBucket = []byte("schedules")
	backupsBucket   = []byte("backups")
	usersBucket     = []byte("users")
	policiesBucket  = []byte("retention_policies")
)

// TargetStore persists target definitions.
type TargetStore struct {
	db *kvstore.DB
}

// StorageStore persists storage backend definitions.
type StorageStore struct {
	db *kvstore.DB
}

// ScheduleStore persists schedule definitions.
type ScheduleStore struct {
	db *kvstore.DB
}

// BackupStore persists committed backup metadata.
type BackupStore struct {
	db *kvstore.DB
}

// UserStore persists local and federated user metadata.
type UserStore struct {
	db *kvstore.DB
}

// RetentionPolicyStore persists reusable retention policies.
type RetentionPolicyStore struct {
	db *kvstore.DB
}

// NewTargetStore returns a target store backed by db.
func NewTargetStore(db *kvstore.DB) (*TargetStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &TargetStore{db: db}, nil
}

// NewStorageStore returns a storage store backed by db.
func NewStorageStore(db *kvstore.DB) (*StorageStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &StorageStore{db: db}, nil
}

// NewScheduleStore returns a schedule store backed by db.
func NewScheduleStore(db *kvstore.DB) (*ScheduleStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &ScheduleStore{db: db}, nil
}

// NewBackupStore returns a backup store backed by db.
func NewBackupStore(db *kvstore.DB) (*BackupStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &BackupStore{db: db}, nil
}

// NewUserStore returns a user store backed by db.
func NewUserStore(db *kvstore.DB) (*UserStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &UserStore{db: db}, nil
}

// NewRetentionPolicyStore returns a retention policy store backed by db.
func NewRetentionPolicyStore(db *kvstore.DB) (*RetentionPolicyStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &RetentionPolicyStore{db: db}, nil
}

// Save inserts or replaces a target.
func (s *TargetStore) Save(target core.Target) error {
	if err := validateTarget(target); err != nil {
		return err
	}
	return saveJSON(s.db, targetsBucket, []byte(target.ID), target)
}

// Get fetches one target by ID.
func (s *TargetStore) Get(id core.ID) (core.Target, bool, error) {
	var target core.Target
	ok, err := getJSON(s.db, targetsBucket, []byte(id), &target)
	return target, ok, err
}

// List returns all targets ordered by name, then ID.
func (s *TargetStore) List() ([]core.Target, error) {
	var targets []core.Target
	if err := listJSON(s.db, targetsBucket, func(data []byte) error {
		var target core.Target
		if err := json.Unmarshal(data, &target); err != nil {
			return err
		}
		targets = append(targets, target)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Name == targets[j].Name {
			return targets[i].ID < targets[j].ID
		}
		return targets[i].Name < targets[j].Name
	})
	return targets, nil
}

// Delete removes a target by ID.
func (s *TargetStore) Delete(id core.ID) error {
	return deleteKey(s.db, targetsBucket, []byte(id))
}

// Save inserts or replaces a storage definition.
func (s *StorageStore) Save(storage core.Storage) error {
	if err := validateStorage(storage); err != nil {
		return err
	}
	return saveJSON(s.db, storagesBucket, []byte(storage.ID), storage)
}

// Get fetches one storage definition by ID.
func (s *StorageStore) Get(id core.ID) (core.Storage, bool, error) {
	var storage core.Storage
	ok, err := getJSON(s.db, storagesBucket, []byte(id), &storage)
	return storage, ok, err
}

// List returns all storage definitions ordered by name, then ID.
func (s *StorageStore) List() ([]core.Storage, error) {
	var storages []core.Storage
	if err := listJSON(s.db, storagesBucket, func(data []byte) error {
		var storage core.Storage
		if err := json.Unmarshal(data, &storage); err != nil {
			return err
		}
		storages = append(storages, storage)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(storages, func(i, j int) bool {
		if storages[i].Name == storages[j].Name {
			return storages[i].ID < storages[j].ID
		}
		return storages[i].Name < storages[j].Name
	})
	return storages, nil
}

// Delete removes a storage definition by ID.
func (s *StorageStore) Delete(id core.ID) error {
	return deleteKey(s.db, storagesBucket, []byte(id))
}

// Save inserts or replaces a schedule.
func (s *ScheduleStore) Save(schedule core.Schedule) error {
	if err := validateSchedule(schedule); err != nil {
		return err
	}
	return saveJSON(s.db, schedulesBucket, []byte(schedule.ID), schedule)
}

// Get fetches one schedule by ID.
func (s *ScheduleStore) Get(id core.ID) (core.Schedule, bool, error) {
	var schedule core.Schedule
	ok, err := getJSON(s.db, schedulesBucket, []byte(id), &schedule)
	return schedule, ok, err
}

// List returns all schedules ordered by name, then ID.
func (s *ScheduleStore) List() ([]core.Schedule, error) {
	var schedules []core.Schedule
	if err := listJSON(s.db, schedulesBucket, func(data []byte) error {
		var schedule core.Schedule
		if err := json.Unmarshal(data, &schedule); err != nil {
			return err
		}
		schedules = append(schedules, schedule)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(schedules, func(i, j int) bool {
		if schedules[i].Name == schedules[j].Name {
			return schedules[i].ID < schedules[j].ID
		}
		return schedules[i].Name < schedules[j].Name
	})
	return schedules, nil
}

// Delete removes a schedule by ID.
func (s *ScheduleStore) Delete(id core.ID) error {
	return deleteKey(s.db, schedulesBucket, []byte(id))
}

// Save inserts or replaces committed backup metadata.
func (s *BackupStore) Save(backup core.Backup) error {
	if err := validateBackup(backup); err != nil {
		return err
	}
	return saveJSON(s.db, backupsBucket, []byte(backup.ID), backup)
}

// Get fetches one backup by ID.
func (s *BackupStore) Get(id core.ID) (core.Backup, bool, error) {
	var backup core.Backup
	ok, err := getJSON(s.db, backupsBucket, []byte(id), &backup)
	return backup, ok, err
}

// List returns all backups ordered newest first, then ID.
func (s *BackupStore) List() ([]core.Backup, error) {
	var backups []core.Backup
	if err := listJSON(s.db, backupsBucket, func(data []byte) error {
		var backup core.Backup
		if err := json.Unmarshal(data, &backup); err != nil {
			return err
		}
		backups = append(backups, backup)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].EndedAt.Equal(backups[j].EndedAt) {
			return backups[i].ID < backups[j].ID
		}
		return backups[i].EndedAt.After(backups[j].EndedAt)
	})
	return backups, nil
}

// Protect toggles a backup's manual protection flag.
func (s *BackupStore) Protect(id core.ID, protected bool) (core.Backup, error) {
	backup, ok, err := s.Get(id)
	if err != nil {
		return core.Backup{}, err
	}
	if !ok {
		return core.Backup{}, core.WrapKind(core.ErrorKindNotFound, "protect backup", fmt.Errorf("backup %q not found", id))
	}
	backup.Protected = protected
	if err := s.Save(backup); err != nil {
		return core.Backup{}, err
	}
	return backup, nil
}

// Delete removes a backup metadata record by ID.
func (s *BackupStore) Delete(id core.ID) error {
	return deleteKey(s.db, backupsBucket, []byte(id))
}

// Save inserts or replaces a user.
func (s *UserStore) Save(user core.User) error {
	if err := validateUser(user); err != nil {
		return err
	}
	return saveJSON(s.db, usersBucket, []byte(user.ID), user)
}

// Get fetches one user by ID.
func (s *UserStore) Get(id core.ID) (core.User, bool, error) {
	var user core.User
	ok, err := getJSON(s.db, usersBucket, []byte(id), &user)
	return user, ok, err
}

// List returns all users ordered by email, then ID.
func (s *UserStore) List() ([]core.User, error) {
	var users []core.User
	if err := listJSON(s.db, usersBucket, func(data []byte) error {
		var user core.User
		if err := json.Unmarshal(data, &user); err != nil {
			return err
		}
		users = append(users, user)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(users, func(i, j int) bool {
		if users[i].Email == users[j].Email {
			return users[i].ID < users[j].ID
		}
		return users[i].Email < users[j].Email
	})
	return users, nil
}

// Grant changes a user's built-in role.
func (s *UserStore) Grant(id core.ID, role core.RoleName, now time.Time) (core.User, error) {
	if !validRole(role) {
		return core.User{}, fmt.Errorf("invalid role %q", role)
	}
	user, ok, err := s.Get(id)
	if err != nil {
		return core.User{}, err
	}
	if !ok {
		return core.User{}, core.WrapKind(core.ErrorKindNotFound, "grant user role", fmt.Errorf("user %q not found", id))
	}
	user.Role = role
	if now.IsZero() {
		now = time.Now().UTC()
	}
	user.UpdatedAt = now.UTC()
	if err := s.Save(user); err != nil {
		return core.User{}, err
	}
	return user, nil
}

// Delete removes a user by ID.
func (s *UserStore) Delete(id core.ID) error {
	return deleteKey(s.db, usersBucket, []byte(id))
}

// Save inserts or replaces a retention policy.
func (s *RetentionPolicyStore) Save(policy core.RetentionPolicy) error {
	if err := validateRetentionPolicy(policy); err != nil {
		return err
	}
	return saveJSON(s.db, policiesBucket, []byte(policy.ID), policy)
}

// Get fetches one retention policy by ID.
func (s *RetentionPolicyStore) Get(id core.ID) (core.RetentionPolicy, bool, error) {
	var policy core.RetentionPolicy
	ok, err := getJSON(s.db, policiesBucket, []byte(id), &policy)
	return policy, ok, err
}

// List returns all retention policies ordered by name, then ID.
func (s *RetentionPolicyStore) List() ([]core.RetentionPolicy, error) {
	var policies []core.RetentionPolicy
	if err := listJSON(s.db, policiesBucket, func(data []byte) error {
		var policy core.RetentionPolicy
		if err := json.Unmarshal(data, &policy); err != nil {
			return err
		}
		policies = append(policies, policy)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(policies, func(i, j int) bool {
		if policies[i].Name == policies[j].Name {
			return policies[i].ID < policies[j].ID
		}
		return policies[i].Name < policies[j].Name
	})
	return policies, nil
}

// Delete removes a retention policy by ID.
func (s *RetentionPolicyStore) Delete(id core.ID) error {
	return deleteKey(s.db, policiesBucket, []byte(id))
}

func validateTarget(target core.Target) error {
	if target.ID.IsZero() {
		return fmt.Errorf("target id is required")
	}
	if target.Name == "" {
		return fmt.Errorf("target name is required")
	}
	if target.Driver == "" {
		return fmt.Errorf("target driver is required")
	}
	if target.Endpoint == "" {
		return fmt.Errorf("target endpoint is required")
	}
	return nil
}

func validateStorage(storage core.Storage) error {
	if storage.ID.IsZero() {
		return fmt.Errorf("storage id is required")
	}
	if storage.Name == "" {
		return fmt.Errorf("storage name is required")
	}
	if storage.Kind == "" {
		return fmt.Errorf("storage kind is required")
	}
	if storage.URI == "" {
		return fmt.Errorf("storage uri is required")
	}
	return nil
}

func validateSchedule(schedule core.Schedule) error {
	if schedule.ID.IsZero() {
		return fmt.Errorf("schedule id is required")
	}
	if schedule.Name == "" {
		return fmt.Errorf("schedule name is required")
	}
	if schedule.TargetID.IsZero() {
		return fmt.Errorf("schedule target id is required")
	}
	if schedule.StorageID.IsZero() {
		return fmt.Errorf("schedule storage id is required")
	}
	if schedule.BackupType == "" {
		return fmt.Errorf("schedule backup type is required")
	}
	if schedule.Expression == "" {
		return fmt.Errorf("schedule expression is required")
	}
	if _, err := sched.ParseCron(schedule.Expression); err != nil {
		if _, windowErr := sched.ParseWindow(schedule.Expression); windowErr != nil {
			return fmt.Errorf("schedule expression: %w", err)
		}
	}
	return nil
}

func validateBackup(backup core.Backup) error {
	if backup.ID.IsZero() {
		return fmt.Errorf("backup id is required")
	}
	if backup.TargetID.IsZero() {
		return fmt.Errorf("backup target id is required")
	}
	if backup.StorageID.IsZero() {
		return fmt.Errorf("backup storage id is required")
	}
	if backup.JobID.IsZero() {
		return fmt.Errorf("backup job id is required")
	}
	if backup.Type == "" {
		return fmt.Errorf("backup type is required")
	}
	if backup.ManifestID.IsZero() {
		return fmt.Errorf("backup manifest id is required")
	}
	if backup.EndedAt.IsZero() {
		return fmt.Errorf("backup ended_at is required")
	}
	return nil
}

func validateUser(user core.User) error {
	if user.ID.IsZero() {
		return fmt.Errorf("user id is required")
	}
	if user.Email == "" {
		return fmt.Errorf("user email is required")
	}
	if user.DisplayName == "" {
		return fmt.Errorf("user display name is required")
	}
	if !validRole(user.Role) {
		return fmt.Errorf("invalid user role %q", user.Role)
	}
	return nil
}

func validateRetentionPolicy(policy core.RetentionPolicy) error {
	if policy.ID.IsZero() {
		return fmt.Errorf("retention policy id is required")
	}
	if policy.Name == "" {
		return fmt.Errorf("retention policy name is required")
	}
	if len(policy.Rules) == 0 {
		return fmt.Errorf("retention policy rules are required")
	}
	for i, rule := range policy.Rules {
		if rule.Kind == "" {
			return fmt.Errorf("retention policy rule %d kind is required", i)
		}
	}
	return nil
}

func validRole(role core.RoleName) bool {
	switch role {
	case core.RoleAdmin, core.RoleOperator, core.RoleViewer:
		return true
	default:
		return false
	}
}

func saveJSON(db *kvstore.DB, bucketName []byte, key []byte, value any) error {
	if db == nil {
		return fmt.Errorf("kv database is required")
	}
	if len(key) == 0 {
		return fmt.Errorf("key is required")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return db.Update(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(bucketName)
		if err != nil {
			return err
		}
		return bucket.Put(key, data)
	})
}

func getJSON(db *kvstore.DB, bucketName []byte, key []byte, value any) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("kv database is required")
	}
	if len(key) == 0 {
		return false, fmt.Errorf("key is required")
	}
	var ok bool
	err := db.View(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(bucketName)
		if err != nil {
			return err
		}
		data, exists, err := bucket.Get(key)
		if err != nil || !exists {
			ok = exists
			return err
		}
		ok = true
		return json.Unmarshal(data, value)
	})
	return ok, err
}

func listJSON(db *kvstore.DB, bucketName []byte, fn func([]byte) error) error {
	if db == nil {
		return fmt.Errorf("kv database is required")
	}
	return db.View(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(bucketName)
		if err != nil {
			return err
		}
		it, err := bucket.Scan([]byte{1}, nil)
		if err != nil {
			return err
		}
		for it.Valid() {
			if err := fn(it.Value()); err != nil {
				return err
			}
			it.Next()
		}
		return it.Err()
	})
}

func deleteKey(db *kvstore.DB, bucketName []byte, key []byte) error {
	if db == nil {
		return fmt.Errorf("kv database is required")
	}
	if len(key) == 0 {
		return fmt.Errorf("key is required")
	}
	return db.Update(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(bucketName)
		if err != nil {
			return err
		}
		return bucket.Delete(key)
	})
}

func stampCreatedUpdated(createdAt time.Time, now time.Time) (time.Time, time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if createdAt.IsZero() {
		createdAt = now
	}
	return createdAt.UTC(), now.UTC()
}
