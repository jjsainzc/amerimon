CREATE TABLE node_reconnections_with_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_node TEXT NOT NULL DEFAULT '',
    node TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    interval_seconds INTEGER NOT NULL DEFAULT 60 CHECK (interval_seconds BETWEEN 1 AND 86400),
    action TEXT NOT NULL DEFAULT 'connect' CHECK (action IN ('connect', 'monitor', 'localmonitor')),
    permanent INTEGER NOT NULL DEFAULT 0 CHECK (permanent IN (0, 1)),
    next_check_at INTEGER NOT NULL DEFAULT 0,
    last_check_at INTEGER NOT NULL DEFAULT 0,
    last_result TEXT NOT NULL DEFAULT 'RECONNECT_PENDING',
    last_details TEXT NOT NULL DEFAULT '',
    UNIQUE (source_node, node)
);

INSERT INTO node_reconnections_with_sources
    (id, node, enabled, interval_seconds, action, permanent, next_check_at, last_check_at, last_result, last_details)
SELECT id, node, enabled, interval_seconds, action, permanent, next_check_at, last_check_at, last_result, last_details
FROM node_reconnections;

DROP TABLE node_reconnections;
ALTER TABLE node_reconnections_with_sources RENAME TO node_reconnections;
