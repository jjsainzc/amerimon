CREATE TABLE IF NOT EXISTS audio_tracks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    stored_filename TEXT NOT NULL UNIQUE,
    original_filename TEXT NOT NULL,
    playback_mode TEXT NOT NULL CHECK (playback_mode IN ('on_connection', 'broadcast_once', 'broadcast_silence')),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    file_size INTEGER NOT NULL CHECK (file_size > 0),
    duration_ms INTEGER NOT NULL CHECK (duration_ms > 0),
    sample_rate INTEGER NOT NULL CHECK (sample_rate = 8000),
    bits_per_sample INTEGER NOT NULL CHECK (bits_per_sample = 8),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audio_tracks_mode_enabled
    ON audio_tracks (playback_mode, enabled);

CREATE UNIQUE INDEX IF NOT EXISTS idx_audio_tracks_single_active_welcome
    ON audio_tracks (playback_mode)
    WHERE playback_mode = 'on_connection' AND enabled = 1;
