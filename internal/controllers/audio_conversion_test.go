package controllers

// Este archivo contiene pruebas para la validación y conversión de archivos de audio.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

// testEncodedAudio genera un archivo de audio de prueba con la extensión y duración especificadas, utilizando FFmpeg, y devuelve sus datos en bytes.
func testEncodedAudio(t *testing.T, extension string, seconds string) []byte {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), "sample"+extension)
	output, err := exec.Command(ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", seconds, "-ac", "2", path).CombinedOutput()
	if err != nil {
		t.Fatalf("create %s: %v %s", extension, err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestAudioConversionFormats verifica que los archivos de audio en diferentes formatos se conviertan correctamente a WAV con las especificaciones esperadas.
func TestAudioConversionFormats(t *testing.T) {
	for _, extension := range []string{".mp3", ".flac", ".m4a", ".ogg", ".wav"} {
		t.Run(extension, func(t *testing.T) {
			input := testEncodedAudio(t, extension, "0.3")
			wav, duration, rate, bits, err := convertUploadedAudio(context.Background(), input, extension)
			if err != nil {
				t.Fatal(err)
			}
			if rate != 8000 || bits != 16 || duration.Milliseconds() < 250 || duration.Milliseconds() > 400 ||
				!bytes.Equal(wav[:4], []byte("RIFF")) {
				t.Fatalf("wrong converted format: rate=%d bits=%d duration=%v", rate, bits, duration)
			}
		})
	}
}

// TestAudioConversionErrors comprueba que se manejen correctamente los errores durante la conversión de audio, incluyendo formatos inválidos, conversiones fallidas y tiempos de espera.
func TestAudioConversionErrors(t *testing.T) {
	assertCode := func(ctx context.Context, data []byte, extension, code string) {
		t.Helper()
		_, _, _, _, err := convertUploadedAudio(ctx, data, extension)
		var conversion *audioConversionError
		if !errors.As(err, &conversion) || conversion.code != code {
			t.Fatalf("error = %v, want %s", err, code)
		}
	}
	assertCode(context.Background(), []byte("invalid"), ".txt", "AUDIO_ERROR_INPUT_FORMAT")
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		assertCode(context.Background(), []byte("not audio"), ".mp3", "AUDIO_ERROR_CONVERSION")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assertCode(ctx, []byte("invalid"), ".mp3", "AUDIO_ERROR_CONVERSION_TIMEOUT")
	}
	audioConversionSlot <- struct{}{}
	assertCode(context.Background(), []byte("invalid"), ".mp3", "AUDIO_ERROR_CONVERSION_BUSY")
	<-audioConversionSlot
	t.Setenv("PATH", t.TempDir())
	assertCode(context.Background(), []byte("invalid"), ".mp3", "AUDIO_ERROR_CONVERTER_MISSING")
	if _, _, _, _, err := convertUploadedAudio(context.Background(), testPCMMonoWAV(8000, 16, 1), ".wav"); err != nil {
		t.Fatalf("compatible WAV should not need FFmpeg: %v", err)
	}
}

// TestAudioConversionRejectsOversizeRatherThanTruncating asegura que los archivos de audio que exceden el tamaño máximo permitido sean rechazados en lugar de truncados.
func TestAudioConversionRejectsOversizeRatherThanTruncating(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "long.flac")
	output, err := exec.Command(ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "anullsrc=r=8000:cl=mono", "-t", "1640", path).CombinedOutput()
	if err != nil {
		t.Fatalf("create long audio: %v %s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	_, _, _, _, err = convertUploadedAudio(context.Background(), data, ".flac")
	var conversion *audioConversionError
	if !errors.As(err, &conversion) || conversion.code != "AUDIO_ERROR_CONVERTED_SIZE" {
		t.Fatalf("oversize output must be rejected: %v", err)
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("conversion temporary files remain: %v %v", entries, err)
	}
}

// TestAudioUploadMP3ConvertedAndPersisted verifica que un archivo MP3 subido se convierta correctamente y se persista en la base de datos.
func TestAudioUploadMP3ConvertedAndPersisted(t *testing.T) {
	input := testEncodedAudio(t, ".mp3", "0.3")
	previous := config.App
	defer func() { config.App = previous }()
	database, err := db.Open(filepath.Join(t.TempDir(), "audio.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer database.Close()
	config.App.DB = database
	config.App.Audio.StorageDir = t.TempDir()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	writer.WriteField("name", "MP3 upload")
	writer.WriteField("playback_mode", "broadcast_once")
	part, err := writer.CreateFormFile("file", "test.MP3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(input); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	if err := UploadAudioTrack(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 201 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	var response struct {
		Data AudioTrack `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.OriginalName != "test.MP3" || response.Data.SampleRate != 8000 || response.Data.BitsPerSample != 16 {
		t.Fatalf("wrong metadata: %+v", response.Data)
	}
}
