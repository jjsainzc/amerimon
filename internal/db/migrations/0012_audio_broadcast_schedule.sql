CREATE TABLE IF NOT EXISTS audio_broadcast_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    audio_id INTEGER NOT NULL,
    interval_minutes INTEGER NOT NULL,
    last_played_at DATETIME NULL,
    next_play_at DATETIME NULL,
    FOREIGN KEY (audio_id) REFERENCES audio_tracks(id) ON DELETE CASCADE,
    UNIQUE (audio_id),
    CHECK (interval_minutes > 0)
);

CREATE INDEX IF NOT EXISTS idx_audio_broadcast_schedules_due
    ON audio_broadcast_schedules (next_play_at);

CREATE TABLE IF NOT EXISTS audio_playback_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    busy_until TEXT NOT NULL,
    playback_token TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO audio_playback_state (id, busy_until, playback_token)
VALUES (1, '1970-01-01T00:00:00Z', '');
