package controllers

// Este archivo contiene pruebas para la consulta y actualización de la configuración general del sitio.

import (
	"encoding/json"
	"net/http"
	"testing"

	"amerimon/internal/config"
	"amerimon/internal/db"
)

func TestSiteSettingsPersistAllstarlinkRefreshInterval(t *testing.T) {
	previous := config.App
	t.Cleanup(func() { config.App = previous })
	database, err := db.Open(t.TempDir() + "/settings.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	config.App.DB = database

	c, rec := nodeTestContext(http.MethodPut, map[string]any{
		"title": "Amerimon", "subtitle": "", "weather_location": "",
		"allstarlink_refresh_interval_ms": -1,
	})
	if err := SaveSiteSettings(c); err != nil {
		t.Fatal(err)
	}
	var response SiteSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.AllstarlinkRefreshIntervalMS != -1 {
		t.Fatalf("unexpected saved interval: %+v", response)
	}

	c, rec = nodeTestContext(http.MethodPut, map[string]any{
		"title": "Amerimon", "allstarlink_refresh_interval_ms": 1234,
	})
	if err := SaveSiteSettings(c); err != nil || rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid interval should be rejected: status=%d err=%v", rec.Code, err)
	}
}
