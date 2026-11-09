package controllers

// Este archivo contiene pruebas para la validación y reproducción de fragmentos de audio.

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"amerimon/internal/config"
	"amerimon/internal/db"
	"github.com/labstack/echo/v4"
)

func TestAudioSegmentExactSamples(t *testing.T) {
	pcm := make([]byte, 16000)
	for i := 0; i < len(pcm)/2; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(i))
	}

	wav := append(recordingWAVHeader(int64(len(pcm))), pcm...)
	original := append([]byte(nil), wav...)
	segment, err := audioSegmentWAV(wav, 250, 750)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(segment[44:], pcm[4000:12000]) || !bytes.Equal(wav, original) {
		t.Fatal("segment samples differ or original changed")
	}
	_, duration, rate, bits, err := normalizeAudioWAV(segment)
	if err != nil || duration.Milliseconds() != 500 || rate != 8000 || bits != 16 {
		t.Fatalf("invalid segment: %v %v %d %d", err, duration, rate, bits)
	}
	for _, bounds := range [][2]int64{{-1, 200}, {500, 500}, {600, 500}, {0, 1001}} {
		if _, err := audioSegmentWAV(wav, bounds[0], bounds[1]); err == nil {
			t.Fatalf("invalid bounds accepted: %v", bounds)
		}
	}
}

func TestAudioSegmentPersistenceAndScheduledSamples(t *testing.T) {
	previous := config.App
	defer func() { config.App = previous }()
	database, err := db.Open(filepath.Join(t.TempDir(), "audio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config.App.DB = database
	config.App.Audio.StorageDir = t.TempDir()
	dir := audioStoragePath()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	filename := "01234567-89ab-cdef-0123-456789abcdef.wav"
	pcm := make([]byte, 16000)
	for i := range pcm {
		pcm[i] = byte(i)
	}
	wav := append(recordingWAVHeader(int64(len(pcm))), pcm...)
	if err := os.WriteFile(filepath.Join(dir, filename), wav, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
			INSERT INTO audio_tracks (id, name, stored_filename, original_filename, playback_mode, enabled, file_size, sample_rate, bits_per_sample, duration_ms)
			VALUES (1, 'Segment', ?, 'test.wav', 'broadcast_silence', 1, ?, 8000, 16, 1000);
			INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at) VALUES (1, 15, '2030-01-01T00:00:00Z')`,
		filename, len(wav)); err != nil {
		t.Fatal(err)
	}
	update := func(payload string, want int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString(payload))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("1")
		if err := UpdateAudioTrack(c); err != nil || rec.Code != want {
			t.Fatalf("update %s: status=%d err=%v body=%s", payload, rec.Code, err, rec.Body.String())
		}
	}
	update(`{"start_ms":250,"end_ms":750}`, http.StatusOK)
	for _, payload := range []string{
		`{"start_ms":-1,"end_ms":750}`, `{"start_ms":750,"end_ms":250}`,
		`{"start_ms":0,"end_ms":1001}`, `{"start_ms":250}`, `{"end_ms":750}`,
		`{"start_ms":250,"end_ms":0}`, `{"start_ms":250.5,"end_ms":750}`,
	} {
		update(payload, http.StatusBadRequest)
	}
	manual, err := loadAudioTrack(context.Background(), 1)
	if err != nil || manual.StartMS != 250 || manual.EndMS != 750 || manual.playbackDurationMS() != 500 {
		t.Fatalf("saved bounds not loaded: %+v err=%v", manual, err)
	}
	scheduled, err := loadScheduledAudioTracks(context.Background())
	if err != nil || len(scheduled) != 1 {
		t.Fatalf("scheduled tracks: %+v err=%v", scheduled, err)
	}
	track := scheduled[0]
	if track.StartMS != manual.StartMS || track.EndMS != manual.EndMS || track.playbackDurationMS() != 500 ||
		track.IntervalMinutes != 15 || track.NextPlayAt.Format("2006-01-02T15:04:05Z") != "2030-01-01T00:00:00Z" {
		t.Fatalf("schedule or bounds changed: %+v", track)
	}
	segment, err := prepareAudioTrackPlayback(track)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, segment))
	if err != nil || !bytes.Equal(data[44:], pcm[4000:12000]) {
		t.Fatalf("scheduled segment samples differ: %v", err)
	}
	original, err := os.ReadFile(filepath.Join(dir, filename))
	if err != nil || !bytes.Equal(original, wav) {
		t.Fatalf("original modified: %v", err)
	}
	update(`{"start_ms":0,"end_ms":0}`, http.StatusOK)
	track, err = loadAudioTrack(context.Background(), 1)
	if err != nil || track.StartMS != 0 || track.EndMS != 0 || track.playbackDurationMS() != 1000 {
		t.Fatalf("reset failed: %+v %v", track, err)
	}
	full, err := prepareAudioTrackPlayback(track)
	if err != nil || full != filename {
		t.Fatalf("reset playback is not original: %s %v", full, err)
	}
}
