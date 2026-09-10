package persistence

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pulse-data/pulse/internal/auth"
	"github.com/pulse-data/pulse/internal/events"
	"github.com/pulse-data/pulse/internal/webhook"
)

type Postgres struct{ pool *pgxpool.Pool }

func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Postgres{pool: p}, nil
}
func (p *Postgres) Close()                          { p.pool.Close() }
func (p *Postgres) Ready(ctx context.Context) error { return p.pool.Ping(ctx) }

func (p *Postgres) Create(ctx context.Context, name string) (auth.Tenant, string, error) {
	key, err := auth.NewAPIKey()
	if err != nil {
		return auth.Tenant{}, "", err
	}
	id := "ten_" + key[len(key)-16:]
	var t auth.Tenant
	err = p.pool.QueryRow(ctx, `INSERT INTO tenants(id,name,api_key_hash,status) VALUES($1,$2,$3,'active') RETURNING id,name,api_key_hash,status,created_at,updated_at`, id, name, auth.HashAPIKey(key)).Scan(&t.ID, &t.Name, &t.APIKeyHash, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	return t, key, err
}
func (p *Postgres) RotateKey(ctx context.Context, tenantID string) (string, error) {
	key, err := auth.NewAPIKey()
	if err != nil {
		return "", err
	}
	result, err := p.pool.Exec(ctx, `UPDATE tenants SET api_key_hash=$1,updated_at=now() WHERE id=$2 AND status='active'`, auth.HashAPIKey(key), tenantID)
	if err != nil {
		return "", err
	}
	if result.RowsAffected() != 1 {
		return "", auth.ErrTenantNotFound
	}
	return key, nil
}
func (p *Postgres) Authenticate(ctx context.Context, key string) (auth.Tenant, error) {
	var t auth.Tenant
	err := p.pool.QueryRow(ctx, `SELECT id,name,api_key_hash,status,created_at,updated_at FROM tenants WHERE status='active' AND api_key_hash=$1`, auth.HashAPIKey(key)).Scan(&t.ID, &t.Name, &t.APIKeyHash, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Tenant{}, auth.ErrUnauthorized
	}
	return t, err
}
func (p *Postgres) Get(ctx context.Context, tenantID string) (auth.Tenant, error) {
	var t auth.Tenant
	err := p.pool.QueryRow(ctx, `SELECT id,name,api_key_hash,status,created_at,updated_at FROM tenants WHERE id=$1`, tenantID).Scan(&t.ID, &t.Name, &t.APIKeyHash, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Tenant{}, auth.ErrTenantNotFound
	}
	return t, err
}

func (p *Postgres) SaveEvent(ctx context.Context, event events.Event) (bool, error) {
	result, err := p.pool.Exec(ctx, `INSERT INTO events(tenant_id,event_id,event_type,event_timestamp,payload) VALUES($1,$2,$3,$4,$5) ON CONFLICT(tenant_id,event_id) DO NOTHING`, event.TenantID, event.EventID, event.Type, event.Timestamp, event.Payload)
	return result.RowsAffected() == 1, err
}

type EventStore interface {
	SaveEvent(context.Context, events.Event) (bool, error)
}

func (p *Postgres) CreateSubscription(ctx context.Context, sub webhook.Subscription) (webhook.Subscription, error) {
	_, err := p.pool.Exec(ctx, `INSERT INTO webhook_subscriptions(id,tenant_id,event_type,endpoint,secret,enabled) VALUES($1,$2,$3,$4,$5,$6)`, sub.ID, sub.TenantID, sub.EventType, sub.Endpoint, sub.Secret, sub.Enabled)
	return sub, err
}
func (p *Postgres) ListSubscriptions(ctx context.Context, tenantID, eventType string) ([]webhook.Subscription, error) {
	rows, err := p.pool.Query(ctx, `SELECT id,tenant_id,event_type,endpoint,secret,enabled,created_at,updated_at FROM webhook_subscriptions WHERE tenant_id=$1 AND event_type=$2 AND enabled=true`, tenantID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []webhook.Subscription
	for rows.Next() {
		var s webhook.Subscription
		if err := rows.Scan(&s.ID, &s.TenantID, &s.EventType, &s.Endpoint, &s.Secret, &s.Enabled, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *Postgres) ClaimDelivery(ctx context.Context, tenantID, eventID, subscriptionID string) (bool, error) {
	var claimed int
	err := p.pool.QueryRow(ctx, `
		INSERT INTO webhook_delivery_claims(tenant_id,event_id,subscription_id,status,lease_until,claimed_at)
		VALUES($1,$2,$3,'processing',now() + interval '1 hour',now())
		ON CONFLICT (tenant_id,event_id,subscription_id) DO UPDATE
		SET status='processing', lease_until=now() + interval '1 hour', claimed_at=now(), completed_at=NULL
		WHERE webhook_delivery_claims.status <> 'completed'
		  AND (webhook_delivery_claims.lease_until IS NULL OR webhook_delivery_claims.lease_until <= now())
		RETURNING 1`, tenantID, eventID, subscriptionID).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && claimed == 1, err
}

func (p *Postgres) CompleteDelivery(ctx context.Context, tenantID, eventID, subscriptionID string) error {
	_, err := p.pool.Exec(ctx, `UPDATE webhook_delivery_claims SET status='completed', lease_until=NULL, completed_at=now() WHERE tenant_id=$1 AND event_id=$2 AND subscription_id=$3 AND status='processing' AND lease_until > now()`, tenantID, eventID, subscriptionID)
	return err
}

func (p *Postgres) ReleaseDelivery(ctx context.Context, tenantID, eventID, subscriptionID string) error {
	_, err := p.pool.Exec(ctx, `UPDATE webhook_delivery_claims SET status='pending', lease_until=NULL WHERE tenant_id=$1 AND event_id=$2 AND subscription_id=$3 AND status='processing' AND lease_until > now()`, tenantID, eventID, subscriptionID)
	return err
}
