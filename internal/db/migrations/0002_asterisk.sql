-- Usuarios mínimos para enlazar sesiones de listen-live (nombre/callsign).
CREATE TABLE IF NOT EXISTS users (
    userid    INTEGER PRIMARY KEY AUTOINCREMENT,
    nombre    TEXT,
    callsign  TEXT,
    email     TEXT,
    role      TEXT NOT NULL DEFAULT 'U',
    is_active INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS listenlive (
    listenliveid INTEGER PRIMARY KEY AUTOINCREMENT,
    userid       INTEGER,
    ip_origen    TEXT,
    uuid         TEXT,
    fecha_inicio DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    fecha_fin    DATETIME
);
CREATE INDEX IF NOT EXISTS idx_listenlive_uuid ON listenlive (uuid);

CREATE TABLE IF NOT EXISTS allstarlink_node_activity (
    local_node       TEXT NOT NULL,
    remote_node      TEXT NOT NULL,
    ultima_actividad DATETIME NOT NULL,
    PRIMARY KEY (local_node, remote_node)
);

CREATE TABLE IF NOT EXISTS asterisk_commands (
    id      TEXT PRIMARY KEY,
    label   TEXT,
    command TEXT,
    active  INTEGER DEFAULT 1
);

CREATE TABLE IF NOT EXISTS asterisk_connection_history (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id        TEXT NOT NULL DEFAULT 'default',
    remote_ip          TEXT,
    local_node         TEXT NOT NULL,
    remote_node        TEXT NOT NULL,
    connection_type    TEXT NOT NULL CHECK (connection_type IN ('iax','iaxrpt','echolink')),
    remote_callsign    TEXT,
    remote_description TEXT,
    direction          TEXT,
    mode               TEXT,
    link_state         TEXT,
    first_seen_at      DATETIME NOT NULL,
    last_seen_at       DATETIME NOT NULL,
    disconnected_at    DATETIME,
    disconnect_reason  TEXT
);
CREATE INDEX IF NOT EXISTS idx_connection_pair_history ON asterisk_connection_history (instance_id, local_node, remote_node, first_seen_at);
CREATE INDEX IF NOT EXISTS idx_connection_remote_history ON asterisk_connection_history (instance_id, remote_node, first_seen_at);
CREATE INDEX IF NOT EXISTS idx_connection_open ON asterisk_connection_history (instance_id, local_node, remote_node, disconnected_at);

INSERT OR IGNORE INTO asterisk_commands VALUES ('autopatch_disable', 'Disable Autopatch', 'rpt cmd %s cop 10 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('autopatch_enable', 'Enable Autopatch', 'rpt cmd %s cop 9 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('connect', 'Connect', 'rpt cmd %s ilink %d %s', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('core_uptime', 'Show Asterisk uptime', 'core show uptime', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('core_version', 'Show Asterisk & OS Versions', 'core show version', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('disconnect', 'Disconnect', 'rpt cmd %s ilink 11 %s', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('force_id', 'Force ID (local only)', 'rpt cmd %s status 11 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('iax_channels', 'Show IAX Channel Status', 'iax2 show channels', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('iax_netstats', 'Show IAX Network Status', 'iax2 show netstats', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('iax_peers', 'Show IAX Peers', 'iax2 show peers', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('iax_registry', 'Show IAX Registrations', 'iax2 show registry', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('incoming_disable', 'Disable incoming connections', 'rpt cmd %s cop 49 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('incoming_enable', 'Enable incoming connections', 'rpt cmd %s cop 50 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('linking_disable', 'Disable Linking', 'rpt cmd %s cop 12 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('linking_enable', 'Enable Linking', 'rpt cmd %s cop 11 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('link_status', 'Show Link Status', 'rpt lstats %s', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('localmonitor', 'Local Monitor', 'rpt cmd %s ilink %d %s', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('monitor', 'Monitor', 'rpt cmd %s ilink %d %s', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('node_status', 'Show Node Status', 'rpt stats %s', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('rpt_channels', 'Show RPT Channel Status', 'rpt show channels %s', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('rpt_registrations', 'Show RPT Registrations', 'rpt show registrations', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('rpt_variables', 'Show RPT Variables', 'rpt show variables %s', 1);
INSERT OR IGNORE INTO asterisk_commands VALUES ('say_link_extended_status', 'Say Link Extended Status', 'rpt cmd %s ilink 15 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('say_link_status', 'Say Link Status', 'rpt cmd %s ilink 5 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('say_time_global', 'Say Time of Day (global)', 'rpt cmd %s status 2 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('say_time_local', 'Say Time of Day (local only)', 'rpt cmd %s status 12 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('system_disable', 'Disable System', 'rpt cmd %s cop 3 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('system_enable', 'Enable System', 'rpt cmd %s cop 2 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('system_state_normal', 'System State 0 (normal)', 'rpt cmd %s cop 14 0', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('system_state_rpt_only', 'System State 1 (rpt only)', 'rpt cmd %s cop 14 1', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('telemetry_demand', 'Local Telemetry Output on Demand', 'rpt cmd %s cop 35 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('telemetry_disable', 'Local Telemetry Output Disable', 'rpt cmd %s cop 34 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('telemetry_enable', 'Local Telemetry Output Enable', 'rpt cmd %s cop 33 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('test_tone', 'Test Tone On/Off (global)', 'rpt cmd %s cop 4 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('user_functions_disable', 'Disable User Functions', 'rpt cmd %s cop 18 xxx', 0);
INSERT OR IGNORE INTO asterisk_commands VALUES ('user_functions_enable', 'Enable User Functions', 'rpt cmd %s cop 17 xxx', 0);
