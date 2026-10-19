package controllers

import (
	"net/http"
	"strings"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

type SiteSettings struct {
	Title           string `json:"title"`
	Subtitle        string `json:"subtitle"`
	AsteriskVersion string `json:"asterisk_version"`
}

func GetSiteSettings(c echo.Context) error {
	settings := SiteSettings{Title: "Amerimon"}
	if config.App.DB != nil {
		_ = config.App.DB.QueryRowContext(c.Request().Context(), `SELECT value FROM app_settings WHERE key = 'header_title'`).Scan(&settings.Title)
		_ = config.App.DB.QueryRowContext(c.Request().Context(), `SELECT value FROM app_settings WHERE key = 'header_subtitle'`).Scan(&settings.Subtitle)
	}
	if strings.TrimSpace(settings.Title) == "" {
		settings.Title = "Amerimon"
	}
	settings.AsteriskVersion = readAsteriskVersion()
	return c.JSON(http.StatusOK, settings)
}

func SaveSiteSettings(c echo.Context) error {
	var settings SiteSettings
	if err := c.Bind(&settings); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Configuración inválida."})
	}
	settings.Title = strings.TrimSpace(settings.Title)
	settings.Subtitle = strings.TrimSpace(settings.Subtitle)
	if settings.Title == "" {
		settings.Title = "Amerimon"
	}
	if len([]rune(settings.Title)) > 80 || len([]rune(settings.Subtitle)) > 120 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "El título admite 80 caracteres y el subtítulo 120."})
	}
	if config.App.DB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"message": "La base de datos no está disponible."})
	}
	for key, value := range map[string]string{"header_title": settings.Title, "header_subtitle": settings.Subtitle} {
		if _, err := config.App.DB.ExecContext(c.Request().Context(), `INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "No se pudo guardar la configuración."})
		}
	}
	return GetSiteSettings(c)
}
