-- devices: 设备基础信息
CREATE TABLE IF NOT EXISTS devices (
    id              BIGSERIAL PRIMARY KEY,
    phy_id          TEXT NOT NULL UNIQUE,
    iccid           TEXT,
    imei            TEXT,
    model           TEXT,
    firmware_ver    TEXT,
    last_seen_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_devices_last_seen_at ON devices(last_seen_at);

-- ports: 设备端口快照
CREATE TABLE IF NOT EXISTS ports (
    device_id       BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    port_no         INT NOT NULL,
    status          INT NOT NULL DEFAULT 0,
    power_w         INT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(device_id, port_no)
);

-- orders: 订单
CREATE TABLE IF NOT EXISTS orders (
    id              BIGSERIAL PRIMARY KEY,
    device_id       BIGINT NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    port_no         INT NOT NULL,
    order_no        TEXT NOT NULL,
    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    kwh_0p01        BIGINT,      -- 以 0.01kWh 为单位
    amount_cent     BIGINT,      -- 分
    status          INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(order_no)
);
CREATE INDEX IF NOT EXISTS idx_orders_device_port ON orders(device_id, port_no);
CREATE INDEX IF NOT EXISTS idx_orders_time ON orders(start_time, end_time);

-- cmd_log: 指令日志（上下行）
CREATE TABLE IF NOT EXISTS cmd_log (
    id              BIGSERIAL PRIMARY KEY,
    device_id       BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    msg_id          INT,
    cmd             INT NOT NULL,
    direction       SMALLINT NOT NULL, -- 0=UP,1=DOWN
    payload         BYTEA,
    success         BOOLEAN,
    err_code        INT,
    duration_ms     INT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cmdlog_device_time ON cmd_log(device_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cmdlog_msg ON cmd_log(msg_id, cmd);

-- outbound_queue: 下行任务队列
CREATE TABLE IF NOT EXISTS outbound_queue (
    id              BIGSERIAL PRIMARY KEY,
    device_id       BIGINT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    port_no         INT,
    cmd             INT NOT NULL,
    payload         BYTEA,
    priority        SMALLINT NOT NULL DEFAULT 10,
    retries         INT NOT NULL DEFAULT 0,
    not_before      TIMESTAMPTZ,
    status          SMALLINT NOT NULL DEFAULT 0, -- 0=pending,1=sent,2=done,3=dead
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_outq_sched ON outbound_queue(status, priority, not_before NULLS FIRST, created_at);


