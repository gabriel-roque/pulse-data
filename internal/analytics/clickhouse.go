package analytics

import (
	"context"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/pulse-data/pulse/internal/events"
)

type Store interface {
	Record(context.Context, events.Event) error
	Summary(context.Context, string, string, time.Time, time.Time) ([]Summary, error)
}
type Summary struct {
	Type  string `json:"type"`
	Count uint64 `json:"count"`
}

type ClickHouse struct{ conn clickhouse.Conn }

func NewClickHouse(addr, database string) (ClickHouse, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{Addr: []string{addr}, Auth: clickhouse.Auth{Database: database}})
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = conn.Ping(ctx)
		cancel()
		if err != nil {
			_ = conn.Close()
		}
	}
	return ClickHouse{conn: conn}, err
}
func (c ClickHouse) Close() error                    { return c.conn.Close() }
func (c ClickHouse) Ready(ctx context.Context) error { return c.conn.Ping(ctx) }
func (c ClickHouse) Record(ctx context.Context, e events.Event) error {
	return c.conn.Exec(ctx, `INSERT INTO events (tenant_id,event_id,event_type,event_timestamp,payload) VALUES (?, ?, ?, ?, ?)`, e.TenantID, e.EventID, e.Type, e.Timestamp, e.Payload)
}
func (c ClickHouse) Summary(ctx context.Context, tenantID, eventType string, from, to time.Time) ([]Summary, error) {
	rows, err := c.conn.Query(ctx, `SELECT event_type, uniqExact(event_id) FROM events FINAL WHERE tenant_id=? AND (?='' OR event_type=?) AND event_timestamp>=? AND event_timestamp<? GROUP BY event_type ORDER BY uniqExact(event_id) DESC`, tenantID, eventType, eventType, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.Type, &s.Count); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
