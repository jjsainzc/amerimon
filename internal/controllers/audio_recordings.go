package controllers

// Este archivo implementa la gestión de grabaciones de audio del nodo principal.

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

const recordingColumns = `id, name, node, uuid, stored_filename, state, file_size, duration_ms, created_at, COALESCE(finished_at, ''), error_code, library_track_id`

// AudioRecording representa la información de una grabación de audio almacenada en el nodo principal.
type AudioRecording struct {
	ID             int64         `json:"id"`
	Name           string        `json:"name"`
	Node           string        `json:"node"`
	UUID           string        `json:"-"`
	Filename       string        `json:"-"`
	State          string        `json:"state"`
	FileSize       int64         `json:"file_size"`
	DurationMS     int64         `json:"duration_ms"`
	CreatedAt      string        `json:"created_at"`
	FinishedAt     string        `json:"finished_at,omitempty"`
	ErrorCode      string        `json:"error_code,omitempty"`
	LibraryTrackID sql.NullInt64 `json:"-"`
	InLibrary      bool          `json:"in_library"`
	AutoPaused     bool          `json:"auto_paused"`
	LevelDBFS      float64       `json:"level_dbfs"`
}

// scanRecording escanea una fila de la base de datos y devuelve una estructura AudioRecording con la información correspondiente.
func scanRecording(row interface{ Scan(...any) error }) (AudioRecording, error) {
	r := AudioRecording{LevelDBFS: -60}
	err := row.Scan(&r.ID, &r.Name, &r.Node, &r.UUID, &r.Filename, &r.State, &r.FileSize,
		&r.DurationMS, &r.CreatedAt, &r.FinishedAt, &r.ErrorCode, &r.LibraryTrackID)
	r.InLibrary = r.LibraryTrackID.Valid
	return r, err
}

// recordingStoragePath devuelve la ruta del directorio donde se almacenan las grabaciones de audio.
func recordingStoragePath() string {
	return filepath.Join(filepath.Dir(config.App.DBPath), "recordings")
}

// recordingWAVHeader genera un encabezado WAV para una grabación de audio con el tamaño especificado.
func recordingWAVHeader(size int64) []byte {
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+size))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 1)
	binary.LittleEndian.PutUint32(h[24:], 8000)
	binary.LittleEndian.PutUint32(h[28:], 16000)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(size))
	return h
}

// recordingSink representa un sumidero de grabación de audio que maneja la escritura de datos PCM en un archivo y actualiza el estado de la grabación.
type recordingSink struct {
	mu       sync.Mutex
	record   AudioRecording
	file     *os.File
	database *sql.DB
	ready    chan struct{}
	received bool
	closed   bool
	lastSync time.Time
	preRoll  []byte
	tail     int
	levelAt  time.Time
}

// PCM at 8 kHz: 200 ms pre-roll, 500 ms release, RMS threshold about -40 dBFS.
// Write escribe datos PCM en el archivo de grabación, actualiza el nivel RMS y maneja la lógica de auto-pausa basada en la audibilidad.
func (s *recordingSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, io.ErrClosedPipe
	}
	if !s.received {
		s.received = true
		s.record.State = "recording"
		close(s.ready)
	}
	if len(p)%2 != 0 {
		return 0, s.failLocked(errors.New("PCM frame has an odd length"))
	}
	var sum int64
	for i := 0; i < len(p); i += 2 {
		sample := int64(int16(binary.LittleEndian.Uint16(p[i:])))
		sum += sample * sample
	}
	level := -60.0
	if sum > 0 {
		level = math.Max(-60, math.Min(0, 10*math.Log10(float64(sum)/float64(len(p)/2)/(32768*32768))))
	}
	now := time.Now()
	if now.Sub(s.levelAt) >= 500*time.Millisecond {
		s.record.LevelDBFS = level
		s.levelAt = now
	} else {
		s.record.LevelDBFS = math.Max(s.record.LevelDBFS, level)
	}
	if s.record.State == "paused" {
		return len(p), nil
	}
	audible := len(p) > 0 && sum > int64(len(p)/2)*327*327
	if !audible && s.tail <= 0 {
		s.record.AutoPaused = true
		s.preRoll = append(s.preRoll, p...)
		if len(s.preRoll) > 3200 {
			s.preRoll = s.preRoll[len(s.preRoll)-3200:]
		}
	} else {
		s.record.AutoPaused = false
		if audible {
			s.tail = 8000
		} else {
			s.tail -= len(p)
		}
		data := append(s.preRoll, p...)
		s.preRoll = nil
		if s.record.FileSize+int64(len(data)) > audioMaxUploadBytes {
			if err := s.finishLocked("completed", "AUDIO_RECORD_LIMIT"); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		n, err := s.file.Write(data)
		s.record.FileSize += int64(n)
		s.record.DurationMS = (s.record.FileSize - 44) / 16
		if err != nil {
			return 0, s.failLocked(err)
		}
	}
	if time.Since(s.lastSync) >= time.Second {
		if err := s.checkpointLocked(); err != nil {
			return 0, s.failLocked(err)
		}
	}
	return len(p), nil
}

// snapshot devuelve una instantánea del estado actual de la grabación de audio, incluyendo el nivel RMS y el estado de auto-pausa.
func (s *recordingSink) snapshot() AudioRecording {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.record
	if s.closed || time.Since(s.levelAt) > time.Second {
		record.LevelDBFS = -60
	}
	return record
}

// checkpointLocked actualiza el encabezado WAV y persiste el estado de la grabación en la base de datos.
func (s *recordingSink) checkpointLocked() error {
	if _, err := s.file.WriteAt(recordingWAVHeader(s.record.FileSize-44), 0); err != nil {
		return err
	}
	if err := s.file.Sync(); err != nil {
		return err
	}
	_, err := s.database.Exec(`UPDATE audio_recordings SET state=?, file_size=?, duration_ms=?, finished_at=NULLIF(?, ''), error_code=? WHERE id=?`,
		s.record.State, s.record.FileSize, s.record.DurationMS, s.record.FinishedAt, s.record.ErrorCode, s.record.ID)
	s.lastSync = time.Now()
	return err
}

// failLocked marca la grabación como fallida y registra el error.
func (s *recordingSink) failLocked(cause error) error {
	log.Printf("audio recording %d: %v", s.record.ID, cause)
	return errors.Join(cause, s.finishLocked("failed", "AUDIO_RECORD_WRITE_ERROR"))
}

// finishLocked finaliza la grabación con el estado y código de error especificados, cerrando el archivo y actualizando la base de datos.
func (s *recordingSink) finishLocked(state, code string) error {
	if s.closed {
		return nil
	}
	s.closed = true
	s.record.State, s.record.ErrorCode = state, code
	s.record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	err := errors.Join(s.checkpointLocked(), s.file.Close())
	if err != nil {
		s.record.State, s.record.ErrorCode = "failed", "AUDIO_RECORD_WRITE_ERROR"
		_, persistErr := s.database.Exec(`UPDATE audio_recordings SET state='failed', error_code=?, finished_at=? WHERE id=?`,
			s.record.ErrorCode, s.record.FinishedAt, s.record.ID)
		return errors.Join(err, persistErr)
	}
	return nil
}

// Close cierra la grabación de audio, marcándola como interrumpida si aún no se ha finalizado.
func (s *recordingSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finishLocked("interrupted", "AUDIO_RECORD_INTERRUPTED")
}

// AudioRecordingControl gestiona el control de grabaciones de audio, incluyendo la grabación activa y las operaciones de origen y colgado.
type AudioRecordingControl struct {
	mu        sync.Mutex
	manager   *config.AudioStreamManager
	active    *recordingSink
	originate func(channel, uuid string) error
	hangup    func(uuid string) error
}

// NewAudioRecordingControl crea un nuevo controlador de grabación de audio, inicializando los métodos de origen y colgado.
func NewAudioRecordingControl(manager *config.AudioStreamManager) *AudioRecordingControl {
	return &AudioRecordingControl{
		manager: manager,
		originate: func(channel, uuid string) error {
			ami := config.CurrentAMI()
			path := ami.RPTConfigPath
			if path == "" {
				path = "/etc/asterisk/rpt.conf"
			}
			absolute, err := filepath.Abs(path)
			if err != nil {
				return err
			}
			return config.ExecuteAMIOriginate(ami.Host, ami.Port, ami.User, ami.Secret, channel,
				"listen-live-eagi", uuid, 5*time.Second,
				config.AMIOriginateVariable{Name: "__AMERIMON_RPT_CONFIG", Value: absolute})
		},
		hangup: func(uuid string) error {
			_, err := hangupListenLiveCalls("recording", uuid)
			return err
		},
	}
}

// recordingError maneja la respuesta de error de la API de grabación de audio, registrando el error y devolviendo un JSON con el código de error.
func recordingError(c echo.Context, status int, code string, err error) error {
	if err != nil {
		log.Printf("audio recording: %s: %v", code, err)
	}
	return c.JSON(status, echo.Map{"status": "error", "code": code})
}

// List devuelve la lista de grabaciones de audio, incluyendo la grabación activa si existe.
func (p *AudioRecordingControl) List(c echo.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if config.App.DB == nil {
		return recordingError(c, 503, "AUDIO_ERROR_DATABASE_UNAVAILABLE", nil)
	}
	rows, err := config.App.DB.QueryContext(c.Request().Context(), `SELECT `+recordingColumns+` FROM audio_recordings ORDER BY id DESC`)
	if err != nil {
		return recordingError(c, 500, "AUDIO_RECORD_LOAD_ERROR", err)
	}
	defer rows.Close()
	result := make([]AudioRecording, 0)
	for rows.Next() {
		r, err := scanRecording(rows)
		if err != nil {
			return recordingError(c, 500, "AUDIO_RECORD_LOAD_ERROR", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return recordingError(c, 500, "AUDIO_RECORD_LOAD_ERROR", err)
	}
	rows.Close()
	if p.active != nil {
		current := p.active.snapshot()
		for i := range result {
			if current.ID == result[i].ID && (current.State == "starting" || current.State == "recording" || current.State == "paused") {
				result[i] = current
			}
		}
	}
	return c.JSON(200, echo.Map{"status": "success", "data": result})
}

// Start inicia una nueva grabación de audio, creando un canal de origen y configurando la grabación activa.
func (p *AudioRecordingControl) Start(c echo.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if config.App.DB == nil || p.manager == nil {
		return recordingError(c, 503, "AUDIO_ERROR_DATABASE_UNAVAILABLE", nil)
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.Bind(&req); err != nil || !audioTrackNamePattern.MatchString(strings.TrimSpace(req.Name)) {
		return recordingError(c, 400, "AUDIO_ERROR_NAME", err)
	}
	if p.active != nil {
		state := p.active.snapshot().State
		if state == "starting" || state == "recording" || state == "paused" {
			return recordingError(c, 409, "AUDIO_RECORD_BUSY", nil)
		}
	}
	channel, err := listenLiveOriginateChannel("nodo")
	if err != nil {
		return recordingError(c, 502, "AUDIO_RECORD_START_ERROR", err)
	}
	channel = strings.Replace(channel, "@amerimon-listen-node/", "@amerimon-record-node/", 1)
	filename, err := newAudioFilename()
	if err != nil {
		return recordingError(c, 500, "AUDIO_RECORD_START_ERROR", err)
	}
	if err := os.MkdirAll(recordingStoragePath(), 0o700); err != nil {
		return recordingError(c, 500, "AUDIO_RECORD_WRITE_ERROR", err)
	}
	path := filepath.Join(recordingStoragePath(), filename)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return recordingError(c, 500, "AUDIO_RECORD_WRITE_ERROR", err)
	}
	if _, err := f.Write(recordingWAVHeader(0)); err != nil {
		f.Close()
		if cleanupErr := os.Remove(path); cleanupErr != nil {
			log.Printf("audio recording cleanup: %v", cleanupErr)
		}
		return recordingError(c, 500, "AUDIO_RECORD_WRITE_ERROR", err)
	}
	r := AudioRecording{
		Name: strings.TrimSpace(req.Name), Node: strings.Split(strings.TrimPrefix(channel, "Local/"), "@")[0],
		UUID: strings.TrimSuffix(filename, ".wav"), Filename: filename, State: "starting", FileSize: 44,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), AutoPaused: true,
	}
	res, err := config.App.DB.ExecContext(c.Request().Context(),
		`INSERT INTO audio_recordings (name,node,uuid,stored_filename,state,created_at) VALUES (?,?,?,?,?,?)`,
		r.Name, r.Node, r.UUID, r.Filename, r.State, r.CreatedAt)
	if err == nil {
		r.ID, err = res.LastInsertId()
	}
	if err != nil {
		f.Close()
		if cleanupErr := os.Remove(path); cleanupErr != nil {
			log.Printf("audio recording cleanup: %v", cleanupErr)
		}
		return recordingError(c, 500, "AUDIO_RECORD_START_ERROR", err)
	}
	s := &recordingSink{record: r, file: f, database: config.App.DB, ready: make(chan struct{})}
	p.active = s
	p.manager.AddConsumer(r.UUID, s)
	err = p.originate(channel, r.UUID)
	if err == nil {
		select {
		case <-s.ready:
			if state := s.snapshot().State; state == "recording" {
				return c.JSON(http.StatusCreated, echo.Map{"status": "success", "data": s.snapshot()})
			}
			err = errors.New("audio stream ended during startup")
		case <-time.After(12 * time.Second):
			err = errors.New("no PCM audio received from EAGI")
		case <-c.Request().Context().Done():
			err = c.Request().Context().Err()
		}
	}
	p.manager.RemoveConsumer(r.UUID)
	p.manager.CloseAudioConnection(r.UUID)
	s.mu.Lock()
	finishErr := s.finishLocked("failed", "AUDIO_RECORD_START_ERROR")
	s.mu.Unlock()
	return recordingError(c, 502, "AUDIO_RECORD_START_ERROR", errors.Join(err, finishErr, p.hangup(r.UUID)))
}

// Change modifica el estado de la grabación activa, permitiendo pausar, reanudar o detener la grabación.
func (p *AudioRecordingControl) Change(c echo.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	var req struct {
		Action string `json:"action"`
	}
	if err != nil || id < 1 || c.Bind(&req) != nil ||
		(req.Action != "pause" && req.Action != "resume" && req.Action != "stop") {
		return recordingError(c, 400, "AUDIO_ERROR_REQUEST_INVALID", err)
	}
	s := p.active
	if s == nil || s.snapshot().ID != id {
		return recordingError(c, 409, "AUDIO_RECORD_NOT_ACTIVE", nil)
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return recordingError(c, 409, "AUDIO_RECORD_NOT_ACTIVE", nil)
	}
	if req.Action == "stop" {
		err = s.finishLocked("completed", "")
	} else {
		s.record.State = "paused"
		if req.Action == "resume" {
			s.record.State = "recording"
		}
		s.preRoll, s.tail = nil, 0
		s.record.AutoPaused = req.Action == "resume"
		err = s.checkpointLocked()
	}
	s.mu.Unlock()
	if req.Action == "stop" {
		p.manager.RemoveConsumer(s.record.UUID)
		p.manager.CloseAudioConnection(s.record.UUID)
		err = errors.Join(err, p.hangup(s.record.UUID))
	}
	if err != nil {
		return recordingError(c, 500, "AUDIO_RECORD_CONTROL_ERROR", err)
	}
	return c.JSON(200, echo.Map{"status": "success", "data": s.snapshot()})
}

// saved obtiene la grabación de audio guardada correspondiente al ID proporcionado en la solicitud.
func (p *AudioRecordingControl) saved(c echo.Context) (AudioRecording, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return AudioRecording{}, errors.New("invalid recording ID")
	}
	if config.App.DB == nil {
		return AudioRecording{}, errors.New("database unavailable")
	}
	r, err := scanRecording(config.App.DB.QueryRowContext(c.Request().Context(), `SELECT `+recordingColumns+` FROM audio_recordings WHERE id=?`, id))
	if err != nil {
		return r, err
	}
	if !audioFilenamePattern.MatchString(r.Filename) {
		return r, errors.New("invalid stored filename")
	}
	if r.State == "starting" || r.State == "recording" || r.State == "paused" {
		return r, errors.New("recording is still active")
	}
	return r, nil
}

// File devuelve el archivo de audio correspondiente a la grabación guardada, estableciendo los encabezados HTTP adecuados para la respuesta.
func (p *AudioRecordingControl) File(c echo.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, err := p.saved(c)
	if err != nil {
		return recordingError(c, 409, "AUDIO_RECORD_FILE_ERROR", err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Content-Type", "audio/wav")
	return c.File(filepath.Join(recordingStoragePath(), r.Filename))
}

// Delete elimina la grabación de audio guardada y su archivo asociado.
func (p *AudioRecordingControl) Delete(c echo.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, err := p.saved(c)
	if err != nil {
		return recordingError(c, 409, "AUDIO_RECORD_DELETE_ERROR", err)
	}
	if err := os.Remove(filepath.Join(recordingStoragePath(), r.Filename)); err != nil && !os.IsNotExist(err) {
		return recordingError(c, 500, "AUDIO_RECORD_DELETE_ERROR", err)
	}
	if _, err := config.App.DB.ExecContext(c.Request().Context(), `DELETE FROM audio_recordings WHERE id=?`, r.ID); err != nil {
		return recordingError(c, 500, "AUDIO_RECORD_DELETE_ERROR", err)
	}
	return c.NoContent(http.StatusNoContent)
}

// AddToLibrary agrega la grabación de audio guardada a la biblioteca de audio, normalizando el archivo WAV y verificando su tamaño.
func (p *AudioRecordingControl) AddToLibrary(c echo.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, err := p.saved(c)
	if err != nil {
		return recordingError(c, 409, "AUDIO_RECORD_LIBRARY_ERROR", err)
	}
	if r.LibraryTrackID.Valid {
		return recordingError(c, 409, "AUDIO_RECORD_ALREADY_ADDED", nil)
	}
	f, err := os.Open(filepath.Join(recordingStoragePath(), r.Filename))
	if err != nil {
		return recordingError(c, 500, "AUDIO_RECORD_FILE_ERROR", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, audioMaxUploadBytes+1))
	if err != nil || len(data) > audioMaxUploadBytes {
		return recordingError(c, 500, "AUDIO_RECORD_FILE_ERROR", err)
	}
	data, duration, rate, bits, err := normalizeAudioWAV(data)
	if err != nil {
		return recordingError(c, 400, "AUDIO_RECORD_EMPTY", err)
	}
	return persistAudioTrack(c, r.Name, r.Name+".wav", "broadcast_once", 0, data, duration, rate, bits, r.ID)
}

// RecoverAudioRecordings recupera las grabaciones de audio que quedaron en estado activo tras un cierre inesperado, verificando y corrigiendo los archivos WAV.
func RecoverAudioRecordings() error {
	rows, err := config.App.DB.Query(`SELECT ` + recordingColumns + ` FROM audio_recordings WHERE state IN ('starting','recording','paused')`)
	if err != nil {
		return err
	}
	var records []AudioRecording
	for rows.Next() {
		r, scanErr := scanRecording(rows)
		if scanErr != nil {
			rows.Close()
			return scanErr
		}
		records = append(records, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range records {
		if !audioFilenamePattern.MatchString(r.Filename) {
			return fmt.Errorf("invalid recording filename for %d", r.ID)
		}
		f, openErr := os.OpenFile(filepath.Join(recordingStoragePath(), r.Filename), os.O_RDWR, 0)
		state, code, size := "interrupted", "AUDIO_RECORD_INTERRUPTED", int64(44)
		if openErr == nil {
			info, statErr := f.Stat()
			if statErr != nil {
				f.Close()
				return statErr
			}
			if info.Size() < 44 || info.Size() > audioMaxUploadBytes {
				f.Close()
				return fmt.Errorf("invalid recording size for %d", r.ID)
			}
			size = 44 + (info.Size()-44)/2*2
			writeErr := f.Truncate(size)
			if writeErr == nil {
				_, writeErr = f.WriteAt(recordingWAVHeader(size-44), 0)
			}
			if err := errors.Join(writeErr, f.Sync(), f.Close()); err != nil {
				return err
			}
		} else if errors.Is(openErr, os.ErrNotExist) {
			state, code = "failed", "AUDIO_RECORD_FILE_ERROR"
		} else {
			return openErr
		}
		if _, err := config.App.DB.ExecContext(context.Background(),
			`UPDATE audio_recordings SET state=?, error_code=?, file_size=?, duration_ms=?, finished_at=? WHERE id=?`,
			state, code, size, (size-44)/16, time.Now().UTC().Format(time.RFC3339Nano), r.ID); err != nil {
			return err
		}
		log.Printf("audio recording recovered id=%d state=%s", r.ID, state)
	}
	return nil
}
