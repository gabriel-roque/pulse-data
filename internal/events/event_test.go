package events

import (
	"fmt"
	"testing"
	"time"
)

func validJSON() []byte {
	return []byte(fmt.Sprintf(`{"eventId":"evt_1","type":"payment.completed","timestamp":%q,"payload":{"amount":10}}`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)))
}

func TestDecodeAndTenantDerivation(t *testing.T) {
	e, err := Decode(validJSON(), "ten_a", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if e.TenantID != "ten_a" || e.EventID != "evt_1" {
		t.Fatalf("unexpected event: %+v", e)
	}
}
func TestDecodeRejectsUnknownAndInvalidFields(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"eventId":"evt","type":"x","timestamp":"2020-01-01T00:00:00Z","payload":{},"tenantId":"attacker"}`),
		[]byte(`{"eventId":"evt","type":"x","timestamp":"2020-01-01T00:00:00Z","payload":null}`),
		[]byte(`{"eventId":"evt","type":"x y","timestamp":"2020-01-01T00:00:00Z","payload":{}}`),
	}
	for _, input := range cases {
		if _, err := Decode(input, "ten_a", 1024); err == nil {
			t.Fatalf("expected rejection for %s", input)
		}
	}
}
func TestValidatePayloadLimitAndFutureTimestamp(t *testing.T) {
	e := Event{TenantID: "ten", EventID: "evt", Type: "x", Timestamp: time.Now().Add(time.Hour), Payload: []byte(`{}`)}
	if err := e.Validate(10); err == nil {
		t.Fatal("future event accepted")
	}
	e.Timestamp = time.Now()
	e.Payload = []byte(`{"large":"payload"}`)
	if err := e.Validate(3); err == nil {
		t.Fatal("oversized payload accepted")
	}
}
