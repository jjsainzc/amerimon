package controllers

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"amerimon/internal/config"
)

const (
	audioSilenceStartTimeout = 12 * time.Second
	audioSilenceFrameTimeout = 500 * time.Millisecond
	audioSilenceBytes        = 5 * 16000
)

type scheduledSilenceMonitor interface {
	wait() (bool, error)
	check() (bool, error)
	close() error
}

type audioSilenceMonitor struct {
	mu      sync.Mutex
	started time.Time
	last    time.Time
	bytes   int
	busy    bool
	closed  bool
	manager *config.AudioStreamManager
	uuid    string
}

func startAudioSilenceMonitor(manager *config.AudioStreamManager, node string) (scheduledSilenceMonitor, error) {
	filename, err := newAudioFilename()
	if err != nil {
		return nil, err
	}
	monitor := &audioSilenceMonitor{manager: manager, uuid: strings.TrimSuffix(filename, ".wav")}
	manager.AddConsumer(monitor.uuid, monitor)
	if err := originateNodeAudioCapture(fmt.Sprintf("Local/%s@amerimon-record-node/n", node), monitor.uuid); err != nil {
		if closeErr := monitor.close(); closeErr != nil {
			return nil, errors.Join(err, closeErr)
		}
		return nil, err
	}
	return monitor, nil
}

func (m *audioSilenceMonitor) Write(p []byte) (int, error) {
	return m.writeAt(p, time.Now())
}

func (m *audioSilenceMonitor) writeAt(p []byte, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 || len(p)%2 != 0 {
		return 0, errors.New("trama PCM inválida")
	}
	if m.started.IsZero() {
		m.started = now
	}
	if !m.last.IsZero() && now.Sub(m.last) > audioSilenceFrameTimeout {
		m.closed = true
		return 0, errors.New("flujo PCM interrumpido")
	}
	m.last = now
	m.bytes += len(p)
	if now.Sub(m.started)-time.Duration(m.bytes)*time.Second/16000 > audioSilenceFrameTimeout {
		m.closed = true
		return 0, errors.New("flujo PCM incompleto")
	}
	if audioPCMSquaredSum(p) > int64(len(p)/2)*327*327 {
		m.busy = true
	}
	return len(p), nil
}

func (m *audioSilenceMonitor) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *audioSilenceMonitor) state(now time.Time) (bool, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false, false, errors.New("flujo PCM cerrado")
	}
	if m.busy {
		return false, true, nil
	}
	if !m.last.IsZero() && (now.Sub(m.last) > audioSilenceFrameTimeout ||
		now.Sub(m.started)-time.Duration(m.bytes)*time.Second/16000 > audioSilenceFrameTimeout) {
		return false, false, errors.New("flujo PCM sin muestras recientes")
	}
	return !m.started.IsZero() && now.Sub(m.started) >= audioQuietPeriod && m.bytes >= audioSilenceBytes, false, nil
}

func (m *audioSilenceMonitor) wait() (bool, error) {
	deadline := time.NewTimer(audioSilenceStartTimeout + audioQuietPeriod)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			quiet, busy, err := m.state(time.Now())
			if err != nil || busy || quiet {
				return quiet, err
			}
		case <-deadline.C:
			return false, errors.New("no se recibieron cinco segundos de audio PCM")
		}
	}
}

func (m *audioSilenceMonitor) check() (bool, error) {
	quiet, _, err := m.state(time.Now())
	return quiet, err
}

func (m *audioSilenceMonitor) close() error {
	m.manager.RemoveConsumer(m.uuid)
	m.manager.CloseAudioConnection(m.uuid)
	_ = m.Close()
	_, err := hangupListenLiveCalls("audio-silence", m.uuid)
	return err
}
