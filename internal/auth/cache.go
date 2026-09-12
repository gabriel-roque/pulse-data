package auth

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"
)

type cachedTenant struct {
	tenant  Tenant
	expires time.Time
}

type CachedStore struct {
	inner Store
	ttl   time.Duration
	mu    sync.RWMutex
	items map[string]cachedTenant
}

const maxCachedKeys = 4096

func NewCachedStore(inner Store, ttl time.Duration) Store {
	if ttl <= 0 {
		return inner
	}
	return &CachedStore{inner: inner, ttl: ttl, items: make(map[string]cachedTenant)}
}

func (s *CachedStore) Create(ctx context.Context, name string) (Tenant, string, error) {
	return s.inner.Create(ctx, name)
}

func (s *CachedStore) RotateKey(ctx context.Context, tenantID string) (string, error) {
	key, err := s.inner.RotateKey(ctx, tenantID)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	for apiKey, item := range s.items {
		if item.tenant.ID == tenantID {
			delete(s.items, apiKey)
		}
	}
	s.mu.Unlock()
	return key, nil
}

func (s *CachedStore) Authenticate(ctx context.Context, key string) (Tenant, error) {
	now := time.Now()
	digest := sha256.Sum256([]byte(key))
	cacheKey := string(digest[:])
	s.mu.RLock()
	item, ok := s.items[cacheKey]
	s.mu.RUnlock()
	if ok && now.Before(item.expires) {
		return item.tenant, nil
	}
	if ok {
		s.mu.Lock()
		delete(s.items, cacheKey)
		s.mu.Unlock()
	}
	tenant, err := s.inner.Authenticate(ctx, key)
	if err != nil {
		return Tenant{}, err
	}
	s.mu.Lock()
	for cachedKey, cached := range s.items {
		if !now.Before(cached.expires) {
			delete(s.items, cachedKey)
		}
	}
	if len(s.items) >= maxCachedKeys {
		for cachedKey := range s.items {
			delete(s.items, cachedKey)
			break
		}
	}
	s.items[cacheKey] = cachedTenant{tenant: tenant, expires: now.Add(s.ttl)}
	s.mu.Unlock()
	return tenant, nil
}

func (s *CachedStore) Get(ctx context.Context, tenantID string) (Tenant, error) {
	return s.inner.Get(ctx, tenantID)
}
