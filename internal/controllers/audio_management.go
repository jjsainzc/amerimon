package controllers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

const (
	audioMaxUploadBytes = 25 << 20
	audioMaxStoredBytes = audioMaxUploadBytes*2 + (1 << 20)
	audioSubdirectory   = "amerimon"
	audioMaxInterval    = 10080
	defaultAsteriskDir  = "/var/lib/asterisk"
	legacyAudioRoot     = "/var/lib/asterisk/sounds"
)

var errAudioPlaybackBusy = errors.New("ya hay otro audio en reproducción")
var errAudioDuplexRequired = errors.New("el nodo principal requiere un valor duplex distinto de 0 en rpt.conf para emitir a los nodos enlazados")

var audioTrackNamePattern = regexp.MustCompile(`^[\pL\pN _().,'-]{1,100}$`)
var audioFilenamePattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}\.wav$`)
var rptAudioSectionPattern = regexp.MustCompile(`^\[([^\]]+)\]\s*(?:\(([^)]*)\))?`)

type AudioTrack struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	OriginalName    string     `json:"original_name"`
	PlaybackMode    string     `json:"playback_mode"`
	Enabled         bool       `json:"enabled"`
	FileSize        int64      `json:"file_size"`
	DurationMS      int64      `json:"duration_ms"`
	StartMS         int64      `json:"start_ms"`
	EndMS           int64      `json:"end_ms"`
	SampleRate      int        `json:"sample_rate"`
	BitsPerSample   int        `json:"bits_per_sample"`
	IntervalMinutes int        `json:"interval_minutes,omitempty"`
	LastPlayedAt    *time.Time `json:"last_played_at,omitempty"`
	NextPlayAt      *time.Time `json:"next_play_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type storedAudioTrack struct {
	AudioTrack
	StoredFilename string
}

func audioStoragePath() string {
	root := strings.TrimSpace(config.App.Audio.StorageDir)
	dataDir, _ := asteriskDirectories()
	if root == "" || filepath.Clean(root) == legacyAudioRoot {
		root = filepath.Join(dataDir, "sounds")
	}
	return filepath.Join(root, audioSubdirectory)
}

func legacyAudioStoragePath() string {
	_, varLibDir := asteriskDirectories()
	return filepath.Join(varLibDir, "sounds", audioSubdirectory)
}

func asteriskDirectories() (dataDir, varLibDir string) {
	dataDir, varLibDir = defaultAsteriskDir, defaultAsteriskDir
	rptConfigPath := strings.TrimSpace(config.App.AMI.RPTConfigPath)
	if rptConfigPath == "" {
		rptConfigPath = "/etc/asterisk/rpt.conf"
	}
	configPath := filepath.Join(filepath.Dir(rptConfigPath), "asterisk.conf")
	file, err := os.Open(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("audio: no se pudo leer %s: %v", configPath, err)
		}
		return dataDir, varLibDir
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		line, _, _ = strings.Cut(line, ";")
		key, value, ok := strings.Cut(line, "=>")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if !filepath.IsAbs(value) {
			continue
		}
		switch key {
		case "astdatadir":
			dataDir = filepath.Clean(value)
		case "astvarlibdir":
			varLibDir = filepath.Clean(value)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("audio: no se pudo recorrer %s: %v", configPath, err)
	}
	return dataDir, varLibDir
}

func validAudioPlaybackMode(mode string) bool {
	switch mode {
	case "broadcast_once", "broadcast_silence":
		return true
	default:
		return false
	}
}

func normalizeAudioWAV(data []byte) ([]byte, time.Duration, int, int, error) {
	if len(data) < 44 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, 0, 0, errors.New("el archivo no es un WAV válido")
	}
	riffSize := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if riffSize > uint64(len(data)) {
		return nil, 0, 0, 0, errors.New("el archivo WAV está incompleto")
	}

	var (
		format, channels, sampleRate, byteRate, blockAlign, bits int
		hasFormat, hasData                                       bool
		audioDataSize                                            uint64
	)
	for offset := 12; offset+8 <= int(riffSize); {
		chunkSize := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := uint64(offset + 8)
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > riffSize {
			return nil, 0, 0, 0, errors.New("el archivo WAV contiene un bloque inválido")
		}
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if chunkSize < 16 || hasFormat {
				return nil, 0, 0, 0, errors.New("el bloque de formato WAV no es válido")
			}
			chunk := data[chunkStart:chunkEnd]
			format = int(binary.LittleEndian.Uint16(chunk[0:2]))
			channels = int(binary.LittleEndian.Uint16(chunk[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(chunk[4:8]))
			byteRate = int(binary.LittleEndian.Uint32(chunk[8:12]))
			blockAlign = int(binary.LittleEndian.Uint16(chunk[12:14]))
			bits = int(binary.LittleEndian.Uint16(chunk[14:16]))
			hasFormat = true
		case "data":
			hasData = chunkSize > 0
			audioDataSize += chunkSize
		}
		next := chunkEnd + (chunkSize & 1)
		if next > riffSize || next > uint64(math.MaxInt) {
			return nil, 0, 0, 0, errors.New("el archivo WAV contiene un bloque inválido")
		}
		offset = int(next)
	}
	bytesPerSample := bits / 8
	if !hasFormat || !hasData || format != 1 || channels != 1 || sampleRate != 8000 ||
		(bits != 8 && bits != 16) || blockAlign != bytesPerSample || byteRate != sampleRate*bytesPerSample ||
		audioDataSize%uint64(bytesPerSample) != 0 {
		return nil, 0, 0, 0, errors.New("el audio debe ser PCM WAV mono, 8 o 16 bits, 8000 Hz")
	}
	if audioDataSize > uint64(math.MaxInt64/1000) {
		return nil, 0, 0, 0, errors.New("la duración WAV supera el límite admitido")
	}
	sampleCount := audioDataSize / uint64(bytesPerSample)
	durationMS := (sampleCount*1000 + uint64(sampleRate) - 1) / uint64(sampleRate)
	if durationMS == 0 {
		durationMS = 1
	}
	if bits == 16 {
		return data[:riffSize], time.Duration(durationMS) * time.Millisecond, sampleRate, bits, nil
	}
	normalized, err := convertPCM8BitWAVTo16Bit(data[:riffSize])
	if err != nil {
		return nil, 0, 0, 0, err
	}
	return normalized, time.Duration(durationMS) * time.Millisecond, sampleRate, 16, nil
}

func convertPCM8BitWAVTo16Bit(data []byte) ([]byte, error) {
	var output bytes.Buffer
	output.Grow(len(data) * 2)
	output.WriteString("RIFF")
	_ = binary.Write(&output, binary.LittleEndian, uint32(0))
	output.WriteString("WAVE")
	for offset := 12; offset+8 <= len(data); {
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		chunkStart := offset + 8
		chunkEnd := chunkStart + int(chunkSize)
		if chunkEnd > len(data) {
			return nil, errors.New("el archivo WAV contiene un bloque inválido")
		}
		chunkID := string(data[offset : offset+4])
		writeChunkSize := chunkSize
		if chunkID == "data" {
			if chunkSize > math.MaxUint32/2 {
				return nil, errors.New("la duración WAV supera el límite admitido")
			}
			writeChunkSize *= 2
		}
		output.WriteString(chunkID)
		_ = binary.Write(&output, binary.LittleEndian, writeChunkSize)
		switch chunkID {
		case "fmt ":
			chunk := append([]byte(nil), data[chunkStart:chunkEnd]...)
			binary.LittleEndian.PutUint32(chunk[8:12], 16000)
			binary.LittleEndian.PutUint16(chunk[12:14], 2)
			binary.LittleEndian.PutUint16(chunk[14:16], 16)
			output.Write(chunk)
		case "data":
			for _, sample := range data[chunkStart:chunkEnd] {
				converted := uint16(int16((int(sample) - 128) << 8))
				output.WriteByte(byte(converted))
				output.WriteByte(byte(converted >> 8))
			}
		default:
			output.Write(data[chunkStart:chunkEnd])
		}
		if writeChunkSize&1 != 0 {
			output.WriteByte(0)
		}
		offset = chunkEnd + int(chunkSize&1)
	}
	if output.Len()-8 > math.MaxUint32 {
		return nil, errors.New("la duración WAV supera el límite admitido")
	}
	binary.LittleEndian.PutUint32(output.Bytes()[4:8], uint32(output.Len()-8))
	return output.Bytes(), nil
}

func ListAudioTracks(c echo.Context) error {
	if config.App.DB == nil {
		return audioAPIError(c, http.StatusServiceUnavailable, "La base de datos no está disponible.")
	}
	rows, err := config.App.DB.QueryContext(c.Request().Context(), `
		SELECT t.id, t.name, t.original_filename, t.playback_mode, t.enabled, t.file_size,
			t.sample_rate, t.bits_per_sample, t.duration_ms, t.created_at,
			COALESCE(s.interval_minutes, 0), s.last_played_at, s.next_play_at, t.stored_filename, t.start_ms, t.end_ms
		FROM audio_tracks t
		LEFT JOIN audio_broadcast_schedules s ON s.audio_id = t.id
		ORDER BY t.created_at DESC, t.id DESC`)
	if err != nil {
		log.Printf("audio: no se pudo consultar la biblioteca: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo consultar la biblioteca de audio.")
	}
	defer rows.Close()
	tracks := make([]storedAudioTrack, 0)
	for rows.Next() {
		var track storedAudioTrack
		var enabled int
		var createdAt string
		var lastPlayed, nextPlay sql.NullString
		if err := rows.Scan(
			&track.ID, &track.Name, &track.OriginalName, &track.PlaybackMode, &enabled, &track.FileSize,
			&track.SampleRate, &track.BitsPerSample, &track.DurationMS, &createdAt, &track.IntervalMinutes,
			&lastPlayed, &nextPlay, &track.StoredFilename, &track.StartMS, &track.EndMS,
		); err != nil {
			log.Printf("audio: no se pudo interpretar un registro: %v", err)
			return audioAPIError(c, http.StatusInternalServerError, "No se pudo leer la biblioteca de audio.")
		}
		track.Enabled = enabled != 0
		track.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			track.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		}
		if err != nil {
			log.Printf("audio: fecha de creación inválida para registro %d: %v", track.ID, err)
			return audioAPIError(c, http.StatusInternalServerError, "No se pudo leer la biblioteca de audio.")
		}
		track.LastPlayedAt = parseAudioTimestamp(lastPlayed)
		track.NextPlayAt = parseAudioTimestamp(nextPlay)
		tracks = append(tracks, track)
	}
	if err := rows.Err(); err != nil {
		log.Printf("audio: error al recorrer la biblioteca: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo consultar la biblioteca de audio.")
	}
	publicTracks := make([]AudioTrack, 0, len(tracks))
	for _, track := range tracks {
		publicTracks = append(publicTracks, track.AudioTrack)
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "data": publicTracks})
}

func UploadAudioTrack(c echo.Context) error {
	if config.App.DB == nil {
		return audioAPIError(c, http.StatusServiceUnavailable, "La base de datos no está disponible.")
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, audioMaxUploadBytes+(1<<20))
	if err := c.Request().ParseMultipartForm(1 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			return audioAPIError(c, http.StatusRequestEntityTooLarge, "El archivo no puede superar 25 MiB.")
		}
		return audioAPIError(c, http.StatusBadRequest, "La solicitud de audio no es válida.")
	}
	defer c.Request().MultipartForm.RemoveAll()
	name := strings.TrimSpace(c.FormValue("name"))
	mode := strings.TrimSpace(c.FormValue("playback_mode"))
	if !audioTrackNamePattern.MatchString(name) || !validAudioPlaybackMode(mode) {
		return audioAPIError(c, http.StatusBadRequest, "Indica un nombre válido y una modalidad de reproducción.")
	}
	intervalMinutes := 0
	if mode == "broadcast_silence" {
		value, err := strconv.Atoi(c.FormValue("interval_minutes"))
		if err != nil || value < 1 || value > audioMaxInterval {
			return audioAPIError(c, http.StatusBadRequest, "El intervalo debe estar entre 1 y 10080 minutos.")
		}
		intervalMinutes = value
	}
	file, header, err := c.Request().FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "code": "AUDIO_ERROR_AUDIO_REQUIRED"})
	}
	defer file.Close()
	extension := strings.ToLower(filepath.Ext(filepath.Base(header.Filename)))
	if _, ok := audioInputFormats[extension]; !ok {
		return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "code": "AUDIO_ERROR_INPUT_FORMAT"})
	}
	if header.Size > audioMaxUploadBytes {
		return audioAPIError(c, http.StatusRequestEntityTooLarge, "El archivo no puede superar 25 MiB.")
	}
	data, err := io.ReadAll(io.LimitReader(file, audioMaxUploadBytes+1))
	if err != nil {
		return audioAPIError(c, http.StatusBadRequest, "No se pudo leer el archivo de audio.")
	}
	if len(data) == 0 || len(data) > audioMaxUploadBytes {
		return audioAPIError(c, http.StatusRequestEntityTooLarge, "El archivo está vacío o supera 25 MiB.")
	}
	data, duration, sampleRate, bitsPerSample, err := convertUploadedAudio(c.Request().Context(), data, extension)
	if err != nil {
		var conversion *audioConversionError
		if errors.As(err, &conversion) {
			log.Printf("audio: %s: %v", conversion.code, err)
			status := http.StatusBadRequest
			if conversion.code == "AUDIO_ERROR_CONVERTER_MISSING" || conversion.code == "AUDIO_ERROR_CONVERSION_BUSY" {
				status = http.StatusServiceUnavailable
			}
			return c.JSON(status, echo.Map{"status": "error", "code": conversion.code})
		}
		log.Printf("audio: conversion failed: %v", err)
		return audioAPIError(c, http.StatusBadRequest, "No se pudo leer el archivo de audio.")
	}

	originalName := filepath.Base(header.Filename)
	if len(originalName) > 200 || strings.ContainsAny(originalName, "\r\n\x00") {
		return audioAPIError(c, http.StatusBadRequest, "El nombre del archivo no es válido.")
	}
	return persistAudioTrack(c, name, originalName, mode, intervalMinutes, data, duration, sampleRate, bitsPerSample, 0)
}

func persistAudioTrack(c echo.Context, name, originalName, mode string, intervalMinutes int, data []byte, duration time.Duration, sampleRate, bitsPerSample int, recordingID int64) error {
	filename, err := newAudioFilename()
	if err != nil {
		log.Printf("audio: no se pudo crear identificador de archivo: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el audio.")
	}
	dir := audioStoragePath()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Printf("audio: no se pudo crear %s: %v", dir, err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo preparar el directorio de audio.")
	}
	temporary, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		log.Printf("audio: no se pudo preparar archivo temporal: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el audio.")
	}
	tempName := temporary.Name()
	cleanup := func() { _ = os.Remove(tempName) }
	if err := temporary.Chmod(0o640); err != nil {
		temporary.Close()
		cleanup()
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el audio.")
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		cleanup()
		log.Printf("audio: escritura fallida: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el audio.")
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		cleanup()
		log.Printf("audio: sincronización fallida: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el audio.")
	}
	if err := temporary.Close(); err != nil {
		cleanup()
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el audio.")
	}
	destination := filepath.Join(dir, filename)
	if err := os.Rename(tempName, destination); err != nil {
		cleanup()
		log.Printf("audio: no se pudo instalar el archivo: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el audio.")
	}
	removeFile := true
	defer func() {
		if removeFile {
			if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
				log.Printf("audio: no se pudo limpiar archivo tras error %s: %v", filename, err)
			}
		}
	}()

	now := time.Now().UTC()
	tx, err := config.App.DB.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el registro de audio.")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(c.Request().Context(), `
		INSERT INTO audio_tracks
		(name, stored_filename, original_filename, playback_mode, enabled, file_size, sample_rate, bits_per_sample, duration_ms, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)`,
		name, filename, originalName, mode, len(data), sampleRate, bitsPerSample, duration.Milliseconds(), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		log.Printf("audio: no se pudo guardar pista: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el registro de audio.")
	}
	id, err := result.LastInsertId()
	if err != nil {
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el registro de audio.")
	}
	if mode == "broadcast_silence" {
		next := now.Add(time.Duration(intervalMinutes) * time.Minute)
		if _, err := tx.ExecContext(c.Request().Context(), `
			INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at) VALUES (?, ?, ?)`,
			id, intervalMinutes, next.Format(time.RFC3339Nano)); err != nil {
			log.Printf("audio: no se pudo crear programación %d: %v", id, err)
			return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar la programación.")
		}
	}
	if recordingID != 0 {
		if _, err := tx.ExecContext(c.Request().Context(),
			`UPDATE audio_recordings SET library_track_id = ? WHERE id = ?`, id, recordingID); err != nil {
			log.Printf("audio: no se pudo asociar grabación %d: %v", recordingID, err)
			return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el registro de audio.")
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("audio: no se pudo confirmar registro: %v", err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo guardar el registro de audio.")
	}
	removeFile = false
	return c.JSON(http.StatusCreated, echo.Map{"status": "success", "data": AudioTrack{
		ID: id, Name: name, OriginalName: originalName, PlaybackMode: mode, Enabled: true,
		FileSize: int64(len(data)), DurationMS: duration.Milliseconds(),
		SampleRate: sampleRate, BitsPerSample: bitsPerSample, IntervalMinutes: intervalMinutes, CreatedAt: now,
	}})
}

func UpdateAudioTrack(c echo.Context) error {
	if config.App.DB == nil {
		return audioAPIError(c, http.StatusServiceUnavailable, "La base de datos no está disponible.")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return audioAPIError(c, http.StatusBadRequest, "El identificador de audio no es válido.")
	}
	var request struct {
		Name            *string `json:"name"`
		PlaybackMode    *string `json:"playback_mode"`
		Enabled         *bool   `json:"enabled"`
		IntervalMinutes *int    `json:"interval_minutes"`
		StartMS         *int64  `json:"start_ms"`
		EndMS           *int64  `json:"end_ms"`
	}
	if err := c.Bind(&request); err != nil {
		return audioAPIError(c, http.StatusBadRequest, "Solicitud inválida.")
	}
	var trackName, mode string
	var enabled int
	var oldInterval int
	var startMS, endMS, durationMS int64
	err = config.App.DB.QueryRowContext(c.Request().Context(), `
		SELECT t.name, t.playback_mode, t.enabled, COALESCE(s.interval_minutes, 0), t.start_ms, t.end_ms, t.duration_ms
		FROM audio_tracks t LEFT JOIN audio_broadcast_schedules s ON s.audio_id = t.id WHERE t.id = ?`, id).
		Scan(&trackName, &mode, &enabled, &oldInterval, &startMS, &endMS, &durationMS)
	if errors.Is(err, sql.ErrNoRows) {
		return audioAPIError(c, http.StatusNotFound, "El audio no existe.")
	}
	if err != nil {
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo leer el audio.")
	}
	if request.StartMS != nil || request.EndMS != nil {
		if request.StartMS == nil || request.EndMS == nil ||
			!validAudioSegment(*request.StartMS, *request.EndMS, durationMS) {
			return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "code": "AUDIO_ERROR_SEGMENT"})
		}
		startMS, endMS = *request.StartMS, *request.EndMS
	}
	if request.Name != nil {
		trackName = strings.TrimSpace(*request.Name)
		if !audioTrackNamePattern.MatchString(trackName) {
			return audioAPIError(c, http.StatusBadRequest, "El nombre de audio no es válido.")
		}
	}
	if request.PlaybackMode != nil {
		mode = strings.TrimSpace(*request.PlaybackMode)
		if !validAudioPlaybackMode(mode) {
			return audioAPIError(c, http.StatusBadRequest, "La modalidad de reproducción no es válida.")
		}
	}
	if request.Enabled != nil {
		if *request.Enabled {
			enabled = 1
		} else {
			enabled = 0
		}
	}
	interval := oldInterval
	if request.IntervalMinutes != nil {
		interval = *request.IntervalMinutes
	}
	if mode == "broadcast_silence" {
		if interval == 0 {
			interval = 60
		}
		if interval < 1 || interval > audioMaxInterval {
			return audioAPIError(c, http.StatusBadRequest, "El intervalo debe estar entre 1 y 10080 minutos.")
		}
	}

	now := time.Now().UTC()
	tx, err := config.App.DB.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo actualizar el audio.")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(c.Request().Context(), `
		UPDATE audio_tracks SET name = ?, playback_mode = ?, enabled = ?, updated_at = ?, start_ms = ?, end_ms = ? WHERE id = ?`,
		trackName, mode, enabled, now.Format(time.RFC3339Nano), startMS, endMS, id); err != nil {
		log.Printf("audio: no se pudo actualizar pista %d: %v", id, err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo actualizar el audio.")
	}
	if mode == "broadcast_silence" {
		if _, err := tx.ExecContext(c.Request().Context(), `
			INSERT INTO audio_broadcast_schedules (audio_id, interval_minutes, next_play_at)
			VALUES (?, ?, ?)
			ON CONFLICT(audio_id) DO UPDATE SET
				interval_minutes = excluded.interval_minutes,
				next_play_at = CASE
					WHEN audio_broadcast_schedules.interval_minutes != excluded.interval_minutes THEN excluded.next_play_at
					ELSE audio_broadcast_schedules.next_play_at
				END`,
			id, interval, now.Add(time.Duration(interval)*time.Minute).Format(time.RFC3339Nano)); err != nil {
			return audioAPIError(c, http.StatusInternalServerError, "No se pudo actualizar la programación.")
		}
	} else if _, err := tx.ExecContext(c.Request().Context(), `DELETE FROM audio_broadcast_schedules WHERE audio_id = ?`, id); err != nil {
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo actualizar la programación.")
	}
	if err := tx.Commit(); err != nil {
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo actualizar el audio.")
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success"})
}

func DeleteAudioTrack(c echo.Context) error {
	if config.App.DB == nil {
		return audioAPIError(c, http.StatusServiceUnavailable, "La base de datos no está disponible.")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return audioAPIError(c, http.StatusBadRequest, "El identificador de audio no es válido.")
	}
	var filename string
	if err := config.App.DB.QueryRowContext(c.Request().Context(), `SELECT stored_filename FROM audio_tracks WHERE id = ?`, id).Scan(&filename); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return audioAPIError(c, http.StatusNotFound, "El audio no existe.")
		}
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo leer el audio.")
	}
	if !audioFilenamePattern.MatchString(filename) {
		log.Printf("audio: nombre de archivo almacenado inválido para pista %d", id)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo eliminar el audio.")
	}
	result, err := config.App.DB.ExecContext(c.Request().Context(), `DELETE FROM audio_tracks WHERE id = ?`, id)
	if err != nil {
		log.Printf("audio: no se pudo eliminar pista %d: %v", id, err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo eliminar el audio.")
	}
	deleted, err := result.RowsAffected()
	if err != nil || deleted != 1 {
		return audioAPIError(c, http.StatusNotFound, "El audio no existe.")
	}
	paths := []string{filepath.Join(audioStoragePath(), filename), filepath.Join(legacyAudioStoragePath(), filename)}
	for i, path := range paths {
		if i == 1 && path == paths[0] {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("audio: registro %d eliminado pero no se pudo quitar el archivo %s: %v", id, path, err)
			return audioAPIError(c, http.StatusInternalServerError, "El registro se eliminó, pero no se pudo borrar el archivo de audio.")
		}
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success"})
}

func AudioTrackFile(c echo.Context) error {
	if config.App.DB == nil {
		return audioAPIError(c, http.StatusServiceUnavailable, "La base de datos no está disponible.")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return audioAPIError(c, http.StatusBadRequest, "El identificador de audio no es válido.")
	}
	track, err := loadAudioTrack(c.Request().Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return audioAPIError(c, http.StatusNotFound, "El audio no existe.")
	}
	if err != nil {
		log.Printf("audio: preview track %d: %v", id, err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo leer el audio.")
	}
	if !audioFilenamePattern.MatchString(track.StoredFilename) {
		log.Printf("audio: invalid preview filename for track %d", id)
		return audioAPIError(c, http.StatusInternalServerError, "El nombre del archivo no es válido.")
	}
	file, err := os.Open(filepath.Join(audioStoragePath(), track.StoredFilename))
	if os.IsNotExist(err) {
		file, err = os.Open(filepath.Join(legacyAudioStoragePath(), track.StoredFilename))
	}
	if err != nil {
		log.Printf("audio: preview file for track %d: %v", id, err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo leer el archivo de audio.")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > audioMaxStoredBytes {
		log.Printf("audio: invalid preview file for track %d: %v", id, err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo leer el archivo de audio.")
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Content-Type", "audio/wav")
	http.ServeContent(c.Response(), c.Request(), track.StoredFilename, info.ModTime(), file)
	return nil
}

func PlayAudioTrack(c echo.Context) error {
	if config.App.DB == nil {
		return audioAPIError(c, http.StatusServiceUnavailable, "La base de datos no está disponible.")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return audioAPIError(c, http.StatusBadRequest, "El identificador de audio no es válido.")
	}
	track, err := loadAudioTrack(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return audioAPIError(c, http.StatusNotFound, "El audio no existe.")
		}
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo leer el audio.")
	}
	if !track.Enabled {
		return audioAPIError(c, http.StatusConflict, "El audio está desactivado.")
	}
	if track.PlaybackMode != "broadcast_once" {
		return audioAPIError(c, http.StatusConflict, "La reproducción manual solo está disponible para audios de emisión única.")
	}
	var segment struct {
		StartMS *int64 `json:"start_ms"`
		EndMS   *int64 `json:"end_ms"`
	}
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&segment); err != nil {
			return c.JSON(400, echo.Map{"status": "error", "code": "AUDIO_ERROR_SEGMENT"})
		}
	}
	if segment.StartMS != nil || segment.EndMS != nil {
		if segment.StartMS == nil || segment.EndMS == nil ||
			!validAudioSegment(*segment.StartMS, *segment.EndMS, track.DurationMS) {
			return c.JSON(400, echo.Map{"status": "error", "code": "AUDIO_ERROR_SEGMENT"})
		}
		track.StartMS, track.EndMS = *segment.StartMS, *segment.EndMS
	}
	started := time.Now()
	token, err := claimAudioPlayback(c.Request().Context(), track.playbackDurationMS())
	if errors.Is(err, errAudioPlaybackBusy) {
		return audioAPIError(c, http.StatusConflict, "Otro audio está reproduciéndose. Inténtalo cuando termine.")
	}
	if err != nil {
		log.Printf("audio: no se pudo reservar reproducción para pista %d: %v", id, err)
		return audioAPIError(c, http.StatusInternalServerError, "No se pudo reservar la reproducción.")
	}
	filename, err := prepareAudioTrackPlayback(track)
	if err != nil {
		log.Printf("audio: segment creation failed: %v", err)
		if releaseErr := releaseAudioPlayback(c.Request().Context(), token); releaseErr != nil {
			log.Printf("audio: segment reservation release failed: %v", releaseErr)
		}
		return c.JSON(500, echo.Map{"status": "error", "code": "AUDIO_ERROR_SEGMENT"})
	}
	command, localNode, amiOutput, err := playAudioOnPrimary(filename)
	if err != nil {
		if releaseErr := releaseAudioPlayback(c.Request().Context(), token); releaseErr != nil {
			log.Printf("audio: no se pudo liberar reproducción fallida para pista %d: %v", id, releaseErr)
		}
		recordAsteriskOperation(c.Request().Context(), "audio_broadcast", localNode, "", track.PlaybackMode, false, command, "ami_error", err.Error(), started)
		log.Printf("audio: emisión única falló para pista %d: %v", id, err)
		if errors.Is(err, errAudioDuplexRequired) {
			return audioAPIError(c, http.StatusConflict, "El nodo principal necesita un valor duplex entre 1 y 4 en rpt.conf para emitir audio a los nodos enlazados. Ajusta la sección del nodo y recarga app_rpt.")
		}
		return audioAPIError(c, http.StatusBadGateway, "No se pudo iniciar la reproducción del audio.")
	}
	details := fmt.Sprintf("Audio %q enviado a Asterisk", track.Name)
	if track.EndMS != 0 {
		details += fmt.Sprintf(" [%d-%d ms]", track.StartMS, track.EndMS)
	}
	if amiOutput != "" {
		details += ": " + amiOutput
	}
	recordAsteriskOperation(c.Request().Context(), "audio_broadcast", localNode, "", track.PlaybackMode, false, command, "success", details, started)
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "code": "AUDIO_PLAY_REQUESTED"})
}

func claimAudioPlayback(ctx context.Context, durationMS int64) (string, error) {
	if config.App.DB == nil {
		return "", errors.New("base de datos no disponible")
	}
	if durationMS < 1 {
		return "", errors.New("duración de audio inválida")
	}
	tx, err := config.App.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	token, err := claimAudioPlaybackTx(ctx, tx, durationMS)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func claimAudioPlaybackTx(ctx context.Context, tx *sql.Tx, durationMS int64) (string, error) {
	if durationMS < 1 {
		return "", errors.New("duración de audio inválida")
	}
	var busyUntil string
	if err := tx.QueryRowContext(ctx, `SELECT busy_until FROM audio_playback_state WHERE id = 1`).Scan(&busyUntil); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	until, err := time.Parse(time.RFC3339Nano, busyUntil)
	if err != nil {
		return "", fmt.Errorf("estado de reproducción inválido: %w", err)
	}
	if until.After(now) {
		return "", errAudioPlaybackBusy
	}
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(tokenBytes[:])
	busyFor := time.Duration(durationMS)*time.Millisecond + 2*time.Second
	if _, err := tx.ExecContext(ctx, `
		UPDATE audio_playback_state SET busy_until = ?, playback_token = ? WHERE id = 1`,
		now.Add(busyFor).Format(time.RFC3339Nano), token); err != nil {
		return "", err
	}
	return token, nil
}

func releaseAudioPlayback(ctx context.Context, token string) error {
	_, err := config.App.DB.ExecContext(ctx, `
		UPDATE audio_playback_state SET busy_until = '1970-01-01T00:00:00Z', playback_token = ''
		WHERE id = 1 AND playback_token = ?`, token)
	return err
}

func loadAudioTrack(ctx context.Context, id int64) (storedAudioTrack, error) {
	var track storedAudioTrack
	var enabled int
	var createdAt string
	err := config.App.DB.QueryRowContext(ctx, `
		SELECT t.id, t.name, t.original_filename, t.playback_mode, t.enabled, t.file_size,
			t.sample_rate, t.bits_per_sample, t.duration_ms, t.created_at, t.stored_filename,
			COALESCE(s.interval_minutes, 0), t.start_ms, t.end_ms
		FROM audio_tracks t LEFT JOIN audio_broadcast_schedules s ON s.audio_id = t.id WHERE t.id = ?`, id).
		Scan(&track.ID, &track.Name, &track.OriginalName, &track.PlaybackMode, &enabled, &track.FileSize,
			&track.SampleRate, &track.BitsPerSample, &track.DurationMS, &createdAt, &track.StoredFilename, &track.IntervalMinutes,
			&track.StartMS, &track.EndMS)
	if err != nil {
		return storedAudioTrack{}, err
	}
	track.Enabled = enabled != 0
	track.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		track.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
	}
	return track, err
}

func playAudioOnPrimary(filename string) (string, string, string, error) {
	if !audioFilenamePattern.MatchString(filename) {
		return "", "", "", errors.New("el nombre del archivo de audio almacenado no es válido")
	}
	primary, err := newPrimaryNodeControl().primary()
	if err != nil {
		return "", "", "", fmt.Errorf("no se pudo identificar el nodo principal: %w", err)
	}
	rptPath := newPrimaryNodeControl().rptPath
	duplex, found, err := readRPTSectionSetting(rptPath, primary, "duplex")
	if err != nil {
		return "", primary, "", fmt.Errorf("no se pudo comprobar duplex del nodo principal: %w", err)
	}
	if !audioDuplexAllowsLinkedPlayback(duplex, found) {
		return "", primary, "", errAudioDuplexRequired
	}
	if err := prepareAudioFileForPlayback(filename); err != nil {
		return "", primary, "", fmt.Errorf("no se pudo preparar el archivo para Asterisk: %w", err)
	}
	instance, err := resolveAudioPrimaryInstance(primary)
	if err != nil {
		return "", primary, "", fmt.Errorf("no se pudo resolver el nodo principal para audio: %w", err)
	}
	command := fmt.Sprintf("rpt playback %s %s/%s", primary, audioSubdirectory, strings.TrimSuffix(filename, filepath.Ext(filename)))
	output, err := config.ExecuteAMICommand(instance.Host, instance.Port, instance.User, instance.Secret, command, 5*time.Second)
	if err != nil {
		return command, primary, "", fmt.Errorf("Asterisk no pudo iniciar la reproducción: %w", err)
	}
	output = strings.Join(config.ParseAMIOutputToLines(output), " | ")
	if err := audioPlaybackOutputError(output); err != nil {
		return command, primary, output, err
	}
	return command, primary, output, nil
}

func resolveAudioPrimaryInstance(primary string) (allmonInstance, error) {
	instances, err := allStarLinkActivityInstances()
	if err != nil {
		return allmonInstance{}, err
	}
	for _, instance := range instances {
		if instance.Node != primary {
			continue
		}
		if strings.TrimSpace(instance.Host) == "" || strings.TrimSpace(instance.Port) == "" ||
			strings.TrimSpace(instance.User) == "" || strings.TrimSpace(instance.Secret) == "" {
			return allmonInstance{}, errors.New("la configuración AMI del nodo principal está incompleta")
		}
		return instance, nil
	}
	return allmonInstance{}, errors.New("el nodo principal no está disponible en la configuración de nodos monitorizados")
}

func audioDuplexAllowsLinkedPlayback(value string, found bool) bool {
	duplex, err := strconv.Atoi(value)
	return found && err == nil && duplex >= 1 && duplex <= 4
}

func audioPlaybackOutputError(output string) error {
	lower := strings.ToLower(output)
	for _, failure := range []string{
		"no such command",
		"no such node",
		"unable to open",
		"file not found",
		"no such file",
		"does not exist",
	} {
		if strings.Contains(lower, failure) {
			return fmt.Errorf("respuesta de app_rpt: %s", output)
		}
	}
	return nil
}

func readRPTSectionSetting(path, sectionName, settingName string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	type sectionConfig struct {
		parents []string
		values  map[string]string
	}
	sections := make(map[string]*sectionConfig)
	currentSection := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if header := rptAudioSectionPattern.FindStringSubmatch(line); header != nil {
			currentSection = strings.TrimSpace(header[1])
			section := sections[currentSection]
			if section == nil {
				section = &sectionConfig{values: make(map[string]string)}
				sections[currentSection] = section
			}
			for _, parent := range strings.Split(header[2], ",") {
				parent = strings.TrimSpace(strings.TrimSuffix(parent, "!"))
				if parent != "" {
					section.parents = append(section.parents, parent)
				}
			}
			continue
		}
		if currentSection == "" {
			continue
		}
		line, _, _ = strings.Cut(line, ";")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		sections[currentSection].values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return "", false, err
	}

	visited := make(map[string]bool)
	var lookup func(string) (string, bool)
	lookup = func(name string) (string, bool) {
		if visited[name] {
			return "", false
		}
		visited[name] = true
		defer delete(visited, name)
		section := sections[name]
		if section == nil {
			return "", false
		}
		value, found := "", false
		for _, parent := range section.parents {
			if parentValue, parentFound := lookup(parent); parentFound {
				value, found = parentValue, true
			}
		}
		if ownValue, ownFound := section.values[strings.ToLower(settingName)]; ownFound {
			return ownValue, true
		}
		return value, found
	}
	value, found := lookup(sectionName)
	return value, found, nil
}

func prepareAudioFileForPlayback(filename string) error {
	path := filepath.Join(audioStoragePath(), filename)
	info, err := os.Stat(path)
	sourcePath := path
	if os.IsNotExist(err) {
		legacyPath := filepath.Join(legacyAudioStoragePath(), filename)
		if legacyPath != path {
			sourcePath = legacyPath
			info, err = os.Stat(sourcePath)
		}
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > audioMaxStoredBytes {
		return errors.New("el archivo de audio almacenado no es regular o supera el límite admitido")
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	normalized, duration, sampleRate, bitsPerSample, err := normalizeAudioWAV(data)
	if err != nil {
		return err
	}
	if sourcePath != path || !bytes.Equal(data, normalized) {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		temporary, err := os.CreateTemp(filepath.Dir(path), ".normalize-*")
		if err != nil {
			return err
		}
		tempPath := temporary.Name()
		defer os.Remove(tempPath)
		if err := temporary.Chmod(info.Mode().Perm()); err != nil {
			temporary.Close()
			return err
		}
		if _, err := temporary.Write(normalized); err != nil {
			temporary.Close()
			return err
		}
		if err := temporary.Sync(); err != nil {
			temporary.Close()
			return err
		}
		if err := temporary.Close(); err != nil {
			return err
		}
		if err := os.Rename(tempPath, path); err != nil {
			return err
		}
	}
	if config.App.DB != nil {
		_, err = config.App.DB.Exec(`
			UPDATE audio_tracks SET file_size = ?, sample_rate = ?, bits_per_sample = ?, duration_ms = ?, updated_at = ?
			WHERE stored_filename = ?`,
			len(normalized), sampleRate, bitsPerSample, duration.Milliseconds(), time.Now().UTC().Format(time.RFC3339Nano), filename)
	}
	return err
}

func newAudioFilename() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x.wav", random[0:4], random[4:6], random[6:8], random[8:10], random[10:]), nil
}

func parseAudioTimestamp(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		parsed, err = time.Parse("2006-01-02 15:04:05", value.String)
	}
	if err != nil {
		return nil
	}
	return &parsed
}

func audioAPIError(c echo.Context, status int, message string) error {
	code, ok := audioErrorCodeByMessage[message]
	if !ok {
		code = "AUDIO_ERROR_UNKNOWN"
	}
	return c.JSON(status, echo.Map{"status": "error", "code": code, "message": message})
}

var audioErrorCodeByMessage = map[string]string{
	"La base de datos no está disponible.":                                      "AUDIO_ERROR_DB_UNAVAILABLE",
	"No se pudo consultar la biblioteca de audio.":                              "AUDIO_ERROR_LIST",
	"No se pudo leer la biblioteca de audio.":                                   "AUDIO_ERROR_LIBRARY_READ",
	"El archivo no puede superar 25 MiB.":                                       "AUDIO_ERROR_FILE_TOO_LARGE",
	"La solicitud de audio no es válida.":                                       "AUDIO_ERROR_REQUEST_INVALID",
	"Indica un nombre válido y una modalidad de reproducción.":                  "AUDIO_ERROR_UPLOAD_FIELDS",
	"El intervalo debe estar entre 1 y 10080 minutos.":                          "AUDIO_ERROR_INTERVAL",
	"Selecciona un archivo WAV.":                                                "AUDIO_ERROR_FILE_REQUIRED",
	"El archivo debe tener extensión .wav.":                                     "AUDIO_ERROR_FILE_EXTENSION",
	"No se pudo leer el archivo de audio.":                                      "AUDIO_ERROR_FILE_READ",
	"El archivo está vacío o supera 25 MiB.":                                    "AUDIO_ERROR_FILE_SIZE",
	"el archivo no es un WAV válido":                                            "AUDIO_ERROR_WAV_INVALID",
	"el archivo WAV está incompleto":                                            "AUDIO_ERROR_WAV_INCOMPLETE",
	"el archivo WAV contiene un bloque inválido":                                "AUDIO_ERROR_WAV_CHUNK",
	"el bloque de formato WAV no es válido":                                     "AUDIO_ERROR_WAV_HEADER",
	"el audio debe ser PCM WAV mono, 8 o 16 bits, 8000 Hz":                      "AUDIO_ERROR_WAV_FORMAT",
	"la duración WAV supera el límite admitido":                                 "AUDIO_ERROR_WAV_DURATION",
	"El nombre del archivo no es válido.":                                       "AUDIO_ERROR_FILENAME",
	"No se pudo guardar el audio.":                                              "AUDIO_ERROR_SAVE",
	"No se pudo preparar el directorio de audio.":                               "AUDIO_ERROR_STORAGE",
	"No se pudo guardar el registro de audio.":                                  "AUDIO_ERROR_SAVE_RECORD",
	"No se pudo guardar la programación.":                                       "AUDIO_ERROR_SCHEDULE_SAVE",
	"El identificador de audio no es válido.":                                   "AUDIO_ERROR_ID",
	"Solicitud inválida.":                                                       "AUDIO_ERROR_REQUEST_INVALID",
	"El audio no existe.":                                                       "AUDIO_ERROR_NOT_FOUND",
	"No se pudo leer el audio.":                                                 "AUDIO_ERROR_TRACK_READ",
	"El nombre de audio no es válido.":                                          "AUDIO_ERROR_NAME",
	"La modalidad de reproducción no es válida.":                                "AUDIO_ERROR_MODE",
	"No se pudo actualizar el audio.":                                           "AUDIO_ERROR_UPDATE",
	"No se pudo actualizar la programación.":                                    "AUDIO_ERROR_SCHEDULE_UPDATE",
	"No se pudo eliminar el audio.":                                             "AUDIO_ERROR_DELETE",
	"El registro se eliminó, pero no se pudo borrar el archivo de audio.":       "AUDIO_ERROR_DELETE_FILE",
	"El audio está desactivado.":                                                "AUDIO_ERROR_DISABLED",
	"La reproducción manual solo está disponible para audios de emisión única.": "AUDIO_ERROR_MANUAL_MODE",
	"Otro audio está reproduciéndose. Inténtalo cuando termine.":                "AUDIO_ERROR_PLAYBACK_BUSY",
	"No se pudo reservar la reproducción.":                                      "AUDIO_ERROR_RESERVE",
	"No se pudo iniciar la reproducción del audio.":                             "AUDIO_ERROR_PLAY",
	"El nodo principal necesita un valor duplex entre 1 y 4 en rpt.conf para emitir audio a los nodos enlazados. Ajusta la sección del nodo y recarga app_rpt.": "AUDIO_ERROR_DUPLEX",
}
