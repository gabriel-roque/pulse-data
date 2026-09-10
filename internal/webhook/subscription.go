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
	ClaimDelivery(context.Context, string, string, string) (bool, error)
	// Delivery is at-least-once: a crash after the remote succeeds and before
	// CompleteDelivery can cause the delivery to be attempted again.
	CompleteDelivery(context.Context, string, string, string) error
	ReleaseDelivery(context.Context, string, string, string) error
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
	mu     sync.RWMutex
	items  map[string]Subscription
	claims map[deliveryClaim]deliveryState
	now    func() time.Time
}

type deliveryClaim struct {
	tenantID       string
	eventID        string
	subscriptionID string
}

type deliveryState struct {
	status     string
	leaseUntil time.Time
}

const (
	deliveryPending    = "pending"
	deliveryProcessing = "processing"
	deliveryCompleted  = "completed"
	deliveryLease      = 5 * time.Minute
)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]Subscription), claims: make(map[deliveryClaim]deliveryState), now: time.Now}
}
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

func (s *MemoryStore) ClaimDelivery(_ context.Context, tenantID, eventID, subscriptionID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claims == nil {
		s.claims = make(map[deliveryClaim]deliveryState)
	}
	key := deliveryClaim{tenantID: tenantID, eventID: eventID, subscriptionID: subscriptionID}
	now := s.currentTime()
	if state, ok := s.claims[key]; ok && (state.status == deliveryCompleted || (state.status == deliveryProcessing && state.leaseUntil.After(now))) {
		return false, nil
	}
	s.claims[key] = deliveryState{status: deliveryProcessing, leaseUntil: now.Add(deliveryLease)}
	return true, nil
}

func (s *MemoryStore) CompleteDelivery(_ context.Context, tenantID, eventID, subscriptionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := deliveryClaim{tenantID: tenantID, eventID: eventID, subscriptionID: subscriptionID}
	state, ok := s.claims[key]
	if !ok || state.status == deliveryCompleted || !state.leaseUntil.After(s.currentTime()) {
		return nil
	}
	state.status = deliveryCompleted
	state.leaseUntil = time.Time{}
	s.claims[key] = state
	return nil
}

func (s *MemoryStore) ReleaseDelivery(_ context.Context, tenantID, eventID, subscriptionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := deliveryClaim{tenantID: tenantID, eventID: eventID, subscriptionID: subscriptionID}
	state, ok := s.claims[key]
	if !ok || state.status != deliveryProcessing || !state.leaseUntil.After(s.currentTime()) {
		return nil
	}
	state.status = deliveryPending
	state.leaseUntil = time.Time{}
	s.claims[key] = state
	return nil
}

func (s *MemoryStore) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

type EventDelivery struct {
	Subscription Subscription
	Event        events.Event
}
