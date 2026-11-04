package controllers

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"mime/multipart"
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

func testPCMMonoWAV(sampleRate, bits, channels int) []byte {
	data := []byte{128, 129, 127, 128}
	if bits == 16 {
		data = []byte{0, 0, 0, 1, 0, 255, 0, 0}
	}
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+len(data)))
	wav.WriteString("WAVEfmt ")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(16))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(1))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&wav, binary.LittleEndian, uint32(sampleRate*channels*bits/8))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(channels*bits/8))
	_ = binary.Write(&wav, binary.LittleEndian, uint16(bits))
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(len(data)))
	wav.Write(data)
	return wav.Bytes()
}

func TestNormalizeAudioWAV(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate int
		bits       int
		channels   int
		wantErr    bool
	}{
		{name: "8-bit is converted for Asterisk", sampleRate: 8000, bits: 8, channels: 1},
		{name: "16-bit is supported", sampleRate: 8000, bits: 16, channels: 1},
		{name: "wrong sample rate", sampleRate: 16000, bits: 8, channels: 1, wantErr: true},
		{name: "wrong bit depth", sampleRate: 8000, bits: 24, channels: 1, wantErr: true},
		{name: "stereo", sampleRate: 8000, bits: 8, channels: 2, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, duration, sampleRate, bits, err := normalizeAudioWAV(testPCMMonoWAV(test.sampleRate, test.bits, test.channels))
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizeAudioWAV() error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && (sampleRate != 8000 || bits != 16 || duration != time.Millisecond) {
				t.Fatalf("unexpected normalized audio metadata: rate=%d bits=%d duration=%s", sampleRate, bits, duration)
			}
			if err == nil {
				_, _, normalizedRate, normalizedBits, parseErr := normalizeAudioWAV(normalized)
				if parseErr != nil || normalizedRate != 8000 || normalizedBits != 16 {
					t.Fatalf("normalized WAV is not Asterisk-compatible: rate=%d bits=%d err=%v", normalizedRate, normalizedBits, parseErr)
				}
			}
		})
	}
	if _, _, _, _, err := normalizeAudioWAV([]byte("not a wav")); err == nil {
		t.Fatal("expected invalid WAV error")
	}
}

func TestAudioStoragePathUsesAsteriskDataDirectory(t *testing.T) {
	previous := config.App
	defer func() { config.App = previous }()

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	varLibDir := filepath.Join(root, "varlib")
	if err := os.WriteFile(filepath.Join(root, "asterisk.conf"), []byte(
		"astdatadir => "+dataDir+"\nastvarlibdir => "+varLibDir+"\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	config.App.AMI.RPTConfigPath = filepath.Join(root, "rpt.conf")
	config.App.Audio.StorageDir = "/var/lib/asterisk/sounds"

	want := filepath.Join(dataDir, "sounds", audioSubdirectory)
	if got := audioStoragePath(); got != want {
		t.Fatalf("audioStoragePath() = %q, want %q", got, want)
	}
	if got := legacyAudioStoragePath(); got != filepath.Join(varLibDir, "sounds", audioSubdirectory) {
		t.Fatalf("legacyAudioStoragePath() = %q", got)
	}
}

func TestAudioStoragePathHonorsCustomDirectory(t *testing.T) {
	previous := config.App
	defer func() { config.App = previous }()

	config.App.Audio.StorageDir = filepath.Join(t.TempDir(), "sounds")
	want := filepath.Join(config.App.Audio.StorageDir, audioSubdirectory)
	if got := audioStoragePath(); got != want {
		t.Fatalf("audioStoragePath() = %q, want %q", got, want)
	}
}

func TestAudioPlaybackOutputError(t *testing.T) {
	if err := audioPlaybackOutputError("Unable to open amerimon/missing-file"); err == nil {
		t.Fatal("expected app_rpt file lookup failure to be reported")
	}
	if err := audioPlaybackOutputError("Playback started"); err != nil {
		t.Fatalf("unexpected playback error: %v", err)
	}
}

func TestReadRPTSectionSettingResolvesNodeTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rpt.conf")
	content := `[node-default](!)
duplex = 0
[616262](node-default)
duplex = 1
[616263](node-default)
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		node      string
		wantValue string
		wantFound bool
	}{
		{node: "616262", wantValue: "1", wantFound: true},
		{node: "616263", wantValue: "0", wantFound: true},
		{node: "616264", wantFound: false},
	} {
		value, found, err := readRPTSectionSetting(path, test.node, "duplex")
		if err != nil || found != test.wantFound || value != test.wantValue {
			t.Errorf("readRPTSectionSetting(%q) = %q, %t, %v; want %q, %t", test.node, value, found, err, test.wantValue, test.wantFound)
		}
	}
}

func TestAudioDuplexAllowsLinkedPlaybackForNonzeroModes(t *testing.T) {
	for _, test := range []struct {
		value string
		found bool
		want  bool
	}{
		{value: "0", found: true, want: false},
		{value: "1", found: true, want: true},
		{value: "2", found: true, want: true},
		{value: "3", found: true, want: true},
		{value: "4", found: true, want: true},
		{value: "5", found: true, want: false},
		{value: "invalid", found: true, want: false},
		{value: "1", found: false, want: false},
	} {
		if got := audioDuplexAllowsLinkedPlayback(test.value, test.found); got != test.want {
			t.Errorf("audioDuplexAllowsLinkedPlayback(%q, %t) = %t; want %t", test.value, test.found, got, test.want)
		}
	}
}

func TestUploadAndPersistAudioTrack(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previous := config.App
	config.App.DB = database
	config.App.Audio.StorageDir = t.TempDir()
	defer func() { config.App = previous }()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("name", "Anuncio")
	_ = writer.WriteField("playback_mode", "broadcast_silence")
	_ = writer.WriteField("interval_minutes", "15")
	part, err := writer.CreateFormFile("file", "announcement.wav")
	if err != nil {
		t.Fatal(err)
	}
	largeWAV := testPCMMonoWAV(8000, 8, 1)
	largeWAV = append(largeWAV[:44], make([]byte, 1<<20)...)
	binary.LittleEndian.PutUint32(largeWAV[4:8], uint32(len(largeWAV)-8))
	binary.LittleEndian.PutUint32(largeWAV[40:44], uint32(len(largeWAV)-44))
	_, _ = part.Write(largeWAV)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/mon/audio/tracks", &body)
	request.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	if err := UploadAudioTrack(echo.New().NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
	var response struct {
		Data AudioTrack `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.ID == 0 || response.Data.PlaybackMode != "broadcast_silence" ||
		response.Data.IntervalMinutes != 15 || response.Data.DurationMS != 131072 || response.Data.BitsPerSample != 16 {
		t.Fatalf("unexpected uploaded track: %+v", response.Data)
	}
	var filename string
	if err := database.QueryRow(`SELECT stored_filename FROM audio_tracks WHERE id = ?`, response.Data.ID).Scan(&filename); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(audioStoragePath(), filename)); err != nil {
		t.Fatalf("uploaded file missing: %v", err)
	}
	storedBytes, err := os.ReadFile(filepath.Join(audioStoragePath(), filename))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, storedBits, err := normalizeAudioWAV(storedBytes)
	if err != nil || storedBits != 16 {
		t.Fatalf("uploaded file was not normalized for Asterisk: bits=%d err=%v", storedBits, err)
	}
	var interval int
	var nextPlay string
	if err := database.QueryRow(`SELECT interval_minutes, next_play_at FROM audio_broadcast_schedules WHERE audio_id = ?`, response.Data.ID).Scan(&interval, &nextPlay); err != nil {
		t.Fatal(err)
	}
	if interval != 15 || nextPlay == "" {
		t.Fatalf("schedule was not persisted: interval=%d next=%q", interval, nextPlay)
	}
	tracks, err := loadScheduledAudioTracks(context.Background())
	if err != nil || len(tracks) != 1 || tracks[0].ID != response.Data.ID || !tracks[0].Enabled {
		t.Fatalf("persisted track not available to the background scheduler: tracks=%+v err=%v", tracks, err)
	}
}

func TestPrepareLegacyAudioForAsterisk(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previous := config.App
	root := t.TempDir()
	config.App.DB = database
	config.App.Audio.StorageDir = root
	defer func() { config.App = previous }()

	if err := os.MkdirAll(audioStoragePath(), 0o750); err != nil {
		t.Fatal(err)
	}
	filename := "01234567-89ab-cdef-0123-456789abcdef.wav"
	legacyWAV := testPCMMonoWAV(8000, 8, 1)
	if err := os.WriteFile(filepath.Join(audioStoragePath(), filename), legacyWAV, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO audio_tracks
		(name, stored_filename, original_filename, playback_mode, enabled, file_size, duration_ms, sample_rate, bits_per_sample)
		VALUES ('Existing', ?, 'existing.wav', 'broadcast_once', 1, ?, 1, 8000, 8)`, filename, len(legacyWAV)); err != nil {
		t.Fatal(err)
	}
	if err := prepareAudioFileForPlayback(filename); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filepath.Join(audioStoragePath(), filename))
	if err != nil {
		t.Fatal(err)
	}
	_, _, sampleRate, bits, err := normalizeAudioWAV(stored)
	if err != nil || sampleRate != 8000 || bits != 16 {
		t.Fatalf("legacy file was not converted for Asterisk: rate=%d bits=%d err=%v", sampleRate, bits, err)
	}
	var storedBits int
	if err := database.QueryRow(`SELECT bits_per_sample FROM audio_tracks WHERE stored_filename = ?`, filename).Scan(&storedBits); err != nil {
		t.Fatal(err)
	}
	if storedBits != 16 {
		t.Fatalf("legacy metadata was not updated: bits=%d", storedBits)
	}
}

func TestPrepareAudioCopiesLegacyLocationIntoAsteriskDataDirectory(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previous := config.App
	defer func() { config.App = previous }()

	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	varLibDir := filepath.Join(root, "varlib")
	if err := os.WriteFile(filepath.Join(root, "asterisk.conf"), []byte(
		"astdatadir => "+dataDir+"\nastvarlibdir => "+varLibDir+"\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	config.App.DB = database
	config.App.AMI.RPTConfigPath = filepath.Join(root, "rpt.conf")
	config.App.Audio.StorageDir = "/var/lib/asterisk/sounds"

	filename := "01234567-89ab-cdef-0123-456789abcdef.wav"
	legacyPath := filepath.Join(legacyAudioStoragePath(), filename)
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o750); err != nil {
		t.Fatal(err)
	}
	legacyWAV := testPCMMonoWAV(8000, 8, 1)
	if err := os.WriteFile(legacyPath, legacyWAV, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO audio_tracks
		(name, stored_filename, original_filename, playback_mode, enabled, file_size, duration_ms, sample_rate, bits_per_sample)
		VALUES ('Existing', ?, 'existing.wav', 'broadcast_once', 1, ?, 1, 8000, 8)`, filename, len(legacyWAV)); err != nil {
		t.Fatal(err)
	}

	if err := prepareAudioFileForPlayback(filename); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(audioStoragePath(), filename)
	stored, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("legacy audio was not copied to Asterisk's data sound directory: %v", err)
	}
	_, _, sampleRate, bits, err := normalizeAudioWAV(stored)
	if err != nil || sampleRate != 8000 || bits != 16 {
		t.Fatalf("copied audio was not normalized: rate=%d bits=%d err=%v", sampleRate, bits, err)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy source should be preserved: %v", err)
	}
}

func TestAudioScheduleAdvancesOnlySuccessfulPlaybackTimestamp(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previousDB := config.App.DB
	config.App.DB = database
	defer func() { config.App.DB = previousDB }()

	result, err := database.Exec(`
		INSERT INTO audio_tracks (name, stored_filename, original_filename, playback_mode, file_size, sample_rate, bits_per_sample, duration_ms)
		VALUES ('test', '01234567-89ab-cdef-0123-456789abcdef.wav', 'test.wav', 'broadcast_silence', 48, 8000, 8, 1)`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := database.Exec(`INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at) VALUES (?, 10, ?)`,
		id, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := updateAudioScheduleAfterAttempt(context.Background(), id, now, 10, false); err != nil {
		t.Fatal(err)
	}
	var lastPlayed, nextPlay string
	if err := database.QueryRow(`SELECT COALESCE(last_played_at, ''), next_play_at FROM audio_broadcast_schedules WHERE audio_id = ?`, id).
		Scan(&lastPlayed, &nextPlay); err != nil {
		t.Fatal(err)
	}
	if lastPlayed != "" || nextPlay != now.Add(10*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("failed playback should postpone retry without recording success: last=%q next=%q", lastPlayed, nextPlay)
	}
	if err := updateAudioScheduleAfterAttempt(context.Background(), id, now, 10, true); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT last_played_at, next_play_at FROM audio_broadcast_schedules WHERE audio_id = ?`, id).
		Scan(&lastPlayed, &nextPlay); err != nil {
		t.Fatal(err)
	}
	if lastPlayed != now.Format(time.RFC3339Nano) || nextPlay != now.Add(10*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("successful playback timestamps not persisted: last=%q next=%q", lastPlayed, nextPlay)
	}
}

func TestUpdateAudioTrackPersistsPlaybackModeAndInterval(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previous := config.App
	config.App.DB = database
	defer func() { config.App = previous }()

	result, err := database.Exec(`
		INSERT INTO audio_tracks (name, stored_filename, original_filename, playback_mode, enabled, file_size, sample_rate, bits_per_sample, duration_ms)
		VALUES ('test', '01234567-89ab-cdef-0123-456789abcdef.wav', 'test.wav', 'broadcast_once', 1, 48, 8000, 16, 1000)`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	update := func(payload string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString(payload))
		request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		recorder := httptest.NewRecorder()
		context := echo.New().NewContext(request, recorder)
		context.SetParamNames("id")
		context.SetParamValues(strconv.FormatInt(id, 10))
		if err := UpdateAudioTrack(context); err != nil {
			t.Fatalf("UpdateAudioTrack() error = %v", err)
		}
		return recorder
	}

	recorder := update(`{"playback_mode":"broadcast_silence","interval_minutes":2}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("enable scheduled playback status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var interval int
	var nextPlay string
	if err := database.QueryRow(`SELECT interval_minutes, next_play_at FROM audio_broadcast_schedules WHERE audio_id = ?`, id).
		Scan(&interval, &nextPlay); err != nil {
		t.Fatal(err)
	}
	if interval != 2 {
		t.Fatalf("saved interval = %d, want 2", interval)
	}
	firstNext, err := time.Parse(time.RFC3339Nano, nextPlay)
	if err != nil || time.Until(firstNext) < time.Minute || time.Until(firstNext) > 2*time.Minute {
		t.Fatalf("initial scheduled playback is not based on saved interval: next=%q err=%v", nextPlay, err)
	}

	recorder = update(`{"playback_mode":"broadcast_silence","interval_minutes":7}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("change scheduled interval status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if err := database.QueryRow(`SELECT interval_minutes, next_play_at FROM audio_broadcast_schedules WHERE audio_id = ?`, id).
		Scan(&interval, &nextPlay); err != nil {
		t.Fatal(err)
	}
	if interval != 7 {
		t.Fatalf("updated interval = %d, want 7", interval)
	}
	updatedNext, err := time.Parse(time.RFC3339Nano, nextPlay)
	if err != nil || time.Until(updatedNext) < 6*time.Minute || time.Until(updatedNext) > 7*time.Minute {
		t.Fatalf("next playback was not reset using updated interval: next=%q err=%v", nextPlay, err)
	}

	recorder = update(`{"playback_mode":"broadcast_once"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("disable scheduled playback status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var count int
	if err := database.QueryRow(`SELECT count(*) FROM audio_broadcast_schedules WHERE audio_id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("one-time playback must remove its interval schedule, found %d rows", count)
	}
}

func TestWelcomePlaybackModeIsNoLongerSupported(t *testing.T) {
	if validAudioPlaybackMode("on_connection") {
		t.Fatal("welcome playback mode should no longer be accepted")
	}
}

func TestAudioPlaybackClaimsPreventOverlapAndCanBeReleased(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previousDB := config.App.DB
	config.App.DB = database
	defer func() { config.App.DB = previousDB }()

	first, err := claimAudioPlayback(context.Background(), 5000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimAudioPlayback(context.Background(), 1000); !errors.Is(err, errAudioPlaybackBusy) {
		t.Fatalf("expected overlapping playback to be rejected, got %v", err)
	}
	if err := releaseAudioPlayback(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := claimAudioPlayback(context.Background(), 1000); err != nil {
		t.Fatalf("playback should be available after release: %v", err)
	}
}
