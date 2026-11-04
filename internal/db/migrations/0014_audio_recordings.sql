CREATE TABLE audio_recordings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    node TEXT NOT NULL,
    uuid TEXT NOT NULL UNIQUE,
    stored_filename TEXT NOT NULL UNIQUE,
    state TEXT NOT NULL CHECK (state IN ('starting', 'recording', 'paused', 'completed', 'interrupted', 'failed')),
    file_size INTEGER NOT NULL DEFAULT 44,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    finished_at TEXT,
    error_code TEXT NOT NULL DEFAULT '',
    library_track_id INTEGER REFERENCES audio_tracks(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX audio_recordings_single_active
    ON audio_recordings ((1)) WHERE state IN ('starting', 'recording', 'paused');
