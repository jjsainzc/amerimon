package controllers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"amerimon/internal/config"
)

var (
	audioSchedulerOnce sync.Once
)

type audioNodeState struct {
	instance allmonInstance
	busy     bool
}

func StartAudioPlaybackScheduler() {
	if config.App.DB == nil {
		log.Printf("audio: programador deshabilitado porque no hay base de datos")
		return
	}
	audioSchedulerOnce.Do(func() {
		go func() {
			interval := 5 * time.Second
			if configured := strings.TrimSpace(config.App.Audio.PollInterval); configured != "" {
				parsed, err := time.ParseDuration(configured)
				if err != nil || parsed < time.Second || parsed > time.Minute {
					log.Printf("audio: intervalo de sondeo %q inválido; se usa %s", configured, interval)
				} else {
					interval = parsed
				}
			}
			runAudioPlaybackCycle(time.Now().UTC())
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				runAudioPlaybackCycle(time.Now().UTC())
			}
		}()
	})
}

func runAudioPlaybackCycle(now time.Time) {
	if config.App.DB == nil {
		return
	}
	tracks, err := loadScheduledAudioTracks(context.Background())
	if err != nil {
		log.Printf("audio: no se pudo consultar la programación: %v", err)
		return
	}
	if len(tracks) == 0 {
		return
	}
	instances, err := allStarLinkActivityInstances()
	if err != nil {
		log.Printf("audio: no se pudieron consultar los nodos para el programador: %v", err)
		return
	}
	if len(instances) == 0 {
		log.Printf("audio: no hay nodos configurados para evaluar las reproducciones")
		return
	}
	states := make([]audioNodeState, 0, len(instances))
	allSilent := true
	for _, instance := range instances {
		state, err := readAudioNodeState(instance)
		if err != nil {
			log.Printf("audio: no se pudo comprobar silencio en nodo local %s: %v", instance.Node, err)
			return
		}
		states = append(states, state)
		if state.busy {
			allSilent = false
		}
	}
	if !allSilent {
		return
	}
	for _, track := range tracks {
		if !track.Enabled || track.PlaybackMode != "broadcast_silence" || track.NextPlayAt == nil || track.NextPlayAt.After(now) {
			continue
		}
		token, claimErr := claimAudioPlayback(context.Background(), track.DurationMS)
		if errors.Is(claimErr, errAudioPlaybackBusy) {
			continue
		}
		if claimErr != nil {
			log.Printf("audio: no se pudo reservar reproducción programada para pista %d: %v", track.ID, claimErr)
			continue
		}
		command, localNode, amiOutput, playErr := playAudioOnPrimary(track.StoredFilename)
		result, details := "success", fmt.Sprintf("Audio %q enviado durante silencio total", track.Name)
		if playErr != nil {
			if releaseErr := releaseAudioPlayback(context.Background(), token); releaseErr != nil {
				log.Printf("audio: no se pudo liberar reproducción fallida para pista %d: %v", track.ID, releaseErr)
			}
			result, details = "ami_error", playErr.Error()
			log.Printf("audio: emisión programada falló para pista %d: %v", track.ID, playErr)
		} else if amiOutput != "" {
			details += ": " + amiOutput
		}
		recordAsteriskOperation(context.Background(), "audio_broadcast", localNode, "", track.PlaybackMode, false, command, result, details, now)
		if err := updateAudioScheduleAfterAttempt(context.Background(), track.ID, now, track.IntervalMinutes, playErr == nil); err != nil {
			log.Printf("audio: no se pudo actualizar próxima reproducción de pista %d: %v", track.ID, err)
		}
	}
}

func readAudioNodeState(instance allmonInstance) (audioNodeState, error) {
	nodeStat, err := config.ExecuteAMIRptStatus(instance.Host, instance.Port, instance.User, instance.Secret, instance.Node, "NodeStat", 3*time.Second)
	if err != nil {
		return audioNodeState{}, fmt.Errorf("NodeStat: %w", err)
	}
	cos, err := parseRPTCOS(nodeStat)
	if err != nil {
		return audioNodeState{}, err
	}
	sawStat, err := config.ExecuteAMIRptStatus(instance.Host, instance.Port, instance.User, instance.Secret, instance.Node, "SawStat", 3*time.Second)
	if err != nil {
		return audioNodeState{}, fmt.Errorf("SawStat: %w", err)
	}
	activities, _, err := parseRPTReceiveState(sawStat, instance.Node, time.Now().UTC())
	if err != nil {
		return audioNodeState{}, err
	}
	return audioNodeState{
		instance: instance,
		busy:     cos || len(activities) > 0,
	}, nil
}

func loadScheduledAudioTracks(ctx context.Context) ([]storedAudioTrack, error) {
	rows, err := config.App.DB.QueryContext(ctx, `
		SELECT t.id, t.name, t.stored_filename, t.playback_mode, t.enabled,
			t.duration_ms, s.interval_minutes, s.next_play_at
		FROM audio_tracks t
		LEFT JOIN audio_broadcast_schedules s ON s.audio_id = t.id
		WHERE t.playback_mode = 'broadcast_silence' AND t.enabled = 1
		ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tracks := make([]storedAudioTrack, 0)
	for rows.Next() {
		var track storedAudioTrack
		var enabled int
		var next sql.NullString
		if err := rows.Scan(&track.ID, &track.Name, &track.StoredFilename, &track.PlaybackMode, &enabled,
			&track.DurationMS, &track.IntervalMinutes, &next); err != nil {
			return nil, err
		}
		track.Enabled = enabled != 0
		track.NextPlayAt = parseAudioTimestamp(next)
		tracks = append(tracks, track)
	}
	return tracks, rows.Err()
}

func updateAudioScheduleAfterAttempt(ctx context.Context, id int64, now time.Time, intervalMinutes int, succeeded bool) error {
	if intervalMinutes < 1 {
		return fmt.Errorf("intervalo de reproducción inválido")
	}
	next := now.Add(time.Duration(intervalMinutes) * time.Minute).Format(time.RFC3339Nano)
	if succeeded {
		_, err := config.App.DB.ExecContext(ctx, `
			UPDATE audio_broadcast_schedules SET last_played_at = ?, next_play_at = ? WHERE audio_id = ?`,
			now.Format(time.RFC3339Nano), next, id)
		return err
	}
	_, err := config.App.DB.ExecContext(ctx, `
		UPDATE audio_broadcast_schedules SET next_play_at = ? WHERE audio_id = ?`, next, id)
	return err
}
