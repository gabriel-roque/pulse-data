package webhook

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	"github.com/pulse-data/pulse/internal/events"
)

type Subscription struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenantId"`
	EventType string    `json:"eventType"`
	Endpoint  string    `json:"endpoint"`
	Secret    []byte    `json:"-"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Store interface {
	CreateSubscription(context.Context, Subscription) (Subscription, error)
	ListSubscriptions(context.Context, string, string) ([]Subscription, error)
}

var ErrSubscriptionNotFound = errors.New("subscription not found")

func NewSecret() ([]byte, error) { b := make([]byte, 32); _, err := rand.Read(b); return b, err }
func NewID() (string, error) {
	b, err := NewSecret()
	if err != nil {
		return "", err
	}
	return "wh_" + base64.RawURLEncoding.EncodeToString(b[:12]), nil
}

type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]Subscription
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{items: make(map[string]Subscription)} }
func (s *MemoryStore) CreateSubscription(_ context.Context, sub Subscription) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[sub.ID] = sub
	return sub, nil
}
func (s *MemoryStore) ListSubscriptions(_ context.Context, tenantID, eventType string) ([]Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Subscription
	for _, sub := range s.items {
		if sub.TenantID == tenantID && sub.EventType == eventType && sub.Enabled {
			out = append(out, sub)
		}
	}
	return out, nil
}

type EventDelivery struct {
	Subscription Subscription
	Event        events.Event
}
