package controllers

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

func recordingFixture(t *testing.T) *AudioRecordingControl {
	t.Helper()
	primary, _, _ := primaryNodeFixture(t)
	config.App.AMI.RPTConfigPath = primary.rptPath
	config.App.DBPath = filepath.Join(t.TempDir(), "amerimon.db")
	database, err := db.Open(config.App.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	config.App.DB = database
	config.App.Audio.StorageDir = t.TempDir()
	p := NewAudioRecordingControl(config.NewAudioStreamManager())
	p.originate = func(channel, uuid string) error {
		if channel != "Local/616265@amerimon-record-node/n" {
			t.Fatalf("wrong recording source: %s", channel)
		}
		p.manager.Broadcast(uuid, make([]byte, 320))
		return nil
	}
	p.hangup = func(string) error { return nil }
	t.Cleanup(func() {
		if p.active != nil {
			if err := p.active.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	return p
}

func recordingRequest(t *testing.T, handler echo.HandlerFunc, method string, id int64, body any) *httptest.ResponseRecorder {
	t.Helper()
	c, rec := nodeTestContext(method, body)
	c.SetParamNames("id")
	c.SetParamValues(strconv.FormatInt(id, 10))
	if err := handler(c); err != nil {
		t.Fatal(err)
	}
	return rec
}

func startTestRecording(t *testing.T, p *AudioRecordingControl) AudioRecording {
	t.Helper()
	rec := recordingRequest(t, p.Start, http.MethodPost, 0, map[string]string{"name": "Recording test"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("start recording: %d %s", rec.Code, rec.Body)
	}
	return p.active.snapshot()
}

func recordingTone() []byte {
	frame := make([]byte, 320)
	for i := 0; i < len(frame); i += 2 {
		value := int16(4000)
		if i%4 == 0 {
			value = -value
		}
		binary.LittleEndian.PutUint16(frame[i:], uint16(value))
	}
	return frame
}

func TestAudioRecordingInputLevel(t *testing.T) {
	p := recordingFixture(t)
	r := startTestRecording(t, p)
	if level := p.active.snapshot().LevelDBFS; level != -60 {
		t.Fatalf("silence level = %v", level)
	}
	p.manager.Broadcast(r.UUID, recordingTone())
	want := 20 * math.Log10(4000.0/32768)
	if level := p.active.snapshot().LevelDBFS; math.Abs(level-want) > 0.001 {
		t.Fatalf("RMS level = %v, want %v", level, want)
	}
	list := recordingRequest(t, p.List, http.MethodGet, 0, nil)
	var response struct {
		Data []AudioRecording `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || math.Abs(response.Data[0].LevelDBFS-want) > 0.001 {
		t.Fatalf("input level absent from API: %s", list.Body)
	}
	recordingRequest(t, p.Change, http.MethodPatch, r.ID, map[string]string{"action": "pause"})
	p.active.mu.Lock()
	p.active.levelAt = time.Now().Add(-2 * time.Second)
	p.active.mu.Unlock()
	if level := p.active.snapshot().LevelDBFS; level != -60 {
		t.Fatalf("stale input level = %v", level)
	}
	p.manager.Broadcast(r.UUID, recordingTone())
	if level := p.active.snapshot().LevelDBFS; math.Abs(level-want) > 0.001 {
		t.Fatalf("manual pause must still measure incoming audio: %v", level)
	}
	recordingRequest(t, p.Change, http.MethodPatch, r.ID, map[string]string{"action": "stop"})
	if level := p.active.snapshot().LevelDBFS; level != -60 {
		t.Fatalf("stopped input level = %v", level)
	}
}

func TestAudioRecordingAutomaticallyOmitsSilenceAndRespectsManualPause(t *testing.T) {
	p := recordingFixture(t)
	r := startTestRecording(t, p)
	quiet := make([]byte, 320)
	for i := 0; i < len(quiet); i += 2 {
		binary.LittleEndian.PutUint16(quiet[i:], 100)
	}
	for range 50 {
		p.manager.Broadcast(r.UUID, quiet)
	}
	if got := p.active.snapshot(); got.FileSize != 44 || !got.AutoPaused {
		t.Fatalf("silence stored as audio: %+v", got)
	}
	p.manager.Broadcast(r.UUID, recordingTone())
	if got := p.active.snapshot(); got.FileSize != 44+3200+320 || got.AutoPaused {
		t.Fatalf("voice onset or pre-roll incorrect: %+v", got)
	}
	for range 50 {
		p.manager.Broadcast(r.UUID, make([]byte, 320))
	}
	before := p.active.snapshot()
	if before.FileSize != 44+3200+320+8000 || !before.AutoPaused {
		t.Fatalf("release margin should be exactly 500ms: %+v", before)
	}
	if rec := recordingRequest(t, p.Change, http.MethodPatch, r.ID, map[string]string{"action": "pause"}); rec.Code != 200 {
		t.Fatal(rec.Body)
	}
	for range 10 {
		p.manager.Broadcast(r.UUID, recordingTone())
	}
	if got := p.active.snapshot(); got.FileSize != before.FileSize || got.State != "paused" {
		t.Fatalf("audio automatically overrode manual pause: %+v", got)
	}
	recordingRequest(t, p.Change, http.MethodPatch, r.ID, map[string]string{"action": "resume"})
	p.manager.Broadcast(r.UUID, recordingTone())
	if got := p.active.snapshot(); got.FileSize != before.FileSize+320 || got.State != "recording" {
		t.Fatalf("manual resume incorrect: %+v", got)
	}
	rec := recordingRequest(t, p.Change, http.MethodPatch, r.ID, map[string]string{"action": "stop"})
	if rec.Code != 200 || p.active.snapshot().State != "completed" {
		t.Fatalf("stop failed: %s", rec.Body)
	}
	data, err := os.ReadFile(filepath.Join(recordingStoragePath(), r.Filename))
	if err != nil {
		t.Fatal(err)
	}
	_, duration, rate, bits, err := normalizeAudioWAV(data)
	if err != nil || duration.Milliseconds() != p.active.snapshot().DurationMS || rate != 8000 || bits != 16 {
		t.Fatalf("saved WAV incorrect: duration=%v rate=%d bits=%d error=%v", duration, rate, bits, err)
	}
}

func TestAudioRecordingPersistsAndCopiesToLibraryWithoutBroadcast(t *testing.T) {
	p := recordingFixture(t)
	r := startTestRecording(t, p)
	p.manager.Broadcast(r.UUID, recordingTone())
	for _, handler := range []echo.HandlerFunc{p.Delete, p.File, p.AddToLibrary} {
		if rec := recordingRequest(t, handler, http.MethodPost, r.ID, nil); rec.Code != 409 {
			t.Fatalf("active recording must not be deleted, served or transferred: %s", rec.Body)
		}
	}
	recordingRequest(t, p.Change, http.MethodPatch, r.ID, map[string]string{"action": "stop"})
	rec := recordingRequest(t, p.AddToLibrary, http.MethodPost, r.ID, nil)
	if rec.Code != 201 {
		t.Fatalf("transfer failed: %s", rec.Body)
	}
	var response struct {
		Data AudioTrack `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.PlaybackMode != "broadcast_once" || response.Data.Name != r.Name {
		t.Fatalf("incorrect library entry: %+v", response.Data)
	}
	var filename string
	if err := config.App.DB.QueryRow(`SELECT stored_filename FROM audio_tracks WHERE id=?`, response.Data.ID).Scan(&filename); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(recordingStoragePath(), r.Filename))
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(audioStoragePath(), filename)
	copied, err := os.ReadFile(copyPath)
	if err != nil || !bytes.Equal(copied, original) {
		t.Fatal("library must have an independent, identical copy", err)
	}
	if rec := recordingRequest(t, p.AddToLibrary, http.MethodPost, r.ID, nil); rec.Code != 409 {
		t.Fatal("duplicate transfer accepted")
	}
	list := recordingRequest(t, p.List, http.MethodGet, 0, nil)
	if !bytes.Contains(list.Body.Bytes(), []byte(`"in_library":true`)) {
		t.Fatalf("library copy status missing: %s", list.Body)
	}
	if rec := recordingRequest(t, p.Delete, http.MethodDelete, r.ID, nil); rec.Code != 204 {
		t.Fatal(rec.Body)
	}
	if _, err := os.Stat(copyPath); err != nil {
		t.Fatal("deleting recording removed library copy", err)
	}
}

func TestAudioRecordingRejectsOverlapsAndReportsStartFailure(t *testing.T) {
	p := recordingFixture(t)
	r := startTestRecording(t, p)
	if rec := recordingRequest(t, p.Start, http.MethodPost, 0, map[string]string{"name": "Second"}); rec.Code != 409 {
		t.Fatal("overlapping recording accepted", rec.Body)
	}
	p.manager.CloseClients(r.UUID)
	if got := p.active.snapshot(); got.State != "interrupted" {
		t.Fatalf("unexpected stream termination not persisted: %+v", got)
	}
	p.originate = func(string, string) error { return errors.New("AMI unavailable") }
	rec := recordingRequest(t, p.Start, http.MethodPost, 0, map[string]string{"name": "Failed"})
	if rec.Code != 502 || p.active.snapshot().State != "failed" {
		t.Fatalf("start failure not explicit: %s", rec.Body)
	}
}

func TestAudioRecordingRecoveryRepairsWAVAndKeepsExistingTracks(t *testing.T) {
	p := recordingFixture(t)
	r := startTestRecording(t, p)
	p.manager.Broadcast(r.UUID, recordingTone())
	// Simulate a crash after PCM was written, before the next header checkpoint.
	if err := p.active.file.Close(); err != nil {
		t.Fatal(err)
	}
	p.active.closed = true
	if err := RecoverAudioRecordings(); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := config.App.DB.QueryRow(`SELECT state FROM audio_recordings WHERE id=?`, r.ID).Scan(&state); err != nil || state != "interrupted" {
		t.Fatalf("recovery state incorrect: %s %v", state, err)
	}
	data, err := os.ReadFile(filepath.Join(recordingStoragePath(), r.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := normalizeAudioWAV(data); err != nil {
		t.Fatal("recovered PCM is not playable WAV", err)
	}
}

func TestAudioRecordingEmptyAudioCannotBeAddedToLibrary(t *testing.T) {
	p := recordingFixture(t)
	r := startTestRecording(t, p)
	recordingRequest(t, p.Change, http.MethodPatch, r.ID, map[string]string{"action": "stop"})
	if rec := recordingRequest(t, p.AddToLibrary, http.MethodPost, r.ID, nil); rec.Code != 400 {
		t.Fatal("empty WAV accepted into library", rec.Body)
	}

}

func TestAudioRecordingSizeLimitStopsAndKeepsPlayableWAV(t *testing.T) {
	p := recordingFixture(t)
	r := startTestRecording(t, p)
	size := int64(audioMaxUploadBytes - 100)
	s := p.active
	if err := s.file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	s.record.FileSize = size
	s.record.DurationMS = (size - 44) / 16
	p.manager.Broadcast(r.UUID, recordingTone())
	got := s.snapshot()
	if got.State != "completed" || got.ErrorCode != "AUDIO_RECORD_LIMIT" {
		t.Fatalf("size limit did not finalize: %+v", got)
	}
	data, err := os.ReadFile(filepath.Join(recordingStoragePath(), r.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > audioMaxUploadBytes {
		t.Fatal("recording exceeded maximum size")
	}
	if _, _, _, _, err := normalizeAudioWAV(data); err != nil {
		t.Fatal("size-limited WAV is not playable", err)
	}
}
