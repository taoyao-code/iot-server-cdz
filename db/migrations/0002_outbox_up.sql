-- Outbox for downlink commands
CREATE TABLE IF NOT EXISTS outbound_queue (
    id BIGSERIAL PRIMARY KEY,
    device_id BIGINT NOT NULL,
    phy_id TEXT,
    port_no INT,
    cmd INT NOT NULL,
    payload BYTEA,
    priority INT NOT NULL DEFAULT 100,
    status INT NOT NULL DEFAULT 0,         -- 0=pending,1=sent,2=done,3=failed
    retry_count INT NOT NULL DEFAULT 0,
    not_before TIMESTAMPTZ,
    timeout_sec INT NOT NULL DEFAULT 15,
    correlation_id TEXT,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbound_queue_status_notbefore
    ON outbound_queue(status, not_before, priority, created_at);

CREATE INDEX IF NOT EXISTS idx_outbound_queue_device
    ON outbound_queue(device_id);

CREATE UNIQUE INDEX IF NOT EXISTS uid_outbound_correlation
    ON outbound_queue(correlation_id) WHERE correlation_id IS NOT NULL;

-- trigger to update updated_at
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END; $$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_outbound_updated_at ON outbound_queue;
CREATE TRIGGER trg_outbound_updated_at
BEFORE UPDATE ON outbound_queue
FOR EACH ROW EXECUTE PROCEDURE set_updated_at();

