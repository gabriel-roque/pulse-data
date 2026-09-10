package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

var (
	ErrInvalidEvent = errors.New("invalid event")
	ErrMissingField = errors.New("missing required field")
)

type Event struct {
	TenantID  string          `json:"tenantId"`
	EventID   string          `json:"eventId"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

func (e Event) Validate(maxPayloadBytes int) error {
	if strings.TrimSpace(e.TenantID) == "" {
		return fmt.Errorf("%w: tenantId", ErrMissingField)
	}
	if len(e.EventID) < 1 || len(e.EventID) > 200 {
		return fmt.Errorf("%w: eventId", ErrInvalidEvent)
	}
	if len(e.Type) < 1 || len(e.Type) > 200 || strings.ContainsAny(e.Type, " \t\r\n") {
		return fmt.Errorf("%w: type", ErrInvalidEvent)
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp", ErrMissingField)
	}
	if e.Timestamp.After(time.Now().Add(5 * time.Minute)) {
		return fmt.Errorf("%w: timestamp is too far in the future", ErrInvalidEvent)
	}
	if len(e.Payload) == 0 || bytes.Equal(e.Payload, []byte("null")) || !json.Valid(e.Payload) {
		return fmt.Errorf("%w: payload must be valid JSON", ErrInvalidEvent)
	}
	if maxPayloadBytes > 0 && len(e.Payload) > maxPayloadBytes {
		return fmt.Errorf("%w: payload exceeds limit", ErrInvalidEvent)
	}
	return nil
}

func Decode(data []byte, tenantID string, maxPayloadBytes int) (Event, error) {
	var input struct {
		EventID   string          `json:"eventId"`
		Type      string          `json:"type"`
		Timestamp time.Time       `json:"timestamp"`
		Payload   json.RawMessage `json:"payload"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return Event{}, fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Event{}, fmt.Errorf("%w: multiple JSON values", ErrInvalidEvent)
	}
	e := Event{TenantID: tenantID, EventID: input.EventID, Type: input.Type, Timestamp: input.Timestamp, Payload: input.Payload}
	return e, e.Validate(maxPayloadBytes)
}
