package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
)

var tokensBucket = []byte("tokens")

// TokenStore persists scoped API token metadata and verifier hashes.
type TokenStore struct {
	db    *kvstore.DB
	clock core.Clock
}

type apiTokenRecord struct {
	Token      core.Token `json:"token"`
	SecretHash string     `json:"secret_hash"`
}

// CreatedToken contains public token metadata plus the copy-once bearer secret.
type CreatedToken struct {
	Token  core.Token `json:"token"`
	Secret string     `json:"secret"`
}

// NewTokenStore returns an API token store backed by db.
func NewTokenStore(db *kvstore.DB, clock core.Clock) (*TokenStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	if clock == nil {
		clock = core.RealClock{}
	}
	return &TokenStore{db: db, clock: clock}, nil
}

// Create generates, hashes, and stores a new API token.
func (s *TokenStore) Create(name string, userID core.ID, scopes []string, expiresAt time.Time) (CreatedToken, error) {
	if s == nil || s.db == nil {
		return CreatedToken{}, fmt.Errorf("token store is closed")
	}
	if name == "" {
		return CreatedToken{}, fmt.Errorf("token name is required")
	}
	if userID.IsZero() {
		return CreatedToken{}, fmt.Errorf("token user id is required")
	}
	scopes = normalizeScopes(scopes)
	if len(scopes) == 0 {
		return CreatedToken{}, fmt.Errorf("at least one token scope is required")
	}
	now := s.clock.Now().UTC()
	if !expiresAt.IsZero() {
		expiresAt = expiresAt.UTC()
		if !expiresAt.After(now) {
			return CreatedToken{}, fmt.Errorf("token expires_at must be in the future")
		}
	}
	id, err := core.NewID(s.clock)
	if err != nil {
		return CreatedToken{}, err
	}
	secret, err := generateTokenSecret(id)
	if err != nil {
		return CreatedToken{}, err
	}
	token := core.Token{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	record := apiTokenRecord{Token: token, SecretHash: hashTokenSecret(secret)}
	if err := saveJSON(s.db, tokensBucket, []byte(id), record); err != nil {
		return CreatedToken{}, err
	}
	return CreatedToken{Token: token, Secret: secret}, nil
}

// List returns token metadata ordered by creation time descending.
func (s *TokenStore) List() ([]core.Token, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("token store is closed")
	}
	var tokens []core.Token
	if err := listJSON(s.db, tokensBucket, func(data []byte) error {
		var record apiTokenRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return err
		}
		tokens = append(tokens, record.Token)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].CreatedAt.Equal(tokens[j].CreatedAt) {
			return tokens[i].ID < tokens[j].ID
		}
		return tokens[i].CreatedAt.After(tokens[j].CreatedAt)
	})
	return tokens, nil
}

// Get fetches one token by ID.
func (s *TokenStore) Get(id core.ID) (core.Token, bool, error) {
	if s == nil || s.db == nil {
		return core.Token{}, false, fmt.Errorf("token store is closed")
	}
	var record apiTokenRecord
	ok, err := getJSON(s.db, tokensBucket, []byte(id), &record)
	return record.Token, ok, err
}

// Revoke marks a token as revoked.
func (s *TokenStore) Revoke(id core.ID) (core.Token, error) {
	if s == nil || s.db == nil {
		return core.Token{}, fmt.Errorf("token store is closed")
	}
	var record apiTokenRecord
	ok, err := getJSON(s.db, tokensBucket, []byte(id), &record)
	if err != nil {
		return core.Token{}, err
	}
	if !ok {
		return core.Token{}, core.WrapKind(core.ErrorKindNotFound, "revoke token", fmt.Errorf("token %q not found", id))
	}
	if record.Token.RevokedAt.IsZero() {
		record.Token.RevokedAt = s.clock.Now().UTC()
		if err := saveJSON(s.db, tokensBucket, []byte(id), record); err != nil {
			return core.Token{}, err
		}
	}
	return record.Token, nil
}

// Inactive returns token records that are revoked or past expiration.
func (s *TokenStore) Inactive() ([]core.Token, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("token store is closed")
	}
	tokens, err := s.List()
	if err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	inactive := make([]core.Token, 0)
	for _, token := range tokens {
		expired := !token.ExpiresAt.IsZero() && !token.ExpiresAt.After(now)
		if token.RevokedAt.IsZero() && !expired {
			continue
		}
		inactive = append(inactive, token)
	}
	return inactive, nil
}

// PruneInactive deletes token records that are revoked or past expiration.
func (s *TokenStore) PruneInactive() ([]core.Token, error) {
	inactive, err := s.Inactive()
	if err != nil {
		return nil, err
	}
	deleted := make([]core.Token, 0, len(inactive))
	for _, token := range inactive {
		if err := deleteKey(s.db, tokensBucket, []byte(token.ID)); err != nil {
			return deleted, err
		}
		deleted = append(deleted, token)
	}
	return deleted, nil
}

// Verify checks a bearer token secret against stored verifier hashes.
func (s *TokenStore) Verify(secret string) (core.Token, bool, error) {
	if s == nil || s.db == nil {
		return core.Token{}, false, fmt.Errorf("token store is closed")
	}
	id, ok := parseTokenID(secret)
	if !ok {
		return core.Token{}, false, nil
	}
	var record apiTokenRecord
	exists, err := getJSON(s.db, tokensBucket, []byte(id), &record)
	if err != nil || !exists {
		return core.Token{}, false, err
	}
	if subtle.ConstantTimeCompare([]byte(record.SecretHash), []byte(hashTokenSecret(secret))) != 1 {
		return core.Token{}, false, nil
	}
	now := s.clock.Now().UTC()
	if !record.Token.RevokedAt.IsZero() || (!record.Token.ExpiresAt.IsZero() && !record.Token.ExpiresAt.After(now)) {
		return core.Token{}, false, nil
	}
	return record.Token, true, nil
}

func normalizeScopes(scopes []string) []string {
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

func generateTokenSecret(id core.ID) (string, error) {
	var secret [32]byte
	if _, err := io.ReadFull(rand.Reader, secret[:]); err != nil {
		return "", fmt.Errorf("generate token secret: %w", err)
	}
	return "kro_" + string(id) + "_" + base64.RawURLEncoding.EncodeToString(secret[:]), nil
}

func parseTokenID(secret string) (core.ID, bool) {
	if !strings.HasPrefix(secret, "kro_") {
		return "", false
	}
	rest := strings.TrimPrefix(secret, "kro_")
	id, _, ok := strings.Cut(rest, "_")
	if !ok || id == "" {
		return "", false
	}
	return core.ID(id), true
}

func hashTokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
