ALTER TABLE audio_tracks ADD COLUMN start_ms INTEGER NOT NULL DEFAULT 0 CHECK (start_ms >= 0);
ALTER TABLE audio_tracks ADD COLUMN end_ms INTEGER NOT NULL DEFAULT 0
    CHECK ((start_ms = 0 AND end_ms = 0) OR (end_ms > start_ms AND end_ms <= duration_ms));
