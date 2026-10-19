CREATE TABLE IF NOT EXISTS asterisk_operation_logs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at  TEXT NOT NULL,
    action      TEXT NOT NULL,
    local_node  TEXT NOT NULL DEFAULT '',
    remote_node TEXT NOT NULL DEFAULT '',
    mode        TEXT NOT NULL DEFAULT '',
    permanent   INTEGER NOT NULL DEFAULT 0,
    command     TEXT NOT NULL DEFAULT '',
    result      TEXT NOT NULL,
    details     TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_asterisk_operation_logs_created
    ON asterisk_operation_logs (created_at DESC, id DESC);
