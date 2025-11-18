/*
 Navicat Premium Dump SQL

 Source Server         : iot-测试服
 Source Server Type    : PostgreSQL
 Source Server Version : 150014 (150014)
 Source Host           : 182.43.177.92:5433
 Source Catalog        : iot_server
 Source Schema         : public

 Target Server Type    : PostgreSQL
 Target Server Version : 150014 (150014)
 File Encoding         : 65001

 Date: 18/11/2025 17:53:16
*/


-- ----------------------------
-- Sequence structure for card_balance_logs_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."card_balance_logs_id_seq";
CREATE SEQUENCE "public"."card_balance_logs_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "public"."card_balance_logs_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for card_transactions_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."card_transactions_id_seq";
CREATE SEQUENCE "public"."card_transactions_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "public"."card_transactions_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for cards_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."cards_id_seq";
CREATE SEQUENCE "public"."cards_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "public"."cards_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for cmd_log_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."cmd_log_id_seq";
CREATE SEQUENCE "public"."cmd_log_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "public"."cmd_log_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for device_params_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."device_params_id_seq";
CREATE SEQUENCE "public"."device_params_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 2147483647
START 1
CACHE 1;
ALTER SEQUENCE "public"."device_params_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for devices_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."devices_id_seq";
CREATE SEQUENCE "public"."devices_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "public"."devices_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for events_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."events_id_seq";
CREATE SEQUENCE "public"."events_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "public"."events_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for gateway_sockets_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."gateway_sockets_id_seq";
CREATE SEQUENCE "public"."gateway_sockets_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 2147483647
START 1
CACHE 1;
ALTER SEQUENCE "public"."gateway_sockets_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for health_check_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."health_check_id_seq";
CREATE SEQUENCE "public"."health_check_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 2147483647
START 1
CACHE 1;
ALTER SEQUENCE "public"."health_check_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for orders_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."orders_id_seq";
CREATE SEQUENCE "public"."orders_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "public"."orders_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for ota_tasks_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."ota_tasks_id_seq";
CREATE SEQUENCE "public"."ota_tasks_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 2147483647
START 1
CACHE 1;
ALTER SEQUENCE "public"."ota_tasks_id_seq" OWNER TO "iot";

-- ----------------------------
-- Sequence structure for outbound_queue_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "public"."outbound_queue_id_seq";
CREATE SEQUENCE "public"."outbound_queue_id_seq" 
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "public"."outbound_queue_id_seq" OWNER TO "iot";

-- ----------------------------
-- Table structure for card_balance_logs
-- ----------------------------
DROP TABLE IF EXISTS "public"."card_balance_logs";
CREATE TABLE "public"."card_balance_logs" (
  "id" int8 NOT NULL DEFAULT nextval('card_balance_logs_id_seq'::regclass),
  "card_no" varchar(32) COLLATE "pg_catalog"."default" NOT NULL,
  "transaction_id" int8,
  "change_type" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "amount" numeric(10,2) NOT NULL,
  "balance_before" numeric(10,2) NOT NULL,
  "balance_after" numeric(10,2) NOT NULL,
  "description" text COLLATE "pg_catalog"."default",
  "created_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "public"."card_balance_logs" OWNER TO "iot";
COMMENT ON TABLE "public"."card_balance_logs" IS '卡片余额变更记录表';

-- ----------------------------
-- Table structure for card_transactions
-- ----------------------------
DROP TABLE IF EXISTS "public"."card_transactions";
CREATE TABLE "public"."card_transactions" (
  "id" int8 NOT NULL DEFAULT nextval('card_transactions_id_seq'::regclass),
  "card_no" varchar(32) COLLATE "pg_catalog"."default" NOT NULL,
  "device_id" varchar(64) COLLATE "pg_catalog"."default" NOT NULL,
  "phy_id" varchar(16) COLLATE "pg_catalog"."default" NOT NULL,
  "order_no" varchar(64) COLLATE "pg_catalog"."default" NOT NULL,
  "charge_mode" int4 NOT NULL,
  "amount" numeric(10,2),
  "duration_minutes" int4,
  "power_watts" int4,
  "energy_kwh" numeric(10,3),
  "status" varchar(20) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'pending'::character varying,
  "start_time" timestamp(6),
  "end_time" timestamp(6),
  "created_at" timestamp(6) NOT NULL DEFAULT now(),
  "updated_at" timestamp(6) NOT NULL DEFAULT now(),
  "failure_reason" text COLLATE "pg_catalog"."default",
  "price_per_kwh" numeric(10,4),
  "service_fee_rate" numeric(5,4),
  "total_amount" numeric(10,2)
)
;
ALTER TABLE "public"."card_transactions" OWNER TO "iot";
COMMENT ON COLUMN "public"."card_transactions"."order_no" IS '订单号，唯一标识，格式：CARD{timestamp}{random}';
COMMENT ON COLUMN "public"."card_transactions"."charge_mode" IS '充电模式：1=按时长, 2=按电量, 3=按功率, 4=充满自停';
COMMENT ON COLUMN "public"."card_transactions"."status" IS '订单状态：pending=待确认, charging=充电中, completed=已完成, cancelled=已取消, failed=失败';
COMMENT ON TABLE "public"."card_transactions" IS '刷卡充电交易记录表';

-- ----------------------------
-- Table structure for cards
-- ----------------------------
DROP TABLE IF EXISTS "public"."cards";
CREATE TABLE "public"."cards" (
  "id" int8 NOT NULL DEFAULT nextval('cards_id_seq'::regclass),
  "card_no" varchar(32) COLLATE "pg_catalog"."default" NOT NULL,
  "balance" numeric(10,2) NOT NULL DEFAULT 0,
  "status" varchar(20) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'active'::character varying,
  "user_id" int8,
  "description" text COLLATE "pg_catalog"."default",
  "created_at" timestamp(6) NOT NULL DEFAULT now(),
  "updated_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "public"."cards" OWNER TO "iot";
COMMENT ON COLUMN "public"."cards"."card_no" IS '卡号，唯一标识';
COMMENT ON COLUMN "public"."cards"."balance" IS '卡片余额，单位：元';
COMMENT ON COLUMN "public"."cards"."status" IS '卡片状态：active=正常, inactive=未激活, blocked=已冻结';
COMMENT ON TABLE "public"."cards" IS '充电卡片信息表';

-- ----------------------------
-- Table structure for cmd_log
-- ----------------------------
DROP TABLE IF EXISTS "public"."cmd_log";
CREATE TABLE "public"."cmd_log" (
  "id" int8 NOT NULL DEFAULT nextval('cmd_log_id_seq'::regclass),
  "device_id" int8 NOT NULL,
  "msg_id" int4,
  "cmd" int4 NOT NULL,
  "direction" int2 NOT NULL,
  "payload" bytea,
  "success" bool,
  "err_code" int4,
  "duration_ms" int4,
  "created_at" timestamptz(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "public"."cmd_log" OWNER TO "iot";

-- ----------------------------
-- Table structure for device_params
-- ----------------------------
DROP TABLE IF EXISTS "public"."device_params";
CREATE TABLE "public"."device_params" (
  "id" int4 NOT NULL DEFAULT nextval('device_params_id_seq'::regclass),
  "device_id" int8 NOT NULL,
  "param_id" int4 NOT NULL,
  "param_value" bytea,
  "msg_id" int4,
  "status" int4 NOT NULL DEFAULT 0,
  "created_at" timestamp(6) NOT NULL DEFAULT now(),
  "confirmed_at" timestamp(6),
  "updated_at" timestamp(6) NOT NULL DEFAULT now(),
  "error_message" text COLLATE "pg_catalog"."default"
)
;
ALTER TABLE "public"."device_params" OWNER TO "iot";
COMMENT ON COLUMN "public"."device_params"."device_id" IS '设备ID（外键）';
COMMENT ON COLUMN "public"."device_params"."param_id" IS '参数ID（BKV协议定义）';
COMMENT ON COLUMN "public"."device_params"."param_value" IS '参数值（二进制）';
COMMENT ON COLUMN "public"."device_params"."msg_id" IS '消息ID（用于ACK确认）';
COMMENT ON COLUMN "public"."device_params"."status" IS '状态：0=待确认, 1=已确认, 2=失败';
COMMENT ON COLUMN "public"."device_params"."created_at" IS '创建时间';
COMMENT ON COLUMN "public"."device_params"."confirmed_at" IS '确认时间';
COMMENT ON COLUMN "public"."device_params"."updated_at" IS '更新时间';
COMMENT ON COLUMN "public"."device_params"."error_message" IS '错误信息（失败时记录）';
COMMENT ON TABLE "public"."device_params" IS 'BKV设备参数写入记录（P0修复：持久化存储）';

-- ----------------------------
-- Table structure for devices
-- ----------------------------
DROP TABLE IF EXISTS "public"."devices";
CREATE TABLE "public"."devices" (
  "id" int8 NOT NULL DEFAULT nextval('devices_id_seq'::regclass),
  "phy_id" text COLLATE "pg_catalog"."default" NOT NULL,
  "gateway_id" text COLLATE "pg_catalog"."default",
  "iccid" text COLLATE "pg_catalog"."default",
  "imei" text COLLATE "pg_catalog"."default",
  "model" text COLLATE "pg_catalog"."default",
  "firmware_ver" text COLLATE "pg_catalog"."default",
  "rssi" int4,
  "fw_ver" text COLLATE "pg_catalog"."default",
  "last_seen_at" timestamptz(6),
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "public"."devices" OWNER TO "iot";

-- ----------------------------
-- Table structure for events
-- ----------------------------
DROP TABLE IF EXISTS "public"."events";
CREATE TABLE "public"."events" (
  "id" int8 NOT NULL DEFAULT nextval('events_id_seq'::regclass),
  "order_no" varchar(32) COLLATE "pg_catalog"."default" NOT NULL,
  "event_type" varchar(32) COLLATE "pg_catalog"."default" NOT NULL,
  "event_data" jsonb NOT NULL,
  "sequence_no" int4 NOT NULL,
  "status" int4 NOT NULL DEFAULT 0,
  "retry_count" int4 NOT NULL DEFAULT 0,
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "pushed_at" timestamptz(6),
  "error_message" text COLLATE "pg_catalog"."default"
)
;
ALTER TABLE "public"."events" OWNER TO "iot";

-- ----------------------------
-- Table structure for gateway_sockets
-- ----------------------------
DROP TABLE IF EXISTS "public"."gateway_sockets";
CREATE TABLE "public"."gateway_sockets" (
  "id" int4 NOT NULL DEFAULT nextval('gateway_sockets_id_seq'::regclass),
  "gateway_id" varchar(50) COLLATE "pg_catalog"."default" NOT NULL,
  "socket_no" int2 NOT NULL,
  "socket_mac" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "socket_uid" varchar(20) COLLATE "pg_catalog"."default",
  "channel" int2,
  "status" int2 DEFAULT 0,
  "signal_strength" int2,
  "last_seen_at" timestamptz(6),
  "created_at" timestamptz(6) DEFAULT now(),
  "updated_at" timestamptz(6) DEFAULT now()
)
;
ALTER TABLE "public"."gateway_sockets" OWNER TO "iot";
COMMENT ON COLUMN "public"."gateway_sockets"."gateway_id" IS '网关物理ID';
COMMENT ON COLUMN "public"."gateway_sockets"."socket_no" IS '插座编号(1-250)';
COMMENT ON COLUMN "public"."gateway_sockets"."socket_mac" IS '插座MAC地址';
COMMENT ON COLUMN "public"."gateway_sockets"."socket_uid" IS '插座唯一标识';
COMMENT ON COLUMN "public"."gateway_sockets"."channel" IS '信道(1-15)';
COMMENT ON COLUMN "public"."gateway_sockets"."status" IS '状态: 0=离线, 1=在线, 2=故障';
COMMENT ON COLUMN "public"."gateway_sockets"."signal_strength" IS '信号强度(RSSI)';
COMMENT ON TABLE "public"."gateway_sockets" IS 'BKV网关插座管理表';

-- ----------------------------
-- Table structure for health_check
-- ----------------------------
DROP TABLE IF EXISTS "public"."health_check";
CREATE TABLE "public"."health_check" (
  "id" int4 NOT NULL DEFAULT nextval('health_check_id_seq'::regclass),
  "last_check" timestamp(6) DEFAULT CURRENT_TIMESTAMP
)
;
ALTER TABLE "public"."health_check" OWNER TO "iot";

-- ----------------------------
-- Table structure for orders
-- ----------------------------
DROP TABLE IF EXISTS "public"."orders";
CREATE TABLE "public"."orders" (
  "id" int8 NOT NULL DEFAULT nextval('orders_id_seq'::regclass),
  "device_id" int8 NOT NULL,
  "port_no" int4 NOT NULL,
  "order_no" text COLLATE "pg_catalog"."default" NOT NULL,
  "start_time" timestamptz(6),
  "end_time" timestamptz(6),
  "kwh_0p01" int8,
  "amount_cent" int8,
  "status" int4 NOT NULL DEFAULT 0,
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "updated_at" timestamptz(6) NOT NULL DEFAULT now(),
  "failure_reason" varchar(255) COLLATE "pg_catalog"."default",
  "test_session_id" text COLLATE "pg_catalog"."default",
  "charge_mode" int4 NOT NULL DEFAULT 1
)
;
ALTER TABLE "public"."orders" OWNER TO "iot";
COMMENT ON COLUMN "public"."orders"."status" IS '订单状态: 0=pending, 1=confirmed, 2=charging, 3=timeout, 4=cancelled, 5=completed, 6=failed, 7=stopped, 8=cancelling, 9=stopping, 10=interrupted';
COMMENT ON COLUMN "public"."orders"."test_session_id" IS 'E2E 测试会话标识，用于按 test_session_id 追踪订单和完整链路';
COMMENT ON COLUMN "public"."orders"."charge_mode" IS '充电模式: 1=按时长, 2=按电量, 3=按功率, 4=充满自停';

-- ----------------------------
-- Table structure for orders_port_migration_backup
-- ----------------------------
DROP TABLE IF EXISTS "public"."orders_port_migration_backup";
CREATE TABLE "public"."orders_port_migration_backup" (
  "order_no" text COLLATE "pg_catalog"."default",
  "port_no" int4,
  "device_id" int8,
  "status" int4,
  "created_at" timestamptz(6)
)
;
ALTER TABLE "public"."orders_port_migration_backup" OWNER TO "iot";

-- ----------------------------
-- Table structure for ota_tasks
-- ----------------------------
DROP TABLE IF EXISTS "public"."ota_tasks";
CREATE TABLE "public"."ota_tasks" (
  "id" int4 NOT NULL DEFAULT nextval('ota_tasks_id_seq'::regclass),
  "device_id" int8,
  "target_type" int2 NOT NULL,
  "target_socket_no" int2,
  "firmware_version" varchar(20) COLLATE "pg_catalog"."default" NOT NULL,
  "ftp_server" varchar(50) COLLATE "pg_catalog"."default" NOT NULL,
  "ftp_port" int4 NOT NULL DEFAULT 21,
  "file_name" varchar(50) COLLATE "pg_catalog"."default" NOT NULL,
  "file_size" int8,
  "status" int2 DEFAULT 0,
  "progress" int2 DEFAULT 0,
  "error_msg" text COLLATE "pg_catalog"."default",
  "msg_id" int4,
  "started_at" timestamptz(6),
  "completed_at" timestamptz(6),
  "created_at" timestamptz(6) DEFAULT now(),
  "updated_at" timestamptz(6) DEFAULT now()
)
;
ALTER TABLE "public"."ota_tasks" OWNER TO "iot";
COMMENT ON COLUMN "public"."ota_tasks"."target_type" IS '升级目标类型: 1=DTU, 2=插座';
COMMENT ON COLUMN "public"."ota_tasks"."target_socket_no" IS '插座升级时的插座编号';
COMMENT ON COLUMN "public"."ota_tasks"."status" IS '状态: 0=待发送, 1=已下发, 2=升级中, 3=成功, 4=失败';
COMMENT ON COLUMN "public"."ota_tasks"."progress" IS '升级进度百分比 0-100';
COMMENT ON TABLE "public"."ota_tasks" IS 'BKV设备OTA升级任务表';

-- ----------------------------
-- Table structure for outbound_queue
-- ----------------------------
DROP TABLE IF EXISTS "public"."outbound_queue";
CREATE TABLE "public"."outbound_queue" (
  "id" int8 NOT NULL DEFAULT nextval('outbound_queue_id_seq'::regclass),
  "device_id" int8 NOT NULL,
  "port_no" int4,
  "cmd" int4 NOT NULL,
  "payload" bytea,
  "priority" int2 NOT NULL DEFAULT 10,
  "retries" int4 NOT NULL DEFAULT 0,
  "not_before" timestamptz(6),
  "status" int2 NOT NULL DEFAULT 0,
  "created_at" timestamptz(6) NOT NULL DEFAULT now(),
  "msg_id" int4
)
;
ALTER TABLE "public"."outbound_queue" OWNER TO "iot";

-- ----------------------------
-- Table structure for ports
-- ----------------------------
DROP TABLE IF EXISTS "public"."ports";
CREATE TABLE "public"."ports" (
  "device_id" int8 NOT NULL,
  "port_no" int4 NOT NULL,
  "status" int4 NOT NULL DEFAULT 0,
  "power_w" int4,
  "updated_at" timestamptz(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "public"."ports" OWNER TO "iot";

-- ----------------------------
-- Table structure for schema_migrations
-- ----------------------------
DROP TABLE IF EXISTS "public"."schema_migrations";
CREATE TABLE "public"."schema_migrations" (
  "version" int8 NOT NULL,
  "applied_at" timestamptz(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "public"."schema_migrations" OWNER TO "iot";

-- ----------------------------
-- Function structure for pg_stat_statements
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pg_stat_statements"("showtext" bool, OUT "userid" oid, OUT "dbid" oid, OUT "toplevel" bool, OUT "queryid" int8, OUT "query" text, OUT "plans" int8, OUT "total_plan_time" float8, OUT "min_plan_time" float8, OUT "max_plan_time" float8, OUT "mean_plan_time" float8, OUT "stddev_plan_time" float8, OUT "calls" int8, OUT "total_exec_time" float8, OUT "min_exec_time" float8, OUT "max_exec_time" float8, OUT "mean_exec_time" float8, OUT "stddev_exec_time" float8, OUT "rows" int8, OUT "shared_blks_hit" int8, OUT "shared_blks_read" int8, OUT "shared_blks_dirtied" int8, OUT "shared_blks_written" int8, OUT "local_blks_hit" int8, OUT "local_blks_read" int8, OUT "local_blks_dirtied" int8, OUT "local_blks_written" int8, OUT "temp_blks_read" int8, OUT "temp_blks_written" int8, OUT "blk_read_time" float8, OUT "blk_write_time" float8, OUT "temp_blk_read_time" float8, OUT "temp_blk_write_time" float8, OUT "wal_records" int8, OUT "wal_fpi" int8, OUT "wal_bytes" numeric, OUT "jit_functions" int8, OUT "jit_generation_time" float8, OUT "jit_inlining_count" int8, OUT "jit_inlining_time" float8, OUT "jit_optimization_count" int8, OUT "jit_optimization_time" float8, OUT "jit_emission_count" int8, OUT "jit_emission_time" float8);
CREATE FUNCTION "public"."pg_stat_statements"(IN "showtext" bool, OUT "userid" oid, OUT "dbid" oid, OUT "toplevel" bool, OUT "queryid" int8, OUT "query" text, OUT "plans" int8, OUT "total_plan_time" float8, OUT "min_plan_time" float8, OUT "max_plan_time" float8, OUT "mean_plan_time" float8, OUT "stddev_plan_time" float8, OUT "calls" int8, OUT "total_exec_time" float8, OUT "min_exec_time" float8, OUT "max_exec_time" float8, OUT "mean_exec_time" float8, OUT "stddev_exec_time" float8, OUT "rows" int8, OUT "shared_blks_hit" int8, OUT "shared_blks_read" int8, OUT "shared_blks_dirtied" int8, OUT "shared_blks_written" int8, OUT "local_blks_hit" int8, OUT "local_blks_read" int8, OUT "local_blks_dirtied" int8, OUT "local_blks_written" int8, OUT "temp_blks_read" int8, OUT "temp_blks_written" int8, OUT "blk_read_time" float8, OUT "blk_write_time" float8, OUT "temp_blk_read_time" float8, OUT "temp_blk_write_time" float8, OUT "wal_records" int8, OUT "wal_fpi" int8, OUT "wal_bytes" numeric, OUT "jit_functions" int8, OUT "jit_generation_time" float8, OUT "jit_inlining_count" int8, OUT "jit_inlining_time" float8, OUT "jit_optimization_count" int8, OUT "jit_optimization_time" float8, OUT "jit_emission_count" int8, OUT "jit_emission_time" float8)
  RETURNS SETOF "pg_catalog"."record" AS '$libdir/pg_stat_statements', 'pg_stat_statements_1_10'
  LANGUAGE c VOLATILE STRICT
  COST 1
  ROWS 1000;
ALTER FUNCTION "public"."pg_stat_statements"("showtext" bool, OUT "userid" oid, OUT "dbid" oid, OUT "toplevel" bool, OUT "queryid" int8, OUT "query" text, OUT "plans" int8, OUT "total_plan_time" float8, OUT "min_plan_time" float8, OUT "max_plan_time" float8, OUT "mean_plan_time" float8, OUT "stddev_plan_time" float8, OUT "calls" int8, OUT "total_exec_time" float8, OUT "min_exec_time" float8, OUT "max_exec_time" float8, OUT "mean_exec_time" float8, OUT "stddev_exec_time" float8, OUT "rows" int8, OUT "shared_blks_hit" int8, OUT "shared_blks_read" int8, OUT "shared_blks_dirtied" int8, OUT "shared_blks_written" int8, OUT "local_blks_hit" int8, OUT "local_blks_read" int8, OUT "local_blks_dirtied" int8, OUT "local_blks_written" int8, OUT "temp_blks_read" int8, OUT "temp_blks_written" int8, OUT "blk_read_time" float8, OUT "blk_write_time" float8, OUT "temp_blk_read_time" float8, OUT "temp_blk_write_time" float8, OUT "wal_records" int8, OUT "wal_fpi" int8, OUT "wal_bytes" numeric, OUT "jit_functions" int8, OUT "jit_generation_time" float8, OUT "jit_inlining_count" int8, OUT "jit_inlining_time" float8, OUT "jit_optimization_count" int8, OUT "jit_optimization_time" float8, OUT "jit_emission_count" int8, OUT "jit_emission_time" float8) OWNER TO "iot";

-- ----------------------------
-- Function structure for pg_stat_statements_info
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pg_stat_statements_info"(OUT "dealloc" int8, OUT "stats_reset" timestamptz);
CREATE FUNCTION "public"."pg_stat_statements_info"(OUT "dealloc" int8, OUT "stats_reset" timestamptz)
  RETURNS "pg_catalog"."record" AS '$libdir/pg_stat_statements', 'pg_stat_statements_info'
  LANGUAGE c VOLATILE STRICT
  COST 1;
ALTER FUNCTION "public"."pg_stat_statements_info"(OUT "dealloc" int8, OUT "stats_reset" timestamptz) OWNER TO "iot";

-- ----------------------------
-- Function structure for pg_stat_statements_reset
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."pg_stat_statements_reset"("userid" oid, "dbid" oid, "queryid" int8);
CREATE FUNCTION "public"."pg_stat_statements_reset"("userid" oid=0, "dbid" oid=0, "queryid" int8=0)
  RETURNS "pg_catalog"."void" AS '$libdir/pg_stat_statements', 'pg_stat_statements_reset_1_7'
  LANGUAGE c VOLATILE STRICT
  COST 1;
ALTER FUNCTION "public"."pg_stat_statements_reset"("userid" oid, "dbid" oid, "queryid" int8) OWNER TO "iot";

-- ----------------------------
-- Function structure for set_updated_at
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."set_updated_at"();
CREATE FUNCTION "public"."set_updated_at"()
  RETURNS "pg_catalog"."trigger" AS $BODY$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END; $BODY$
  LANGUAGE plpgsql VOLATILE
  COST 100;
ALTER FUNCTION "public"."set_updated_at"() OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_generate_v1
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_generate_v1"();
CREATE FUNCTION "public"."uuid_generate_v1"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_generate_v1'
  LANGUAGE c VOLATILE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_generate_v1"() OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_generate_v1mc
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_generate_v1mc"();
CREATE FUNCTION "public"."uuid_generate_v1mc"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_generate_v1mc'
  LANGUAGE c VOLATILE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_generate_v1mc"() OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_generate_v3
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_generate_v3"("namespace" uuid, "name" text);
CREATE FUNCTION "public"."uuid_generate_v3"("namespace" uuid, "name" text)
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_generate_v3'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_generate_v3"("namespace" uuid, "name" text) OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_generate_v4
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_generate_v4"();
CREATE FUNCTION "public"."uuid_generate_v4"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_generate_v4'
  LANGUAGE c VOLATILE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_generate_v4"() OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_generate_v5
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_generate_v5"("namespace" uuid, "name" text);
CREATE FUNCTION "public"."uuid_generate_v5"("namespace" uuid, "name" text)
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_generate_v5'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_generate_v5"("namespace" uuid, "name" text) OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_nil
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_nil"();
CREATE FUNCTION "public"."uuid_nil"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_nil'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_nil"() OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_ns_dns
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_ns_dns"();
CREATE FUNCTION "public"."uuid_ns_dns"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_ns_dns'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_ns_dns"() OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_ns_oid
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_ns_oid"();
CREATE FUNCTION "public"."uuid_ns_oid"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_ns_oid'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_ns_oid"() OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_ns_url
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_ns_url"();
CREATE FUNCTION "public"."uuid_ns_url"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_ns_url'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_ns_url"() OWNER TO "iot";

-- ----------------------------
-- Function structure for uuid_ns_x500
-- ----------------------------
DROP FUNCTION IF EXISTS "public"."uuid_ns_x500"();
CREATE FUNCTION "public"."uuid_ns_x500"()
  RETURNS "pg_catalog"."uuid" AS '$libdir/uuid-ossp', 'uuid_ns_x500'
  LANGUAGE c IMMUTABLE STRICT
  COST 1;
ALTER FUNCTION "public"."uuid_ns_x500"() OWNER TO "iot";

-- ----------------------------
-- View structure for pg_stat_statements_info
-- ----------------------------
DROP VIEW IF EXISTS "public"."pg_stat_statements_info";
CREATE VIEW "public"."pg_stat_statements_info" AS  SELECT pg_stat_statements_info.dealloc,
    pg_stat_statements_info.stats_reset
   FROM pg_stat_statements_info() pg_stat_statements_info(dealloc, stats_reset);
ALTER TABLE "public"."pg_stat_statements_info" OWNER TO "iot";

-- ----------------------------
-- View structure for pg_stat_statements
-- ----------------------------
DROP VIEW IF EXISTS "public"."pg_stat_statements";
CREATE VIEW "public"."pg_stat_statements" AS  SELECT pg_stat_statements.userid,
    pg_stat_statements.dbid,
    pg_stat_statements.toplevel,
    pg_stat_statements.queryid,
    pg_stat_statements.query,
    pg_stat_statements.plans,
    pg_stat_statements.total_plan_time,
    pg_stat_statements.min_plan_time,
    pg_stat_statements.max_plan_time,
    pg_stat_statements.mean_plan_time,
    pg_stat_statements.stddev_plan_time,
    pg_stat_statements.calls,
    pg_stat_statements.total_exec_time,
    pg_stat_statements.min_exec_time,
    pg_stat_statements.max_exec_time,
    pg_stat_statements.mean_exec_time,
    pg_stat_statements.stddev_exec_time,
    pg_stat_statements.rows,
    pg_stat_statements.shared_blks_hit,
    pg_stat_statements.shared_blks_read,
    pg_stat_statements.shared_blks_dirtied,
    pg_stat_statements.shared_blks_written,
    pg_stat_statements.local_blks_hit,
    pg_stat_statements.local_blks_read,
    pg_stat_statements.local_blks_dirtied,
    pg_stat_statements.local_blks_written,
    pg_stat_statements.temp_blks_read,
    pg_stat_statements.temp_blks_written,
    pg_stat_statements.blk_read_time,
    pg_stat_statements.blk_write_time,
    pg_stat_statements.temp_blk_read_time,
    pg_stat_statements.temp_blk_write_time,
    pg_stat_statements.wal_records,
    pg_stat_statements.wal_fpi,
    pg_stat_statements.wal_bytes,
    pg_stat_statements.jit_functions,
    pg_stat_statements.jit_generation_time,
    pg_stat_statements.jit_inlining_count,
    pg_stat_statements.jit_inlining_time,
    pg_stat_statements.jit_optimization_count,
    pg_stat_statements.jit_optimization_time,
    pg_stat_statements.jit_emission_count,
    pg_stat_statements.jit_emission_time
   FROM pg_stat_statements(true) pg_stat_statements(userid, dbid, toplevel, queryid, query, plans, total_plan_time, min_plan_time, max_plan_time, mean_plan_time, stddev_plan_time, calls, total_exec_time, min_exec_time, max_exec_time, mean_exec_time, stddev_exec_time, rows, shared_blks_hit, shared_blks_read, shared_blks_dirtied, shared_blks_written, local_blks_hit, local_blks_read, local_blks_dirtied, local_blks_written, temp_blks_read, temp_blks_written, blk_read_time, blk_write_time, temp_blk_read_time, temp_blk_write_time, wal_records, wal_fpi, wal_bytes, jit_functions, jit_generation_time, jit_inlining_count, jit_inlining_time, jit_optimization_count, jit_optimization_time, jit_emission_count, jit_emission_time);
ALTER TABLE "public"."pg_stat_statements" OWNER TO "iot";

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."card_balance_logs_id_seq"
OWNED BY "public"."card_balance_logs"."id";
SELECT setval('"public"."card_balance_logs_id_seq"', 1, false);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."card_transactions_id_seq"
OWNED BY "public"."card_transactions"."id";
SELECT setval('"public"."card_transactions_id_seq"', 1, false);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."cards_id_seq"
OWNED BY "public"."cards"."id";
SELECT setval('"public"."cards_id_seq"', 4, true);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."cmd_log_id_seq"
OWNED BY "public"."cmd_log"."id";
SELECT setval('"public"."cmd_log_id_seq"', 61591, true);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."device_params_id_seq"
OWNED BY "public"."device_params"."id";
SELECT setval('"public"."device_params_id_seq"', 1, false);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."devices_id_seq"
OWNED BY "public"."devices"."id";
SELECT setval('"public"."devices_id_seq"', 113086, true);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."events_id_seq"
OWNED BY "public"."events"."id";
SELECT setval('"public"."events_id_seq"', 1, false);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."gateway_sockets_id_seq"
OWNED BY "public"."gateway_sockets"."id";
SELECT setval('"public"."gateway_sockets_id_seq"', 1, false);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."health_check_id_seq"
OWNED BY "public"."health_check"."id";
SELECT setval('"public"."health_check_id_seq"', 2, true);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."orders_id_seq"
OWNED BY "public"."orders"."id";
SELECT setval('"public"."orders_id_seq"', 137, true);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."ota_tasks_id_seq"
OWNED BY "public"."ota_tasks"."id";
SELECT setval('"public"."ota_tasks_id_seq"', 1, false);

-- ----------------------------
-- Alter sequences owned by
-- ----------------------------
ALTER SEQUENCE "public"."outbound_queue_id_seq"
OWNED BY "public"."outbound_queue"."id";
SELECT setval('"public"."outbound_queue_id_seq"', 1, false);

-- ----------------------------
-- Indexes structure for table card_balance_logs
-- ----------------------------
CREATE INDEX "idx_balance_logs_card_no" ON "public"."card_balance_logs" USING btree (
  "card_no" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_balance_logs_created_at" ON "public"."card_balance_logs" USING btree (
  "created_at" "pg_catalog"."timestamp_ops" DESC NULLS FIRST
);
CREATE INDEX "idx_balance_logs_transaction_id" ON "public"."card_balance_logs" USING btree (
  "transaction_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table card_balance_logs
-- ----------------------------
ALTER TABLE "public"."card_balance_logs" ADD CONSTRAINT "card_balance_logs_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table card_transactions
-- ----------------------------
CREATE INDEX "idx_card_transactions_card_no" ON "public"."card_transactions" USING btree (
  "card_no" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_card_transactions_created_at" ON "public"."card_transactions" USING btree (
  "created_at" "pg_catalog"."timestamp_ops" DESC NULLS FIRST
);
CREATE INDEX "idx_card_transactions_device_id" ON "public"."card_transactions" USING btree (
  "device_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_card_transactions_device_status" ON "public"."card_transactions" USING btree (
  "device_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST,
  "status" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_card_transactions_order_no" ON "public"."card_transactions" USING btree (
  "order_no" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_card_transactions_phy_id" ON "public"."card_transactions" USING btree (
  "phy_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_card_transactions_status" ON "public"."card_transactions" USING btree (
  "status" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table card_transactions
-- ----------------------------
ALTER TABLE "public"."card_transactions" ADD CONSTRAINT "card_transactions_order_no_key" UNIQUE ("order_no");

-- ----------------------------
-- Primary Key structure for table card_transactions
-- ----------------------------
ALTER TABLE "public"."card_transactions" ADD CONSTRAINT "card_transactions_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table cards
-- ----------------------------
CREATE INDEX "idx_cards_card_no" ON "public"."cards" USING btree (
  "card_no" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_cards_status" ON "public"."cards" USING btree (
  "status" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_cards_user_id" ON "public"."cards" USING btree (
  "user_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table cards
-- ----------------------------
ALTER TABLE "public"."cards" ADD CONSTRAINT "cards_card_no_key" UNIQUE ("card_no");

-- ----------------------------
-- Primary Key structure for table cards
-- ----------------------------
ALTER TABLE "public"."cards" ADD CONSTRAINT "cards_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table cmd_log
-- ----------------------------
CREATE INDEX "idx_cmdlog_device_time" ON "public"."cmd_log" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "created_at" "pg_catalog"."timestamptz_ops" DESC NULLS FIRST
);
CREATE INDEX "idx_cmdlog_msg" ON "public"."cmd_log" USING btree (
  "msg_id" "pg_catalog"."int4_ops" ASC NULLS LAST,
  "cmd" "pg_catalog"."int4_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table cmd_log
-- ----------------------------
ALTER TABLE "public"."cmd_log" ADD CONSTRAINT "cmd_log_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table device_params
-- ----------------------------
CREATE INDEX "idx_device_params_device" ON "public"."device_params" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);
CREATE INDEX "idx_device_params_pending" ON "public"."device_params" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "param_id" "pg_catalog"."int4_ops" ASC NULLS LAST
) WHERE status = 0;
CREATE INDEX "idx_device_params_updated" ON "public"."device_params" USING btree (
  "updated_at" "pg_catalog"."timestamp_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table device_params
-- ----------------------------
ALTER TABLE "public"."device_params" ADD CONSTRAINT "device_params_device_id_param_id_key" UNIQUE ("device_id", "param_id");

-- ----------------------------
-- Primary Key structure for table device_params
-- ----------------------------
ALTER TABLE "public"."device_params" ADD CONSTRAINT "device_params_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table devices
-- ----------------------------
CREATE INDEX "idx_devices_gateway_id" ON "public"."devices" USING btree (
  "gateway_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
) WHERE gateway_id IS NOT NULL;
CREATE INDEX "idx_devices_last_seen" ON "public"."devices" USING btree (
  "last_seen_at" "pg_catalog"."timestamptz_ops" DESC NULLS FIRST
) WHERE last_seen_at IS NOT NULL;
COMMENT ON INDEX "public"."idx_devices_last_seen" IS '优化在线设备查询';
CREATE INDEX "idx_devices_last_seen_at" ON "public"."devices" USING btree (
  "last_seen_at" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table devices
-- ----------------------------
ALTER TABLE "public"."devices" ADD CONSTRAINT "devices_phy_id_key" UNIQUE ("phy_id");

-- ----------------------------
-- Primary Key structure for table devices
-- ----------------------------
ALTER TABLE "public"."devices" ADD CONSTRAINT "devices_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table events
-- ----------------------------
CREATE INDEX "idx_events_order_no" ON "public"."events" USING btree (
  "order_no" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_events_retry" ON "public"."events" USING btree (
  "status" "pg_catalog"."int4_ops" ASC NULLS LAST,
  "retry_count" "pg_catalog"."int4_ops" ASC NULLS LAST,
  "created_at" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
) WHERE status = 2 AND retry_count < 5;
CREATE INDEX "idx_events_status_created" ON "public"."events" USING btree (
  "status" "pg_catalog"."int4_ops" ASC NULLS LAST,
  "created_at" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
) WHERE status = ANY (ARRAY[0, 2]);

-- ----------------------------
-- Uniques structure for table events
-- ----------------------------
ALTER TABLE "public"."events" ADD CONSTRAINT "events_order_seq" UNIQUE ("order_no", "sequence_no");

-- ----------------------------
-- Primary Key structure for table events
-- ----------------------------
ALTER TABLE "public"."events" ADD CONSTRAINT "events_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table gateway_sockets
-- ----------------------------
CREATE INDEX "idx_gateway_sockets_gateway" ON "public"."gateway_sockets" USING btree (
  "gateway_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_gateway_sockets_mac" ON "public"."gateway_sockets" USING btree (
  "socket_mac" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);
CREATE INDEX "idx_gateway_sockets_status" ON "public"."gateway_sockets" USING btree (
  "gateway_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST,
  "status" "pg_catalog"."int2_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table gateway_sockets
-- ----------------------------
ALTER TABLE "public"."gateway_sockets" ADD CONSTRAINT "gateway_sockets_gateway_id_socket_no_key" UNIQUE ("gateway_id", "socket_no");

-- ----------------------------
-- Primary Key structure for table gateway_sockets
-- ----------------------------
ALTER TABLE "public"."gateway_sockets" ADD CONSTRAINT "gateway_sockets_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Primary Key structure for table health_check
-- ----------------------------
ALTER TABLE "public"."health_check" ADD CONSTRAINT "health_check_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table orders
-- ----------------------------
CREATE INDEX "idx_orders_charging_by_device" ON "public"."orders" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "status" "pg_catalog"."int4_ops" ASC NULLS LAST,
  "updated_at" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
) WHERE status = 2;
CREATE INDEX "idx_orders_device_port" ON "public"."orders" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "port_no" "pg_catalog"."int4_ops" ASC NULLS LAST
);
CREATE INDEX "idx_orders_interrupted" ON "public"."orders" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "status" "pg_catalog"."int4_ops" ASC NULLS LAST,
  "updated_at" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
) WHERE status = 10;
CREATE INDEX "idx_orders_time" ON "public"."orders" USING btree (
  "start_time" "pg_catalog"."timestamptz_ops" ASC NULLS LAST,
  "end_time" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
);

-- ----------------------------
-- Uniques structure for table orders
-- ----------------------------
ALTER TABLE "public"."orders" ADD CONSTRAINT "orders_order_no_key" UNIQUE ("order_no");

-- ----------------------------
-- Primary Key structure for table orders
-- ----------------------------
ALTER TABLE "public"."orders" ADD CONSTRAINT "orders_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table ota_tasks
-- ----------------------------
CREATE INDEX "idx_ota_tasks_created" ON "public"."ota_tasks" USING btree (
  "created_at" "pg_catalog"."timestamptz_ops" DESC NULLS FIRST
);
CREATE INDEX "idx_ota_tasks_device" ON "public"."ota_tasks" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);
CREATE INDEX "idx_ota_tasks_status" ON "public"."ota_tasks" USING btree (
  "status" "pg_catalog"."int2_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table ota_tasks
-- ----------------------------
ALTER TABLE "public"."ota_tasks" ADD CONSTRAINT "ota_tasks_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table outbound_queue
-- ----------------------------
CREATE INDEX "idx_outbound_queue_device" ON "public"."outbound_queue" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);
CREATE INDEX "idx_outbound_queue_device_msg" ON "public"."outbound_queue" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "msg_id" "pg_catalog"."int4_ops" ASC NULLS LAST
) WHERE msg_id IS NOT NULL;
CREATE INDEX "idx_outbound_queue_status_notbefore" ON "public"."outbound_queue" USING btree (
  "status" "pg_catalog"."int2_ops" ASC NULLS LAST,
  "not_before" "pg_catalog"."timestamptz_ops" ASC NULLS LAST,
  "priority" "pg_catalog"."int2_ops" ASC NULLS LAST,
  "created_at" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
);
CREATE INDEX "idx_outbound_status_priority" ON "public"."outbound_queue" USING btree (
  "status" "pg_catalog"."int2_ops" ASC NULLS LAST,
  "priority" "pg_catalog"."int2_ops" DESC NULLS FIRST,
  "created_at" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
) WHERE status = ANY (ARRAY[0, 1]);
COMMENT ON INDEX "public"."idx_outbound_status_priority" IS '优化下行队列扫描';
CREATE INDEX "idx_outq_sched" ON "public"."outbound_queue" USING btree (
  "status" "pg_catalog"."int2_ops" ASC NULLS LAST,
  "priority" "pg_catalog"."int2_ops" ASC NULLS LAST,
  "not_before" "pg_catalog"."timestamptz_ops" ASC NULLS FIRST,
  "created_at" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
);

-- ----------------------------
-- Triggers structure for table outbound_queue
-- ----------------------------
CREATE TRIGGER "trg_outbound_updated_at" BEFORE UPDATE ON "public"."outbound_queue"
FOR EACH ROW
EXECUTE PROCEDURE "public"."set_updated_at"();

-- ----------------------------
-- Primary Key structure for table outbound_queue
-- ----------------------------
ALTER TABLE "public"."outbound_queue" ADD CONSTRAINT "outbound_queue_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table ports
-- ----------------------------
CREATE INDEX "idx_ports_device_no" ON "public"."ports" USING btree (
  "device_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "port_no" "pg_catalog"."int4_ops" ASC NULLS LAST
);
COMMENT ON INDEX "public"."idx_ports_device_no" IS '优化端口状态查询';

-- ----------------------------
-- Primary Key structure for table ports
-- ----------------------------
ALTER TABLE "public"."ports" ADD CONSTRAINT "ports_pkey" PRIMARY KEY ("device_id", "port_no");

-- ----------------------------
-- Primary Key structure for table schema_migrations
-- ----------------------------
ALTER TABLE "public"."schema_migrations" ADD CONSTRAINT "schema_migrations_pkey" PRIMARY KEY ("version");

-- ----------------------------
-- Foreign Keys structure for table card_balance_logs
-- ----------------------------
ALTER TABLE "public"."card_balance_logs" ADD CONSTRAINT "fk_balance_logs_card" FOREIGN KEY ("card_no") REFERENCES "public"."cards" ("card_no") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table card_transactions
-- ----------------------------
ALTER TABLE "public"."card_transactions" ADD CONSTRAINT "fk_card_transactions_card" FOREIGN KEY ("card_no") REFERENCES "public"."cards" ("card_no") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table cmd_log
-- ----------------------------
ALTER TABLE "public"."cmd_log" ADD CONSTRAINT "cmd_log_device_id_fkey" FOREIGN KEY ("device_id") REFERENCES "public"."devices" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table device_params
-- ----------------------------
ALTER TABLE "public"."device_params" ADD CONSTRAINT "device_params_device_id_fkey" FOREIGN KEY ("device_id") REFERENCES "public"."devices" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table orders
-- ----------------------------
ALTER TABLE "public"."orders" ADD CONSTRAINT "orders_device_id_fkey" FOREIGN KEY ("device_id") REFERENCES "public"."devices" ("id") ON DELETE RESTRICT ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table ota_tasks
-- ----------------------------
ALTER TABLE "public"."ota_tasks" ADD CONSTRAINT "ota_tasks_device_id_fkey" FOREIGN KEY ("device_id") REFERENCES "public"."devices" ("id") ON DELETE NO ACTION ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table outbound_queue
-- ----------------------------
ALTER TABLE "public"."outbound_queue" ADD CONSTRAINT "outbound_queue_device_id_fkey" FOREIGN KEY ("device_id") REFERENCES "public"."devices" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table ports
-- ----------------------------
ALTER TABLE "public"."ports" ADD CONSTRAINT "ports_device_id_fkey" FOREIGN KEY ("device_id") REFERENCES "public"."devices" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
