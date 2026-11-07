package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Simula una actualización: la base ya tiene datos y falta registrar una migración.
func TestMigrateKeepsDataAndBacksUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")

	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := d.Exec(`INSERT INTO favoritos (numero, descripcion) VALUES ('448511', 'HI3ACS')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`DELETE FROM schema_migrations WHERE name = '0005_favoritos.sql'`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	var n int
	if err := d.QueryRow(`SELECT COUNT(1) FROM favoritos WHERE numero = '448511'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("se perdieron datos del cliente: n=%d err=%v", n, err)
	}
	if err := d.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE name = '0005_favoritos.sql'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("la migración no se volvió a registrar: n=%d err=%v", n, err)
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 1 {
		t.Fatalf("se esperaba 1 respaldo, hay %d", len(backups))
	}
}

func TestNodeReconnectionMigrationPreservesClientData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("CREATE TABLE schema_migrations (name TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "0016_node_reconnections.sql" {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := legacy.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := legacy.Exec("INSERT INTO schema_migrations (name) VALUES (?)", entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := legacy.Exec("INSERT INTO favoritos (numero, descripcion) VALUES ('1998', 'custom client node')"); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	var description string
	if err := migrated.QueryRow("SELECT descripcion FROM favoritos WHERE numero = '1998'").Scan(&description); err != nil || description != "custom client node" {
		t.Fatalf("client settings lost: %q %v", description, err)
	}
	var count int
	if err := migrated.QueryRow("SELECT COUNT(*) FROM node_reconnections").Scan(&count); err != nil || count != 0 {
		t.Fatalf("new scheduler should start empty: %d %v", count, err)
	}
	backups, err := filepath.Glob(path + ".bak-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("missing migration backup: %v %v", backups, err)
	}
}

func TestNodeReconnectionSourceMigrationKeepsExistingSchedules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sources.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("CREATE TABLE schema_migrations (name TEXT PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() >= "0017_node_reconnection_sources.sql" {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := legacy.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := legacy.Exec("INSERT INTO schema_migrations (name) VALUES (?)", entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := legacy.Exec(`INSERT INTO node_reconnections
		(id, node, enabled, interval_seconds, action, permanent, next_check_at, last_check_at, last_result, last_details)
		VALUES (9, '3123456', 1, 120, 'monitor', 1, 123456, 123000, 'RECONNECT_PRESENT', 'client details')`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	var source, target, action, result, details string
	var enabled, interval, permanent, next, last int
	if err := migrated.QueryRow(`SELECT source_node, node, enabled, interval_seconds, action, permanent,
		next_check_at, last_check_at, last_result, last_details FROM node_reconnections WHERE id = 9`).Scan(
		&source, &target, &enabled, &interval, &action, &permanent, &next, &last, &result, &details); err != nil {
		t.Fatal(err)
	}
	if source != "" || target != "3123456" || enabled != 1 || interval != 120 || action != "monitor" ||
		permanent != 1 || next != 123456 || last != 123000 || result != "RECONNECT_PRESENT" || details != "client details" {
		t.Fatalf("legacy entry changed: %q %q %d %d %q %d %d %d %q %q", source, target, enabled, interval, action, permanent, next, last, result, details)
	}
	for _, source := range []string{"50001", "70001"} {
		if _, err := migrated.Exec("INSERT INTO node_reconnections (source_node, node) VALUES (?, '3123456')", source); err != nil {
			t.Fatalf("same target across sources not allowed: %v", err)
		}
	}
	if _, err := migrated.Exec("INSERT INTO node_reconnections (source_node, node) VALUES ('70001', '3123456')"); err == nil {
		t.Fatal("duplicate source/target pair accepted")
	}
}

func TestAudioPlaybackMigrationKeepsTracksAndSchedules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audio.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "0013_audio_playback_format.sql" {
			continue
		}
		if entry.Name() == "0011_audio_track.sql" || entry.Name() == "0012_audio_broadcast_schedule.sql" {
			body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := raw.Exec(string(body)); err != nil {
				t.Fatalf("create legacy audio schema: %v", err)
			}
		}
		if _, err := raw.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, entry.Name()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.Exec(`
		INSERT INTO audio_tracks
		(id, name, stored_filename, original_filename, playback_mode, enabled, file_size, duration_ms, sample_rate, bits_per_sample)
		VALUES
		(1, 'Legacy welcome', '11111111-1111-1111-1111-111111111111.wav', 'welcome.wav', 'on_connection', 1, 48, 2, 8000, 8),
		(2, 'Scheduled', '22222222-2222-2222-2222-222222222222.wav', 'scheduled.wav', 'broadcast_silence', 1, 48, 2, 8000, 8)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at) VALUES (2, 15, '2030-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	migrated, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	var mode string
	if err := migrated.QueryRow(`SELECT playback_mode FROM audio_tracks WHERE id = 1`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "broadcast_once" {
		t.Fatalf("legacy welcome should become one-time broadcast, got %q", mode)
	}
	var interval int
	var next string
	if err := migrated.QueryRow(`SELECT interval_minutes, next_play_at FROM audio_broadcast_schedules WHERE audio_id = 2`).Scan(&interval, &next); err != nil {
		t.Fatal(err)
	}
	if interval != 15 || next != "2030-01-01T00:00:00Z" {
		t.Fatalf("schedule was not preserved: interval=%d next=%q", interval, next)
	}
	if _, err := migrated.Exec(`
		INSERT INTO audio_tracks
		(name, stored_filename, original_filename, playback_mode, enabled, file_size, duration_ms, sample_rate, bits_per_sample)
		VALUES ('New format', '33333333-3333-3333-3333-333333333333.wav', 'new.wav', 'broadcast_once', 1, 48, 2, 8000, 16)`); err != nil {
		t.Fatalf("new 16-bit audio metadata should be accepted: %v", err)
	}
}
