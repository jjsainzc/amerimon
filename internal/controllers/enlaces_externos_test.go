package controllers

// Este archivo contiene pruebas para la consulta y administración de enlaces externos.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

func TestEnlacesExternosCRUD(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previous := config.App.DB
	config.App.DB = database
	defer func() { config.App.DB = previous }()

	e := echo.New()
	e.GET("/enlaces", ListEnlacesExternos)
	e.GET("/admin/enlaces", ListEnlacesExternosAdmin)
	e.POST("/admin/enlaces", CreateEnlaceExterno)
	e.PUT("/admin/enlaces/:id", UpdateEnlaceExterno)
	e.DELETE("/admin/enlaces/:id", DeleteEnlaceExterno)
	request := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, req)
		if recorder.Code != status {
			t.Fatalf("%s %s: want %d, got %d: %s", method, path, status, recorder.Code, recorder.Body.String())
		}
		return recorder
	}
	request(http.MethodGet, "/enlaces", "", http.StatusOK)
	created := request(http.MethodPost, "/admin/enlaces",
		`{"descripcion":"Red","url":"https://example.org/"}`, http.StatusCreated)
	var enlace enlaceExterno
	if err := json.Unmarshal(created.Body.Bytes(), &enlace); err != nil {
		t.Fatal(err)
	}
	if enlace.DashID != 1 || enlace.Target != "_blank" || enlace.IsActive != 1 {
		t.Fatalf("unexpected defaults: %+v", enlace)
	}
	public := request(http.MethodGet, "/enlaces", "", http.StatusOK)
	var enlaces []enlaceExterno
	if err := json.Unmarshal(public.Body.Bytes(), &enlaces); err != nil || len(enlaces) != 1 {
		t.Fatalf("active link missing: %s (err %v)", public.Body.String(), err)
	}
	request(http.MethodPut, "/admin/enlaces/1",
		`{"descripcion":"Red actualizada","url":"https://example.org/new","target":"_self","is_active":0}`, http.StatusOK)
	public = request(http.MethodGet, "/enlaces", "", http.StatusOK)
	if strings.TrimSpace(public.Body.String()) != "[]" {
		t.Fatalf("inactive link exposed publicly: %s", public.Body.String())
	}
	admin := request(http.MethodGet, "/admin/enlaces", "", http.StatusOK)
	if err := json.Unmarshal(admin.Body.Bytes(), &enlaces); err != nil || len(enlaces) != 1 ||
		enlaces[0].IsActive != 0 || enlaces[0].Target != "_self" {
		t.Fatalf("unexpected admin list: %s (err %v)", admin.Body.String(), err)
	}
	request(http.MethodPost, "/admin/enlaces",
		`{"descripcion":"Unsafe","url":"javascript:alert(1)"}`, http.StatusBadRequest)
	request(http.MethodPost, "/admin/enlaces",
		`{"descripcion":"Bad target","url":"https://example.org","target":"_parent"}`, http.StatusBadRequest)
	request(http.MethodPost, "/admin/enlaces",
		`{"descripcion":"Bad state","url":"https://example.org","is_active":2}`, http.StatusBadRequest)
	request(http.MethodPost, "/admin/enlaces",
		`{"descripcion":"`+strings.Repeat("a", 256)+`","url":"https://example.org"}`, http.StatusBadRequest)
	request(http.MethodDelete, "/admin/enlaces/1", "", http.StatusNoContent)
	request(http.MethodDelete, "/admin/enlaces/1", "", http.StatusNotFound)
	request(http.MethodPut, "/admin/enlaces/999",
		`{"descripcion":"Missing","url":"https://example.org"}`, http.StatusNotFound)
	request(http.MethodDelete, "/admin/enlaces/invalid", "", http.StatusBadRequest)
}
