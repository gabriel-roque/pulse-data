package auth

import (
	"context"
	"sync"
	"testing"
)

func TestMemoryTenantCreateAuthenticateAndRotate(t *testing.T) {
	s := NewMemoryStore()
	tenant, key, err := s.Create(context.Background(), "Acme")
	if err != nil {
		t.Fatal(err)
	}
	if len(tenant.APIKeyHash) != 32 || string(tenant.APIKeyHash) == key {
		t.Fatal("raw API key was stored")
	}
	if got, err := s.Authenticate(context.Background(), key); err != nil || got.ID != tenant.ID {
		t.Fatalf("authenticate: %v", err)
	}
	rotated, err := s.RotateKey(context.Background(), tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(context.Background(), key); err == nil {
		t.Fatal("old key remained valid")
	}
	if _, err := s.Authenticate(context.Background(), rotated); err != nil {
		t.Fatal(err)
	}
}
func TestMemoryStoreConcurrentAuthentication(t *testing.T) {
	s := NewMemoryStore()
	_, key, err := s.Create(context.Background(), "Acme")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Authenticate(context.Background(), key); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
