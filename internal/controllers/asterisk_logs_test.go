package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

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
