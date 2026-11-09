package controllers

// Este archivo contiene pruebas para la vista previa local de pistas de audio.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

func TestAudioTrackFileLocalPreview(t *testing.T) {
	previous := config.App
	defer func() { config.App = previous }()
	root := t.TempDir()
	database, err := db.Open(filepath.Join(root, "preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config.App.DB = database
	config.App.Audio.StorageDir = filepath.Join(root, "sounds")
	config.App.AMI.RPTConfigPath = filepath.Join(root, "rpt.conf")
	filename := "01234567-89ab-cdef-0123-456789abcdef.wav"
	_, err = database.Exec(`INSERT INTO audio_tracks
		(id, name, stored_filename, original_filename, playback_mode, enabled, file_size, sample_rate, bits_per_sample, duration_ms)
		VALUES (1, 'Preview', ?, 'preview.wav', 'broadcast_silence', 0, 52, 8000, 16, 1)`, filename)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at) VALUES (1, 60, '2099-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(audioStoragePath(), 0o750); err != nil {
		t.Fatal(err)
	}
	wav := testPCMMonoWAV(8000, 16, 1)
	path := filepath.Join(audioStoragePath(), filename)
	if err := os.WriteFile(path, wav, 0o600); err != nil {
		t.Fatal(err)
	}
	request := func(id, byteRange string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if byteRange != "" {
			req.Header.Set("Range", byteRange)
		}
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(id)
		if err := AudioTrackFile(c); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	rec := request("1", "")
	if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), wav) ||
		rec.Header().Get("Content-Type") != "audio/wav" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("local preview: %d %v %s", rec.Code, rec.Header(), rec.Body)
	}
	rec = request("1", "bytes=0-3")
	if rec.Code != 206 || rec.Body.String() != "RIFF" {
		t.Fatalf("range preview: %d %s", rec.Code, rec.Body)
	}
	var enabled int
	var next, last string
	if err := database.QueryRow(`SELECT t.enabled, s.next_play_at, COALESCE(s.last_played_at, '') FROM audio_tracks t
		JOIN audio_broadcast_schedules s ON s.audio_id=t.id WHERE t.id=1`).Scan(&enabled, &next, &last); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || next != "2099-01-01T00:00:00Z" || last != "" {
		t.Fatalf("preview changed broadcast settings: %d %s %s", enabled, next, last)
	}
	for _, tc := range []struct {
		id     string
		status int
	}{{"0", 400}, {"invalid", 400}, {"2", 404}} {
		if rec := request(tc.id, ""); rec.Code != tc.status {
			t.Fatalf("id %s: %d, want %d", tc.id, rec.Code, tc.status)
		}
	}
	if _, err := database.Exec(`UPDATE audio_tracks SET stored_filename='../outside.wav' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if rec := request("1", ""); rec.Code != 500 {
		t.Fatalf("invalid filename accepted: %d", rec.Code)
	}
}
