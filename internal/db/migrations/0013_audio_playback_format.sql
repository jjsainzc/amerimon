DROP INDEX IF EXISTS idx_audio_tracks_mode_enabled;
DROP INDEX IF EXISTS idx_audio_tracks_single_active_welcome;
DROP INDEX IF EXISTS idx_audio_broadcast_schedules_due;

ALTER TABLE audio_tracks RENAME TO audio_tracks_legacy;

CREATE TABLE audio_tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    stored_filename TEXT NOT NULL UNIQUE,
    original_filename TEXT NOT NULL,
    playback_mode TEXT NOT NULL CHECK (playback_mode IN ('broadcast_once', 'broadcast_silence')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    file_size INTEGER NOT NULL CHECK (file_size > 0),
    duration_ms INTEGER NOT NULL CHECK (duration_ms > 0),
    sample_rate INTEGER NOT NULL CHECK (sample_rate = 8000),
    bits_per_sample INTEGER NOT NULL CHECK (bits_per_sample IN (8, 16)),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO audio_tracks (
    id, name, stored_filename, original_filename, playback_mode, enabled, file_size,
    duration_ms, sample_rate, bits_per_sample, created_at, updated_at
)
SELECT
    id, name, stored_filename, original_filename,
    CASE WHEN playback_mode = 'on_connection' THEN 'broadcast_once' ELSE playback_mode END,
    enabled, file_size, duration_ms, sample_rate, bits_per_sample, created_at, updated_at
FROM audio_tracks_legacy;

ALTER TABLE audio_broadcast_schedules RENAME TO audio_broadcast_schedules_legacy;

CREATE TABLE audio_broadcast_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    audio_id INTEGER NOT NULL,
    interval_minutes INTEGER NOT NULL,
    last_played_at DATETIME NULL,
    next_play_at DATETIME NULL,
    FOREIGN KEY (audio_id) REFERENCES audio_tracks(id) ON DELETE CASCADE,
    UNIQUE (audio_id),
    CHECK (interval_minutes > 0)
);

INSERT INTO audio_broadcast_schedules (
    id, audio_id, interval_minutes, last_played_at, next_play_at
)
SELECT id, audio_id, interval_minutes, last_played_at, next_play_at
FROM audio_broadcast_schedules_legacy;

DROP TABLE audio_broadcast_schedules_legacy;
DROP TABLE audio_tracks_legacy;

CREATE INDEX idx_audio_tracks_mode_enabled
    ON audio_tracks (playback_mode, enabled);

CREATE INDEX idx_audio_broadcast_schedules_due
    ON audio_broadcast_schedules (next_play_at);
