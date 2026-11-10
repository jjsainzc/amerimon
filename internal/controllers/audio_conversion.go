package controllers

// Este archivo implementa la validación y conversión de archivos de audio.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var audioInputFormats = map[string]string{
	".wav": "wav", ".mp3": "mp3", ".flac": "flac", ".ogg": "ogg", ".oga": "ogg", ".opus": "ogg",
	".m4a": "mov", ".mp4": "mov", ".aac": "aac", ".wma": "asf", ".aif": "aiff", ".aiff": "aiff",
	".webm": "matroska", ".mka": "matroska", ".amr": "amr", ".ape": "ape",
}

var audioConversionSlot = make(chan struct{}, 1)

// audioConversionError representa un error ocurrido durante la conversión de audio, incluyendo un código de error y el error subyacente.
type audioConversionError struct {
	code string
	err  error
}

// audioDiagnostic captura la salida de diagnóstico de FFmpeg, limitando la cantidad de datos almacenados para evitar un uso excesivo de memoria.
func (e *audioConversionError) Error() string { return e.err.Error() }

// Error devuelve la descripción del error subyacente.
// Unwrap devuelve el error subyacente para permitir el encadenamiento de errores.
func (e *audioConversionError) Unwrap() error { return e.err }

// audioDiagnostic representa un búfer limitado para capturar la salida de diagnóstico de FFmpeg.
type audioDiagnostic struct{ bytes.Buffer }

// audioDiagnostic captura la salida de diagnóstico de FFmpeg, limitando la cantidad de datos almacenados para evitar un uso excesivo de memoria.
func (b *audioDiagnostic) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 4096 - b.Len(); remaining > 0 {
		b.Buffer.Write(p[:min(len(p), remaining)])
	}
	return n, nil
}

// convertUploadedAudio convierte un archivo de audio subido a WAV con las especificaciones esperadas, manejando errores y limitaciones de tamaño.
// convertUploadedAudio convierte un archivo de audio subido a WAV con las especificaciones esperadas, manejando errores y limitaciones de tamaño.
func convertUploadedAudio(ctx context.Context, data []byte, extension string) ([]byte, time.Duration, int, int, error) {
	if extension == ".wav" {
		normalized, duration, rate, bits, err := normalizeAudioWAV(data)
		if err == nil {
			if len(normalized) >= audioMaxUploadBytes {
				return nil, 0, 0, 0, &audioConversionError{
					code: "AUDIO_ERROR_CONVERTED_SIZE", err: errors.New("normalized audio reaches 25 MiB limit"),
				}
			}
			return normalized, duration, rate, bits, nil
		}
	}
	fail := func(code string, err error) ([]byte, time.Duration, int, int, error) {
		return nil, 0, 0, 0, &audioConversionError{code: code, err: err}
	}
	format, ok := audioInputFormats[extension]
	if !ok {
		return fail("AUDIO_ERROR_INPUT_FORMAT", errors.New("unsupported audio extension"))
	}
	select {
	case audioConversionSlot <- struct{}{}:
		defer func() { <-audioConversionSlot }()
	default:
		return fail("AUDIO_ERROR_CONVERSION_BUSY", errors.New("another audio conversion is running"))
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fail("AUDIO_ERROR_CONVERTER_MISSING", err)
	}
	dir, err := os.MkdirTemp("", "amerimon-audio-*")
	if err != nil {
		return fail("AUDIO_ERROR_CONVERSION", err)
	}
	defer os.RemoveAll(dir)
	input := filepath.Join(dir, "input")
	output := filepath.Join(dir, "output.wav")
	if err := os.WriteFile(input, data, 0o600); err != nil {
		return fail("AUDIO_ERROR_CONVERSION", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
		"-threads", "1", "-protocol_whitelist", "file", "-f", format, "-i", input,
		"-map", "0:a:0", "-vn", "-sn", "-dn", "-map_metadata", "-1",
		"-threads", "1", "-ac", "1", "-ar", "8000", "-c:a", "pcm_s16le",
		"-fs", strconv.Itoa(audioMaxUploadBytes+1), "-f", "wav", output)
	var diagnostic audioDiagnostic
	cmd.Stderr = &diagnostic
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fail("AUDIO_ERROR_CONVERSION_TIMEOUT", ctx.Err())
		}
		return fail("AUDIO_ERROR_CONVERSION", fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(diagnostic.String())))
	}
	info, err := os.Stat(output)
	if err != nil {
		return fail("AUDIO_ERROR_CONVERSION", err)
	}
	if info.Size() >= audioMaxUploadBytes {
		return fail("AUDIO_ERROR_CONVERTED_SIZE", errors.New("converted audio reaches 25 MiB limit"))
	}
	converted, err := os.ReadFile(output)
	if err != nil {
		return fail("AUDIO_ERROR_CONVERSION", err)
	}
	normalized, duration, rate, bits, err := normalizeAudioWAV(converted)
	if err != nil {
		return fail("AUDIO_ERROR_CONVERSION", err)
	}
	return normalized, duration, rate, bits, nil
}
