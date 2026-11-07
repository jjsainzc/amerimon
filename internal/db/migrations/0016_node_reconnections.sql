CREATE TABLE IF NOT EXISTS node_reconnections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node TEXT NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    interval_seconds INTEGER NOT NULL DEFAULT 60 CHECK (interval_seconds BETWEEN 1 AND 86400),
    action TEXT NOT NULL DEFAULT 'connect' CHECK (action IN ('connect', 'monitor', 'localmonitor')),
    permanent INTEGER NOT NULL DEFAULT 0 CHECK (permanent IN (0, 1)),
    next_check_at INTEGER NOT NULL DEFAULT 0,
    last_check_at INTEGER NOT NULL DEFAULT 0,
    last_result TEXT NOT NULL DEFAULT 'RECONNECT_PENDING',
    last_details TEXT NOT NULL DEFAULT ''
);
