package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")
var ErrTenantNotFound = errors.New("tenant not found")

type Tenant struct {
	ID         string
	Name       string
	APIKeyHash []byte
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Store interface {
	Create(ctx context.Context, name string) (Tenant, string, error)
	RotateKey(ctx context.Context, tenantID string) (string, error)
	Authenticate(ctx context.Context, key string) (Tenant, error)
	Get(ctx context.Context, tenantID string) (Tenant, error)
}

func NewAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pulse_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func HashAPIKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}

func EqualAPIKeyHash(key string, hash []byte) bool {
	return subtle.ConstantTimeCompare(HashAPIKey(key), hash) == 1
}

func Bearer(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

type MemoryStore struct {
	mu      sync.RWMutex
	tenants map[string]Tenant
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{tenants: make(map[string]Tenant)} }

func (s *MemoryStore) Create(_ context.Context, name string) (Tenant, string, error) {
	if strings.TrimSpace(name) == "" {
		return Tenant{}, "", fmt.Errorf("tenant name is required")
	}
	key, err := NewAPIKey()
	if err != nil {
		return Tenant{}, "", err
	}
	now := time.Now().UTC()
	idKey, err := NewAPIKey()
	if err != nil {
		return Tenant{}, "", err
	}
	id := strings.TrimPrefix(idKey, "pulse_")[:16]
	t := Tenant{ID: "ten_" + id, Name: name, APIKeyHash: HashAPIKey(key), Status: "active", CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	s.tenants[t.ID] = t
	s.mu.Unlock()
	return t, key, nil
}

func (s *MemoryStore) RotateKey(_ context.Context, tenantID string) (string, error) {
	key, err := NewAPIKey()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tenants[tenantID]
	if !ok {
		return "", ErrTenantNotFound
	}
	t.APIKeyHash, t.UpdatedAt = HashAPIKey(key), time.Now().UTC()
	s.tenants[tenantID] = t
	return key, nil
}

func (s *MemoryStore) Authenticate(_ context.Context, key string) (Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tenants {
		if t.Status == "active" && EqualAPIKeyHash(key, t.APIKeyHash) {
			return t, nil
		}
	}
	return Tenant{}, ErrUnauthorized
}

func (s *MemoryStore) Get(_ context.Context, tenantID string) (Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tenants[tenantID]
	if !ok {
		return Tenant{}, ErrTenantNotFound
	}
	return t, nil
}
