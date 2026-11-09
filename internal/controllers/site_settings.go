package controllers

// Este archivo implementa la consulta y actualización de la configuración general del sitio.

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

type SiteSettings struct {
	Title                        string `json:"title"`
	Subtitle                     string `json:"subtitle"`
	WeatherLocation              string `json:"weather_location"`
	AllstarlinkRefreshIntervalMS int    `json:"allstarlink_refresh_interval_ms"`
	AsteriskVersion              string `json:"asterisk_version"`
	AppVersion                   string `json:"app_version"`
}

// GetSiteSettings devuelve el título, subtítulo y ubicación del clima guardados, junto con las versiones de la aplicación y Asterisk.
func GetSiteSettings(c echo.Context) error {
	settings := SiteSettings{Title: "Amerimon", AllstarlinkRefreshIntervalMS: 15000, AppVersion: config.App.Version}
	if config.App.DB != nil {
		_ = config.App.DB.QueryRowContext(c.Request().Context(), `SELECT value FROM app_settings WHERE key = 'header_title'`).Scan(&settings.Title)
		_ = config.App.DB.QueryRowContext(c.Request().Context(), `SELECT value FROM app_settings WHERE key = 'header_subtitle'`).Scan(&settings.Subtitle)
		settings.WeatherLocation = readWeatherLocationSetting(c.Request().Context())
		settings.AllstarlinkRefreshIntervalMS = allstarlinkRefreshInterval(c.Request().Context())
	}
	if strings.TrimSpace(settings.Title) == "" {
		settings.Title = "Amerimon"
	}
	settings.AsteriskVersion = readAsteriskVersion()
	return c.JSON(http.StatusOK, settings)
}

// SaveSiteSettings valida y persiste el título, subtítulo y ubicación del clima; responde con la configuración actualizada.
func SaveSiteSettings(c echo.Context) error {
	var request struct {
		Title                        string `json:"title"`
		Subtitle                     string `json:"subtitle"`
		WeatherLocation              string `json:"weather_location"`
		AllstarlinkRefreshIntervalMS *int   `json:"allstarlink_refresh_interval_ms"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "Configuración inválida."})
	}
	settings := SiteSettings{
		Title: request.Title, Subtitle: request.Subtitle, WeatherLocation: request.WeatherLocation,
		AllstarlinkRefreshIntervalMS: allstarlinkRefreshInterval(c.Request().Context()),
	}
	if request.AllstarlinkRefreshIntervalMS != nil {
		settings.AllstarlinkRefreshIntervalMS = *request.AllstarlinkRefreshIntervalMS
	}
	settings.Title = strings.TrimSpace(settings.Title)
	settings.Subtitle = strings.TrimSpace(settings.Subtitle)
	settings.WeatherLocation = strings.TrimSpace(settings.WeatherLocation)
	if !validAllstarlinkRefreshInterval(settings.AllstarlinkRefreshIntervalMS) {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "El intervalo inicial de AllStarLink no es válido."})
	}
	if settings.Title == "" {
		settings.Title = "Amerimon"
	}
	if len([]rune(settings.Title)) > 80 || len([]rune(settings.Subtitle)) > 120 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "El título admite 80 caracteres y el subtítulo 120."})
	}
	if len([]rune(settings.WeatherLocation)) > 120 {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "La ubicación del clima admite 120 caracteres."})
	}
	if config.App.DB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"message": "La base de datos no está disponible."})
	}
	values := map[string]string{
		"header_title":                    settings.Title,
		"header_subtitle":                 settings.Subtitle,
		"weather_location":                settings.WeatherLocation,
		"allstarlink_refresh_interval_ms": strconv.Itoa(settings.AllstarlinkRefreshIntervalMS),
	}
	for key, value := range values {
		if _, err := config.App.DB.ExecContext(c.Request().Context(), `INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"message": "No se pudo guardar la configuración."})
		}
	}
	return GetSiteSettings(c)
}

func validAllstarlinkRefreshInterval(interval int) bool {
	switch interval {
	case -1, 0, 5000, 10000, 15000, 30000, 60000, 300000:
		return true
	default:
		return false
	}
}

func allstarlinkRefreshInterval(ctx context.Context) int {
	const defaultInterval = 15000
	if config.App.DB == nil {
		return defaultInterval
	}
	var intervalText string
	if err := config.App.DB.QueryRowContext(ctx,
		`SELECT value FROM app_settings WHERE key = 'allstarlink_refresh_interval_ms'`).Scan(&intervalText); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("site settings: no se pudo leer el intervalo inicial de AllStarLink: %v", err)
		}
		return defaultInterval
	}
	interval, err := strconv.Atoi(intervalText)
	if err != nil {
		log.Printf("site settings: intervalo inicial de AllStarLink no numérico: %q", intervalText)
		return defaultInterval
	}
	if !validAllstarlinkRefreshInterval(interval) {
		log.Printf("site settings: intervalo inicial de AllStarLink fuera del rango permitido: %d", interval)
		return defaultInterval
	}
	return interval
}
