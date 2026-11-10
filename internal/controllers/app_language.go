package controllers

// Este archivo implementa la lectura y actualización del idioma de la aplicación,
// que la interfaz web selecciona y la pantalla Nextion usa para sus textos.

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

const appLanguageKey = "app_language"

// Este tipo representa los idiomas disponibles en la aplicación.
var appLanguages = map[string]bool{"es": true, "en": true}

// readAppLanguage lee el idioma actual de la aplicación desde la base de datos. Si no se encuentra o hay un error, devuelve "es".
func readAppLanguage(ctx context.Context) string {
	if config.App.DB == nil {
		return "es"
	}
	var value string
	err := config.App.DB.QueryRowContext(ctx,
		`SELECT value FROM app_settings WHERE key = ?`, appLanguageKey).Scan(&value)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("idioma: no se pudo leer el idioma guardado: %v", err)
		}
		return "es"
	}
	if !appLanguages[value] {
		return "es"
	}
	return value
}

// GetAppLanguage devuelve el idioma guardado de la aplicación.
func GetAppLanguage(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, echo.Map{"language": readAppLanguage(c.Request().Context())})
}

// SaveAppLanguage persiste el idioma elegido en la web y notifica al controlador Nextion.
func SaveAppLanguage(c echo.Context) error {
	var request struct {
		Language string `json:"language"`
	}
	if err := c.Bind(&request); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud de idioma inválida")
	}
	language := strings.ToLower(strings.TrimSpace(request.Language))
	if !appLanguages[language] {
		return apiError(http.StatusBadRequest, "Idioma no admitido")
	}
	if config.App.DB == nil {
		return apiError(http.StatusServiceUnavailable, "La base de datos no está disponible")
	}
	if _, err := config.App.DB.ExecContext(c.Request().Context(),
		`INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		appLanguageKey, language); err != nil {
		log.Printf("idioma: no se pudo guardar el idioma: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudo guardar el idioma")
	}
	nextionRuntime.signalChange()
	return c.JSON(http.StatusOK, echo.Map{"language": language})
}
