package controllers

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

func TestListenLiveFailureIsStoredInOperationLogs(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			conn.Close()
		}
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	previous := config.App
	config.App.DB = database
	config.App.UseDB = true
	config.App.AMI.Host, config.App.AMI.Port = host, port
	defer func() { config.App = previous }()
	request := httptest.NewRequest(http.MethodPost, "/mon/listen-live/start",
		strings.NewReader(`{"ext":"nodo","uuid":"12345678-1234-1234-1234-123456789012"}`))
	request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder := httptest.NewRecorder()
	if err := StartListenLive(echo.New().NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected AMI error, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var action, result, details string
	err = database.QueryRow(`SELECT action, result, details FROM asterisk_operation_logs ORDER BY id DESC LIMIT 1`).Scan(&action, &result, &details)
	if err != nil {
		t.Fatal(err)
	}
	if action != "listen_start" || result != "ami_error" || !strings.Contains(details, "EOF") {
		t.Fatalf("missing diagnostic details: %q %q %q", action, result, details)
	}
}

func TestAsteriskOperationLogIsStoredAndListed(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	previousDB := config.App.DB
	config.App.DB = database
	defer func() { config.App.DB = previousDB }()

	startedAt := time.Now().Add(-1250 * time.Millisecond)
	recordAsteriskOperation(context.Background(), "connect", "616260", "616261", "transceiver", false,
		"rpt cmd 616260 ilink 3 616261", "not_confirmed", "XStat no confirmó el enlace", startedAt)

	e := echo.New()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/mon/asterisk/logs?limit=10", nil)
	if err := GetAsteriskOperationLogs(e.NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}

	var response struct {
		Status string                 `json:"status"`
		Data   []AsteriskOperationLog `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "success" || len(response.Data) != 1 {
		t.Fatalf("unexpected log response: %+v", response)
	}
	entry := response.Data[0]
	if entry.Action != "connect" || entry.LocalNode != "616260" || entry.RemoteNode != "616261" || entry.Result != "not_confirmed" {
		t.Fatalf("unexpected log entry: %+v", entry)
	}
	if entry.DurationMS < 1000 {
		t.Fatalf("expected duration to include elapsed time, got %d ms", entry.DurationMS)
	}
}
