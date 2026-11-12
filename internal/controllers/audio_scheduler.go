package controllers

// Este archivo implementa la programación y coordinación de emisiones de audio.

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
	audioCycleMu       sync.Mutex
)

const audioQuietPeriod = 5 * time.Second

// StartAudioPlaybackScheduler inicia el programador de reproducción de audio, que verifica periódicamente el estado de los nodos y reproduce pistas programadas durante periodos de silencio.
func StartAudioPlaybackScheduler(manager *config.AudioStreamManager) {
	if config.App.DB == nil {
		log.Printf("audio: programador deshabilitado porque no hay base de datos")
		return
	}
	if manager == nil {
		log.Printf("audio: programador deshabilitado porque no hay receptor de audio")
		return
	}
	audioSchedulerOnce.Do(func() {
		go func() {
			openMonitor := func(node string) (scheduledSilenceMonitor, error) {
				return startAudioSilenceMonitor(manager, node)
			}
			interval := 5 * time.Second
			if configured := strings.TrimSpace(config.App.Audio.PollInterval); configured != "" {
				parsed, err := time.ParseDuration(configured)
				if err != nil || parsed < time.Second || parsed > time.Minute {
					log.Printf("audio: intervalo de sondeo %q inválido; se usa %s", configured, interval)
				} else {
					interval = parsed
				}
			}
			runAudioPlaybackCycle(time.Now().UTC(), openMonitor)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for range ticker.C {
				runAudioPlaybackCycle(time.Now().UTC(), openMonitor)
			}
		}()
	})
}

// runAudioPlaybackCycle ejecuta un ciclo de comprobación de reproducción de audio, evaluando el estado de los nodos y reproduciendo pistas programadas si se cumple el periodo de silencio.
func runAudioPlaybackCycle(now time.Time, openMonitor func(string) (scheduledSilenceMonitor, error)) {
	if !audioCycleMu.TryLock() {
		return
	}
	defer audioCycleMu.Unlock()
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
	primary, err := newPrimaryNodeControl().primary()
	if err != nil {
		log.Printf("audio: no se pudo identificar el nodo principal para comprobar silencio: %v", err)
		return
	}
	for _, track := range tracks {
		if !track.Enabled || track.PlaybackMode != "broadcast_silence" || track.NextPlayAt == nil || track.NextPlayAt.After(now) {
			continue
		}
		monitor, err := openMonitor(primary)
		if err != nil {
			log.Printf("audio: no se pudo iniciar monitor de silencio: %v", err)
			return
		}
		defer func() {
			if err := monitor.close(); err != nil {
				log.Printf("audio: no se pudo cerrar monitor de silencio: %v", err)
			}
		}()
		silent, err := monitor.wait()
		if err != nil {
			log.Printf("audio: no se pudo confirmar silencio: %v", err)
			return
		}
		if !silent {
			if err := postponeScheduledAudioPlayback(context.Background(), track, now); err != nil {
				log.Printf("audio: no se pudo aplazar pista %d: %v", track.ID, err)
			}
			return
		}
		token, claimErr := claimScheduledAudioPlayback(context.Background(), track, now)
		if errors.Is(claimErr, errAudioPlaybackBusy) {
			return
		}
		if claimErr != nil {
			log.Printf("audio: no se pudo reservar reproducción programada para pista %d: %v", track.ID, claimErr)
			return
		}
		filename, playErr := prepareAudioTrackPlayback(track)
		var command, localNode, amiOutput string
		if playErr == nil {
			silent, checkErr := monitor.check()
			if checkErr != nil || !silent {
				if err := cancelScheduledAudioPlayback(context.Background(), track, token, now); err != nil {
					log.Printf("audio: no se pudo cancelar reserva de pista %d: %v", track.ID, err)
				}
				if checkErr != nil {
					log.Printf("audio: no se pudo confirmar silencio antes de emitir: %v", checkErr)
				} else if err := postponeScheduledAudioPlayback(context.Background(), track, now); err != nil {
					log.Printf("audio: no se pudo aplazar pista %d: %v", track.ID, err)
				}
				return
			}
			command, localNode, amiOutput, playErr = playAudioOnPrimary(filename)
		}
		result, details := "success", fmt.Sprintf("Audio %q enviado durante silencio total", track.Name)
		if track.EndMS != 0 {
			details += fmt.Sprintf(" [%d-%d ms]", track.StartMS, track.EndMS)
		}
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
		return
	}
}

func postponeScheduledAudioPlayback(ctx context.Context, track storedAudioTrack, now time.Time) error {
	if track.NextPlayAt == nil || track.IntervalMinutes < 1 {
		return errors.New("programación inválida")
	}
	_, err := config.App.DB.ExecContext(ctx, `
		UPDATE audio_broadcast_schedules SET next_play_at = ?
		WHERE audio_id = ? AND next_play_at = ? AND interval_minutes = ?
		AND EXISTS (SELECT 1 FROM audio_tracks WHERE id = ? AND enabled = 1 AND playback_mode = 'broadcast_silence')`,
		now.Add(time.Duration(track.IntervalMinutes)*time.Minute).Format(time.RFC3339Nano),
		track.ID, track.NextPlayAt.Format(time.RFC3339Nano), track.IntervalMinutes, track.ID)
	return err
}

// claimScheduledAudioPlayback intenta reclamar la reproducción programada de una pista de audio, asegurando que la pista esté lista para reproducirse y que no haya conflictos de programación.
func claimScheduledAudioPlayback(ctx context.Context, track storedAudioTrack, now time.Time) (string, error) {
	if track.NextPlayAt == nil || track.NextPlayAt.After(now) || track.IntervalMinutes < 1 {
		return "", fmt.Errorf("programación inválida")
	}
	tx, err := config.App.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	next := now.Add(time.Duration(track.IntervalMinutes) * time.Minute).Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE audio_broadcast_schedules SET next_play_at = ?
		WHERE audio_id = ? AND next_play_at = ? AND interval_minutes = ?
		AND EXISTS (SELECT 1 FROM audio_tracks WHERE id = ? AND enabled = 1 AND playback_mode = 'broadcast_silence')`,
		next, track.ID, track.NextPlayAt.Format(time.RFC3339Nano), track.IntervalMinutes, track.ID)
	if err != nil {
		return "", err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if count != 1 {
		return "", errAudioPlaybackBusy
	}
	token, err := claimAudioPlaybackTx(ctx, tx, track.playbackDurationMS())
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

// cancelScheduledAudioPlayback cancela la reproducción programada de una pista de audio previamente reclamada, liberando el token de reproducción y actualizando la programación.
func cancelScheduledAudioPlayback(ctx context.Context, track storedAudioTrack, token string, now time.Time) error {
	tx, err := config.App.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE audio_broadcast_schedules SET next_play_at = ?
		WHERE audio_id = ? AND next_play_at = ? AND interval_minutes = ?
		AND EXISTS (SELECT 1 FROM audio_playback_state WHERE id = 1 AND playback_token = ?)`,
		track.NextPlayAt.Format(time.RFC3339Nano), track.ID,
		now.Add(time.Duration(track.IntervalMinutes)*time.Minute).Format(time.RFC3339Nano), track.IntervalMinutes, token); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE audio_playback_state SET busy_until = '1970-01-01T00:00:00Z', playback_token = ''
		WHERE id = 1 AND playback_token = ?`, token); err != nil {
		return err
	}
	return tx.Commit()
}

// loadScheduledAudioTracks carga todas las pistas de audio programadas para reproducción en modo de silencio, devolviendo sus detalles y la programación asociada.
func loadScheduledAudioTracks(ctx context.Context) ([]storedAudioTrack, error) {
	rows, err := config.App.DB.QueryContext(ctx, `
		SELECT t.id, t.name, t.stored_filename, t.playback_mode, t.enabled,
			t.duration_ms, s.interval_minutes, s.next_play_at, t.start_ms, t.end_ms
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
			&track.DurationMS, &track.IntervalMinutes, &next, &track.StartMS, &track.EndMS); err != nil {
			return nil, err
		}
		track.Enabled = enabled != 0
		track.NextPlayAt = parseAudioTimestamp(next)
		tracks = append(tracks, track)
	}
	return tracks, rows.Err()
}

// updateAudioScheduleAfterAttempt actualiza la programación de reproducción de una pista de audio después de un intento de reproducción, ajustando el siguiente tiempo de reproducción según el resultado.
func updateAudioScheduleAfterAttempt(ctx context.Context, id int64, now time.Time, intervalMinutes int, succeeded bool) error {
	if intervalMinutes < 1 {
		return fmt.Errorf("intervalo de reproducción inválido")
	}
	next := now.Add(time.Duration(intervalMinutes) * time.Minute).Format(time.RFC3339Nano)
	if succeeded {
		_, err := config.App.DB.ExecContext(ctx, `
			UPDATE audio_broadcast_schedules SET last_played_at = ?,
				next_play_at = CASE WHEN interval_minutes = ? THEN ? ELSE next_play_at END
			WHERE audio_id = ?`,
			now.Format(time.RFC3339Nano), intervalMinutes, next, id)
		return err
	}
	_, err := config.App.DB.ExecContext(ctx, `
		UPDATE audio_broadcast_schedules SET next_play_at = ? WHERE audio_id = ? AND interval_minutes = ?`, next, id, intervalMinutes)
	return err
}
