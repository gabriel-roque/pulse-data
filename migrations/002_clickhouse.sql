CREATE TABLE IF NOT EXISTS events (
    tenant_id String,
    event_id String,
    event_type LowCardinality(String),
    event_timestamp DateTime64(3, 'UTC'),
    payload String,
    received_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(received_at)
ORDER BY (tenant_id, event_timestamp, event_id);
