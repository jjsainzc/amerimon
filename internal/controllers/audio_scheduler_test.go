package controllers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"amerimon/internal/config"
	"amerimon/internal/db"
)

func TestAudioSilenceIncludesSIPAndRejectsUnknownStatus(t *testing.T) {
	for _, test := range []struct {
		name, status  string
		busy, invalid bool
	}{
		{"silent", "Response: Success\r\nConn: 448513 0 10 5\r\nConn: SIP/1001 0 0 -1\r\n", false, false},
		{"SIP talking", "Response: Success\r\nConn: SIP/1001 1 0 -1\r\n", true, false},
		{"RPT talking", "Response: Success\r\nConn: 448513 1 0 5\r\n", true, false},
		{"no links", "Response: Success\r\n", false, false},
		{"missing response", "", false, true},
		{"AMI error", "Response: Error\r\n", false, true},
		{"bad RX", "Response: Success\r\nConn: SIP/1001 unknown 0 -1\r\n", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			busy, err := audioReceivingLinksBusy(test.status, "616262")
			if (err != nil) != test.invalid || (err == nil && busy != test.busy) {
				t.Fatalf("busy=%v error=%v", busy, err)
			}
		})
	}
}

func TestAudioNodeSilenceUsesFreshSIPAndCOSStatus(t *testing.T) {
	setupReceiveActivityTest(t)
	var cos, rx atomic.Bool
	host, port := mockConnectionAMI(t, func(fields map[string]string) string {
		switch fields["Command"] {
		case "NodeStat":
			value := "NO"
			if cos.Load() {
				value = "YES"
			}
			return "Response: Success\r\nSignalOnInput: " + value + "\r\n\r\n"
		case "SawStat":
			value := "0"
			if rx.Load() {
				value = "1"
			}
			return "Response: Success\r\nConn: SIP/1001 " + value + " 0 -1\r\n\r\n"
		}
		return "Response: Error\r\n\r\n"
	})
	instances := []allmonInstance{{Node: "616262", Host: host, Port: port, User: "test", Secret: "test"}}
	silent, err := audioNodesSilent(instances)
	if err != nil || !silent {
		t.Fatalf("silent node rejected: %v %v", silent, err)
	}
	rx.Store(true)
	silent, err = audioNodesSilent(instances)
	if err != nil || silent {
		t.Fatalf("SIP talking treated as silence: %v %v", silent, err)
	}
	rx.Store(false)
	cos.Store(true)
	silent, err = audioNodesSilent(instances)
	if err != nil || silent {
		t.Fatalf("COS treated as silence: %v %v", silent, err)
	}

	if silent, _ := audioNodesSilent(nil); silent {
		t.Fatal("no monitored nodes treated as silence")
	}
}

func TestAudioPrimaryResolvesWithoutAllmon(t *testing.T) {
	previous := config.App
	defer func() { config.App = previous }()
	dir := t.TempDir()
	config.App.Sources.AllmonINIPath = filepath.Join(dir, "missing-allmon.ini")
	config.App.Sources.PrivateNodesPath = filepath.Join(dir, "missing-private.txt")
	config.App.AMI.RPTConfigPath = filepath.Join(dir, "rpt.conf")
	config.App.AMI.Host = "127.0.0.1"
	config.App.AMI.Port = "5038"
	config.App.AMI.User = "test"
	config.App.AMI.Secret = "test"
	if err := os.WriteFile(config.App.AMI.RPTConfigPath, []byte("[41777]\nduplex=1\n[nodes]\n41777=radio@127.0.0.1/41777,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	instance, err := resolveAudioPrimaryInstance("41777")
	if err != nil || instance.Node != "41777" || instance.Host != "127.0.0.1" || instance.Secret != "test" {
		t.Fatalf("local primary requires Allmon: %+v %v", instance, err)
	}
	if _, err := resolveAudioPrimaryInstance("99999"); err == nil {
		t.Fatal("unconfigured node silently resolved")
	}
	config.App.AMI.Secret = ""
	if _, err := resolveAudioPrimaryInstance("41777"); err == nil {
		t.Fatal("incomplete AMI silently accepted")
	}
}

func TestAudioCycleRechecksActivityAndSendsOnlyOnce(t *testing.T) {
	setupReceiveActivityTest(t)
	dir := t.TempDir()
	config.App.Sources.AllmonINIPath = filepath.Join(dir, "missing-allmon.ini")
	config.App.Sources.PrivateNodesPath = filepath.Join(dir, "missing-private.txt")
	config.App.AMI.RPTConfigPath = filepath.Join(dir, "rpt.conf")
	config.App.Audio.StorageDir = dir
	if err := os.WriteFile(config.App.AMI.RPTConfigPath, []byte("[41777]\nduplex=1\n[nodes]\n41777=radio@127.0.0.1/41777,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var reads, sends atomic.Int32
	var interrupt atomic.Bool
	host, port := mockConnectionAMI(t, func(fields map[string]string) string {
		switch fields["Command"] {
		case "NodeStat":
			return "Response: Success\r\nSignalOnInput: NO\r\n\r\n"
		case "SawStat":
			rx := "0"
			if reads.Add(1) >= 3 && interrupt.Load() {
				rx = "1"
			}
			return "Response: Success\r\nConn: SIP/1001 " + rx + " 0 -1\r\n\r\n"
		default:
			if strings.HasPrefix(fields["Command"], "rpt playback 41777 ") {
				sends.Add(1)
				return "Response: Follows\r\nOutput: Playback started\r\nOutput: --END COMMAND--\r\n\r\n"
			}
		}
		return "Response: Error\r\n\r\n"
	})
	config.App.AMI.Host, config.App.AMI.Port = host, port
	config.App.AMI.User, config.App.AMI.Secret = "test", "test"
	filename := "01234567-89ab-cdef-0123-456789abcdef.wav"
	if err := os.MkdirAll(audioStoragePath(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(audioStoragePath(), filename), testPCMMonoWAV(8000, 16, 1), 0o640); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	due := now.Add(-time.Minute).Format(time.RFC3339Nano)
	if _, err := config.App.DB.Exec(`
		INSERT INTO audio_tracks (id, name, stored_filename, original_filename, playback_mode, enabled, file_size, duration_ms, sample_rate, bits_per_sample)
		VALUES (1, 'Audio', ?, 'test.wav', 'broadcast_silence', 1, 52, 1, 8000, 16)`, filename); err != nil {
		t.Fatal(err)
	}
	if _, err := config.App.DB.Exec(`INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at) VALUES (1, 1, ?)`, due); err != nil {
		t.Fatal(err)
	}
	audioSilentSince = time.Time{}
	t.Cleanup(func() { audioSilentSince = time.Time{} })
	runAudioPlaybackCycle(now)
	if sends.Load() != 0 {
		t.Fatal("broadcast without quiet-period confirmation")
	}
	audioSilentSince = time.Now().Add(-audioQuietPeriod)
	interrupt.Store(true)
	runAudioPlaybackCycle(now)
	if sends.Load() != 0 {
		t.Fatal("broadcast despite SIP starting between checks")
	}
	var next string
	if err := config.App.DB.QueryRow(`SELECT next_play_at FROM audio_broadcast_schedules WHERE audio_id = 1`).Scan(&next); err != nil || next != due {
		t.Fatalf("busy recheck lost due schedule: %s %v", next, err)
	}
	interrupt.Store(false)
	audioSilentSince = time.Now().Add(-audioQuietPeriod)
	runAudioPlaybackCycle(now)
	if sends.Load() != 1 {
		t.Fatalf("silent due broadcast count=%d", sends.Load())
	}
	if _, err := config.App.DB.Exec(`UPDATE audio_playback_state SET busy_until='1970-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	audioSilentSince = time.Now().Add(-audioQuietPeriod)
	runAudioPlaybackCycle(now.Add(10 * time.Second))
	if sends.Load() != 1 {
		t.Fatalf("duplicate broadcast before next interval: %d", sends.Load())
	}
}

func TestScheduledAudioClaimPreventsDuplicateAndRollsBackOnActivity(t *testing.T) {
	previous := config.App
	defer func() { config.App = previous }()
	database, err := db.Open(filepath.Join(t.TempDir(), "audio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config.App.DB = database
	now := time.Now().UTC()
	due := now.Add(-time.Minute)
	if _, err := database.Exec(`
		INSERT INTO audio_tracks (id, name, stored_filename, original_filename, playback_mode, enabled, file_size, duration_ms, sample_rate, bits_per_sample)
		VALUES (1, 'Audio', 'test.wav', 'test.wav', 'broadcast_silence', 1, 16044, 1000, 8000, 16);
		INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at) VALUES (1, 1, ?)`,
		due.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	track := storedAudioTrack{AudioTrack: AudioTrack{ID: 1, DurationMS: 1000, IntervalMinutes: 1, NextPlayAt: &due}}
	token, err := claimScheduledAudioPlayback(context.Background(), track, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimScheduledAudioPlayback(context.Background(), track, now); !errors.Is(err, errAudioPlaybackBusy) {
		t.Fatalf("duplicate accepted: %v", err)
	}
	if err := releaseAudioPlayback(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := claimScheduledAudioPlayback(context.Background(), track, now); !errors.Is(err, errAudioPlaybackBusy) {
		t.Fatalf("stale cycle replays after playback reservation released: %v", err)
	}
	if _, err := database.Exec(`UPDATE audio_broadcast_schedules SET next_play_at = ?`, due.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	token, err = claimScheduledAudioPlayback(context.Background(), track, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := cancelScheduledAudioPlayback(context.Background(), track, token, now); err != nil {
		t.Fatal(err)
	}
	var next string
	if err := database.QueryRow(`SELECT next_play_at FROM audio_broadcast_schedules WHERE audio_id = 1`).Scan(&next); err != nil || next != due.Format(time.RFC3339Nano) {
		t.Fatalf("activity should keep original due time: %s %v", next, err)
	}
	token, err = claimScheduledAudioPlayback(context.Background(), track, now)
	if err != nil {
		t.Fatalf("cancelled claim not released: %v", err)
	}
	changed := now.Add(10 * time.Minute).Format(time.RFC3339Nano)
	if _, err := database.Exec(`UPDATE audio_broadcast_schedules SET interval_minutes = 10, next_play_at = ?`, changed); err != nil {
		t.Fatal(err)
	}
	if err := updateAudioScheduleAfterAttempt(context.Background(), 1, now, 1, true); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT next_play_at FROM audio_broadcast_schedules WHERE audio_id = 1`).Scan(&next); err != nil || next != changed {
		t.Fatalf("concurrent interval change overwritten: %s %v", next, err)
	}
}
