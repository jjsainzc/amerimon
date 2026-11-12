package controllers

// Este archivo contiene pruebas para la programación y coordinación de emisiones de audio.

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"amerimon/internal/config"
	"amerimon/internal/db"
)

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

type fakeScheduledSilenceMonitor struct {
	waitSilent, finalSilent bool
	waitErr                 error
	closed                  bool
}

func (m *fakeScheduledSilenceMonitor) wait() (bool, error)  { return m.waitSilent, m.waitErr }
func (m *fakeScheduledSilenceMonitor) check() (bool, error) { return m.finalSilent, nil }
func (m *fakeScheduledSilenceMonitor) close() error         { m.closed = true; return nil }

func TestAudioSilenceMonitorRequiresContinuousPCM(t *testing.T) {
	start := time.Now()
	monitor := &audioSilenceMonitor{}
	frame := make([]byte, 320)
	for i := 0; i < 250; i++ {
		if _, err := monitor.writeAt(frame, start.Add(time.Duration(i)*20*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	if quiet, _, err := monitor.state(start.Add(4999 * time.Millisecond)); err != nil || quiet {
		t.Fatalf("less than five seconds treated as silence: %v %v", quiet, err)
	}
	if _, err := monitor.writeAt(frame, start.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if quiet, _, err := monitor.state(start.Add(5 * time.Second)); err != nil || !quiet {
		t.Fatalf("five seconds of PCM silence rejected: %v %v", quiet, err)
	}
	if _, _, err := monitor.state(start.Add(8 * time.Second)); err == nil {
		t.Fatal("stale PCM treated as silence")
	}
}

func TestAudioSilenceMonitorRejectsVoiceAndBrokenStream(t *testing.T) {
	start := time.Now()
	monitor := &audioSilenceMonitor{}
	if _, err := monitor.writeAt(make([]byte, 320), start); err != nil {
		t.Fatal(err)
	}
	if _, err := monitor.writeAt(recordingTone(), start.Add(20*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if quiet, busy, err := monitor.state(start.Add(20 * time.Millisecond)); err != nil || quiet || !busy {
		t.Fatalf("voice in node mix treated as silence: quiet=%v busy=%v err=%v", quiet, busy, err)
	}
	broken := &audioSilenceMonitor{}
	if _, err := broken.writeAt(make([]byte, 320), start); err != nil {
		t.Fatal(err)
	}
	if _, err := broken.writeAt(make([]byte, 320), start.Add(3*time.Second)); err == nil {
		t.Fatal("gap in PCM was accepted")
	}
	if _, _, err := broken.state(start.Add(3 * time.Second)); err == nil {
		t.Fatal("broken stream treated as silence")
	}
}

func TestAudioSilenceMonitorReceivesEAGIPCM(t *testing.T) {
	manager := config.NewAudioStreamManager()
	uuid := "a1234567-1234-1234-1234-123456789abc"
	monitor := &audioSilenceMonitor{}
	manager.AddConsumer(uuid, monitor)
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		config.HandleEAGIConnection(server, manager)
		close(done)
	}()
	defer func() {
		client.Close()
		<-done
	}()
	go func() {
		if _, err := client.Write([]byte(uuid)); err != nil {
			return
		}
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := client.Write(make([]byte, 320)); err != nil {
				return
			}
		}
	}()
	quiet, err := monitor.wait()
	stillQuiet, checkErr := monitor.check()
	if err != nil || !quiet || checkErr != nil || !stillQuiet {
		t.Fatalf("continuous EAGI silence not confirmed: quiet=%v err=%v checkErr=%v", quiet, err, checkErr)
	}
}

func TestAudioCycleRechecksActivityAndSendsOnlyOnce(t *testing.T) {
	setupReceiveActivityTest(t)
	dir := t.TempDir()
	config.App.Sources.AllmonINIPath = filepath.Join(dir, "allmon.ini")
	config.App.Sources.PrivateNodesPath = filepath.Join(dir, "missing-private.txt")
	config.App.AMI.RPTConfigPath = filepath.Join(dir, "rpt.conf")
	config.App.Audio.StorageDir = dir
	if err := os.WriteFile(config.App.AMI.RPTConfigPath, []byte("[41777]\nduplex=1\n[nodes]\n41777=radio@127.0.0.1/41777,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var sends atomic.Int32
	var otherServerReads atomic.Int32
	host, port := mockConnectionAMI(t, func(fields map[string]string) string {
		if strings.HasPrefix(fields["Command"], "rpt playback 41777 ") {
			sends.Add(1)
			return "Response: Follows\r\nOutput: Playback started\r\nOutput: --END COMMAND--\r\n\r\n"
		}
		return "Response: Error\r\n\r\n"
	})
	otherHost, otherPort := mockConnectionAMI(t, func(fields map[string]string) string {
		otherServerReads.Add(1)
		switch fields["Command"] {
		case "NodeStat":
			return "Response: Success\r\nSignalOnInput: NO\r\n\r\n"
		case "SawStat":
			return "Response: Success\r\nConn: SIP/2001 1 0 -1\r\n\r\n"
		}
		return "Response: Error\r\n\r\n"
	})
	allmon := "[99999]\nsystem=Nodes\nhost=" + otherHost + "\nport=" + otherPort + "\nuser=test\npasswd=test\n"
	if err := os.WriteFile(config.App.Sources.AllmonINIPath, []byte(allmon), 0o600); err != nil {
		t.Fatal(err)
	}
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
	monitor := &fakeScheduledSilenceMonitor{waitErr: errors.New("PCM disconnected")}
	open := func(node string) (scheduledSilenceMonitor, error) {
		if node != "41777" {
			t.Fatalf("monitored wrong node: %s", node)
		}
		return monitor, nil
	}
	runAudioPlaybackCycle(now, open)
	var next string
	if err := config.App.DB.QueryRow(`SELECT next_play_at FROM audio_broadcast_schedules WHERE audio_id = 1`).Scan(&next); err != nil || next != due {
		t.Fatalf("failed capture changed schedule: %s %v", next, err)
	}
	if sends.Load() != 0 || !monitor.closed {
		t.Fatal("failed capture allowed playback or leaked monitor")
	}
	for _, m := range []*fakeScheduledSilenceMonitor{
		{waitSilent: false},
		{waitSilent: true, finalSilent: false},
	} {
		monitor = m
		runAudioPlaybackCycle(now, open)
		if err := config.App.DB.QueryRow(`SELECT next_play_at FROM audio_broadcast_schedules WHERE audio_id = 1`).Scan(&next); err != nil || next != now.Add(time.Minute).Format(time.RFC3339Nano) {
			t.Fatalf("activity did not defer until next interval: %s %v", next, err)
		}
		if sends.Load() != 0 || !monitor.closed {
			t.Fatal("activity allowed playback or leaked monitor")
		}
		if _, err := config.App.DB.Exec(`UPDATE audio_broadcast_schedules SET next_play_at = ?`, due); err != nil {
			t.Fatal(err)
		}
	}
	monitor = &fakeScheduledSilenceMonitor{waitSilent: true, finalSilent: true}
	runAudioPlaybackCycle(now, open)
	if sends.Load() != 1 {
		t.Fatalf("silent due broadcast count=%d", sends.Load())
	}
	if otherServerReads.Load() != 0 {
		t.Fatalf("queried unrelated Allmon server %d times", otherServerReads.Load())
	}
	if _, err := config.App.DB.Exec(`UPDATE audio_playback_state SET busy_until='1970-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	runAudioPlaybackCycle(now.Add(10*time.Second), open)
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
