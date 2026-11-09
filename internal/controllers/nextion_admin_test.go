package controllers

// Este archivo contiene pruebas para la lectura y administración de la configuración Nextion.

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

func setupNextionTest(t *testing.T) {
	t.Helper()
	previous := config.App
	t.Cleanup(func() { config.App = previous })
	database, err := db.Open(t.TempDir() + "/nextion.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	config.App.DB = database
}

func TestSaveNextionSettingsAllowsIncompleteDisabledConfiguration(t *testing.T) {
	setupNextionTest(t)
	c, rec := nodeTestContext(http.MethodPut, map[string]any{
		"enabled": false, "port": "/dev/ttyUSB", "baud_rate": 230400, "interface": "", "favorite_node": "",
	})
	if err := SaveNextionSettings(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d %s", rec.Code, rec.Body.String())
	}
	for key, want := range map[string]string{
		"nextion_enabled": "0", "nextion_port": "/dev/ttyUSB", "nextion_baud_rate": "230400",
		"nextion_interface": "", "nextion_favorite_node": "",
	} {
		var got string
		if err := config.App.DB.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, key).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %q; want %q", key, got, want)
		}
	}
}

func TestSaveNextionSettingsRejectsIncompleteEnabledConfiguration(t *testing.T) {
	setupNextionTest(t)
	c, _ := nodeTestContext(http.MethodPut, map[string]any{
		"enabled": true, "port": "", "baud_rate": 9600, "interface": "", "favorite_node": "",
	})
	err := SaveNextionSettings(c)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid enabled configuration to fail with 400: error=%v", err)
	}
	var count int
	if err := config.App.DB.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key LIKE 'nextion_%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("invalid configuration changed the existing settings: %d settings", count)
	}
}

func TestNextionSerialConfigurationValidation(t *testing.T) {
	for _, test := range []struct {
		name     string
		settings nextionSettings
		wantErr  bool
	}{
		{name: "disabled incomplete", settings: nextionSettings{BaudRate: 9600}},
		{name: "custom device path", settings: nextionSettings{Enabled: true, Port: "/dev/ttyS0", BaudRate: 9600}, wantErr: true},
		{name: "unsupported speed", settings: nextionSettings{Enabled: true, Port: "/dev/ttyUSB0", BaudRate: 230400}, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateNextionSettings(test.settings)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateNextionSettings() error = %v; want error %t", err, test.wantErr)
			}
		})
	}
}

func TestNextionInterfaceIPv4ListsOnlyUniqueUsableIPv4Addresses(t *testing.T) {
	addresses := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.0.2.20"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("192.0.2.10"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("192.0.2.10"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
		&net.IPNet{IP: net.ParseIP("169.254.1.1"), Mask: net.CIDRMask(16, 32)},
		&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
	}
	if got, want := nextionInterfaceIPv4(addresses), "192.0.2.10, 192.0.2.20"; got != want {
		t.Fatalf("nextionInterfaceIPv4() = %q; want %q", got, want)
	}
}

func TestNextionCatalogGroupsLanguageSpecificSourceFirmwareAndPreview(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AMERIMON_NEXTION_DIR", dir)
	name := "AMERINET_Nextion_TestBoard-es"
	for _, extension := range []string{".HMI", ".tft"} {
		if err := os.WriteFile(filepath.Join(dir, name+extension), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	models := listNextionModels()
	var found *nextionModel
	for i := range models {
		if models[i].Name == name {
			found = &models[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("language-specific Nextion model %q was not discovered", name)
	}
	if found.Model != "TestBoard" || found.Language != "es" || !found.SourceAvailable ||
		found.PreviewURL != "assets/images/"+name+".png" {
		t.Fatalf("unexpected model metadata: %+v", *found)
	}
}

func TestDownloadNextionFileServesAdminRequestedHMIAndTFT(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AMERIMON_NEXTION_DIR", dir)
	name := "AMERINET_Nextion_TestBoard-es"
	for _, test := range []struct {
		extension string
		filename  string
	}{
		{extension: "hmi", filename: name + ".HMI"},
		{extension: "tft", filename: name + ".tft"},
	} {
		t.Run(test.extension, func(t *testing.T) {
			const content = "firmware source"
			if err := os.WriteFile(filepath.Join(dir, test.filename), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			c.SetParamNames("model", "extension")
			c.SetParamValues(name, test.extension)
			if err := DownloadNextionFile(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != content {
				t.Fatalf("unexpected download: status=%d body=%q", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Content-Disposition"), test.filename) {
				t.Fatalf("unexpected download filename: %q", rec.Header().Get("Content-Disposition"))
			}
		})
	}
}

func TestNextionCP210xSerialProbe(t *testing.T) {
	ports := discoverNextionPorts()
	for _, port := range ports {
		if !strings.EqualFold(port.VendorID, "10c4") || !strings.EqualFold(port.ProductID, "ea60") {
			continue
		}
		serial, err := openSerial(port.Path, 9600)
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				t.Skipf("el proceso no tiene permisos para probar %s; ejecútalo como root o con acceso dialout", port.Path)
			}
			t.Fatalf("no se pudo abrir el CP210x detectado en %s: %v", port.Path, err)
		}
		if err := serial.Close(); err != nil {
			t.Fatalf("no se pudo cerrar el puerto CP210x %s: %v", port.Path, err)
		}
		return
	}
	t.Skip("no hay un adaptador CP210x 10c4:ea60 conectado")
}
