package controllers

// Este archivo contiene pruebas para la lectura y actualización del idioma de la aplicación.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestAppLanguageDefaultsToSpanish(t *testing.T) {
	setupNextionTest(t)
	if got := readAppLanguage(context.Background()); got != "es" {
		t.Fatalf("readAppLanguage() = %q; want \"es\"", got)
	}
}

func TestSaveAppLanguagePersistsNormalizedValueAndRejectsUnsupported(t *testing.T) {
	setupNextionTest(t)
	c, rec := nodeTestContext(http.MethodPut, map[string]string{"language": " EN "})
	if err := SaveAppLanguage(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d %s", rec.Code, rec.Body.String())
	}
	if got := readAppLanguage(context.Background()); got != "en" {
		t.Fatalf("saved language = %q; want \"en\"", got)
	}

	c, _ = nodeTestContext(http.MethodPut, map[string]string{"language": "fr"})
	err := SaveAppLanguage(c)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported language to fail with 400: error=%v", err)
	}
	if got := readAppLanguage(context.Background()); got != "en" {
		t.Fatalf("rejected language changed the stored value: %q", got)
	}
}
