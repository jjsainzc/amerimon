package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestAudioSegmentMigrationPreservesExistingTrackAndSchedule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "0015_audio_segments.sql" {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO audio_tracks (id, name, stored_filename, original_filename, playback_mode, enabled, file_size, duration_ms, sample_rate, bits_per_sample)
		VALUES (1, 'Existing', 'existing.wav', 'original.mp3', 'broadcast_silence', 1, 16044, 1000, 8000, 16);
		INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at)
		VALUES (1, 15, '2030-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var start, end, size, duration, interval int64
	var filename, next string
	if err := migrated.QueryRow(`
		SELECT start_ms, end_ms, file_size, duration_ms, stored_filename, interval_minutes, next_play_at
		FROM audio_tracks JOIN audio_broadcast_schedules ON audio_id = audio_tracks.id WHERE audio_tracks.id = 1`).
		Scan(&start, &end, &size, &duration, &filename, &interval, &next); err != nil {
		t.Fatal(err)
	}
	if start != 0 || end != 0 || size != 16044 || duration != 1000 ||
		filename != "existing.wav" || interval != 15 || next != "2030-01-01T00:00:00Z" {
		t.Fatalf("migration changed existing data: %d %d %d %d %s %d %s", start, end, size, duration, filename, interval, next)
	}
	if _, err := migrated.Exec(`UPDATE audio_tracks SET start_ms = 250, end_ms = 750 WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	if err := migrated.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.QueryRow(`SELECT start_ms, end_ms FROM audio_tracks WHERE id = 1`).Scan(&start, &end); err != nil {
		t.Fatal(err)
	}
	if start != 250 || end != 750 {
		t.Fatalf("bounds lost after reopening: %d %d", start, end)
	}
}
