package controllers

// Este archivo implementa la consulta y presentación de información meteorológica.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

// Estructuras para respuestas JSON de Open-Meteo y Geocoding
type OpenMeteoGeoResponse struct {
	Results []struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Country   string  `json:"country_code"`
	} `json:"results"`
}

// OpenMeteoWeatherResponse representa la respuesta JSON de Open-Meteo para el clima actual.
type OpenMeteoWeatherResponse struct {
	Current struct {
		Temperature2M      float64 `json:"temperature_2m"`
		WeatherCode        int     `json:"weather_code"`
		RelativeHumidity2M float64 `json:"relative_humidity_2m"`
		PressureMsl        float64 `json:"pressure_msl"`
		WindSpeed10M       float64 `json:"wind_speed_10m"`
		WindDirection10M   float64 `json:"wind_direction_10m"`
		WindGusts10M       float64 `json:"wind_gusts_10m"`
		Precipitation      float64 `json:"precipitation"`
		CloudCover         int     `json:"cloud_cover"`
		IsDay              int     `json:"is_day"`
	} `json:"current"`
	Error bool `json:"error"`
}

// NWSAlertsResponse representa la respuesta JSON de alertas meteorológicas de NWS.
type NWSAlertsResponse struct {
	Features []struct {
		Properties struct {
			Event string `json:"event"`
		} `json:"properties"`
	} `json:"features"`
}

var weatherHTTPClient = &http.Client{Timeout: 10 * time.Second}

// Dependencias reemplazables en pruebas.
var (
	weatherLocationSetting = readWeatherLocationSetting
	weatherGeoIPLookup     = lookupGeoIP
	weatherFetch           = getWeather
)

// readWeatherLocationSetting devuelve la ubicación del clima configurada en la cabecera, o "" si no hay.
func readWeatherLocationSetting(ctx context.Context) string {
	if config.App.DB == nil {
		return ""
	}
	var value string
	_ = config.App.DB.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'weather_location'`).Scan(&value)
	return strings.TrimSpace(value)
}

// fallbackWeatherLocation resuelve la ubicación cuando el navegador no la comparte:
// primero la configurada en la cabecera y, si está vacía, la IP del visitante
// (o la IP pública del servidor si el visitante usa una IP privada o local).
func fallbackWeatherLocation(c echo.Context) (string, error) {
	if configured := weatherLocationSetting(c.Request().Context()); configured != "" {
		return configured, nil
	}
	ip := ""
	if addr, err := netip.ParseAddr(c.RealIP()); err == nil && addr.IsGlobalUnicast() && !addr.IsPrivate() {
		ip = addr.String()
	}
	geo, err := weatherGeoIPLookup(ip)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%.4f,%.4f", geo.Latitude, geo.Longitude), nil
}

// GetWeather devuelve el clima actual para una ubicación o unas coordenadas.
// Sin 'location' usa la ubicación configurada en la cabecera y, si no hay, la IP del visitante.
func GetWeather(c echo.Context) error {
	location := strings.TrimSpace(c.QueryParam("location"))
	if location == "" {
		fallback, err := fallbackWeatherLocation(c)
		if err != nil {
			return apiError(http.StatusBadGateway, "No se pudo determinar la ubicación: "+err.Error())
		}
		location = fallback
	}

	temperatureMode := strings.ToUpper(strings.TrimSpace(c.QueryParam("mode")))
	if temperatureMode == "" {
		temperatureMode = "F"
	}
	if temperatureMode != "F" && temperatureMode != "C" {
		return apiError(http.StatusBadRequest, "El parámetro 'mode' debe ser F o C")
	}

	weatherOutput, err := weatherFetch(location, temperatureMode)
	if err != nil {
		return apiError(http.StatusBadGateway, "No se pudo obtener el clima: "+err.Error())
	}
	return c.String(http.StatusOK, weatherOutput+"\n")
}

// getWeather obtiene los datos meteorológicos de Open-Meteo para una ubicación dada.
func getWeather(locationRaw string, temperatureMode string) (string, error) {
	raw := strings.TrimSpace(locationRaw)

	lat, lon, _, err := resolveLocation(raw)
	if err != nil {
		return "", err
	}

	// Consultar Open-Meteo
	tempUnit := "fahrenheit"
	if temperatureMode == "C" {
		tempUnit = "celsius"
	}

	apiURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%f&longitude=%f&timezone=auto&temperature_unit=%s&wind_speed_unit=ms&precipitation_unit=mm&current=temperature_2m,weather_code,relative_humidity_2m,pressure_msl,wind_speed_10m,wind_direction_10m,wind_gusts_10m,precipitation,cloud_cover,is_day",
		lat, lon, tempUnit,
	)

	resp, err := weatherHTTPClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("conectar con Open-Meteo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Open-Meteo respondió con código %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("leer respuesta de Open-Meteo: %w", err)
	}

	var weatherData OpenMeteoWeatherResponse
	if err := json.Unmarshal(bodyBytes, &weatherData); err != nil || weatherData.Error {
		return "", fmt.Errorf("respuesta inválida de Open-Meteo")
	}

	tNow := weatherData.Current.Temperature2M
	wCode := weatherData.Current.WeatherCode
	rh := weatherData.Current.RelativeHumidity2M
	pmsl := weatherData.Current.PressureMsl
	ws := weatherData.Current.WindSpeed10M
	wd := weatherData.Current.WindDirection10M
	wg := weatherData.Current.WindGusts10M
	cc := weatherData.Current.CloudCover
	isDay := weatherData.Current.IsDay

	var tf, tc float64
	if temperatureMode == "F" {
		tf = math.Round(tNow)
		tc = (tNow - 32) * 5 / 9
	} else {
		tc = tNow
		tf = (tNow * 9 / 5) + 32
	}

	deg := "°"
	mainDeg := ""
	altDeg := ""

	if temperatureMode == "F" {
		mainDeg = fmt.Sprintf("%.0f%sF", tf, deg)
		altDeg = fmt.Sprintf("%.1f%sC", tc, deg)
	} else {
		mainDeg = fmt.Sprintf("%.1f%sC", tc, deg)
		altDeg = fmt.Sprintf("%.0f%sF", tf, deg)
	}

	condTitle := condFromWmo(wCode, cc, isDay)

	// Segmentos de salida
	seg1 := fmt.Sprintf("%s, %s", mainDeg, altDeg)
	if rh > 0 {
		seg1 += fmt.Sprintf(", %.0f%% RH", rh)
	}

	seg2 := condTitle
	if ws > 0 {
		mph := ws * 2.23694
		kmh := ws * 3.6
		dir := toCardinal(wd)
		windStr := fmt.Sprintf("Wind: %.0f mph - %.0f km/h %s", mph, kmh, dir)
		if wg > 0 {
			gustMph := wg * 2.23694
			windStr += fmt.Sprintf(" (gust %.0f)", gustMph)
		}
		seg2 += ", " + windStr
	}

	if pmsl > 0 {
		inhg := pmsl * 0.02953
		seg2 += fmt.Sprintf(" / P: %.1f hPa - %.2f inHG", pmsl, inhg)
	}

	// NWS Alerts
	alertsJoined := getNWSAlerts(lat, lon)
	if alertsJoined != "" {
		return fmt.Sprintf("%s / %s / Alerts: %s", seg1, seg2, alertsJoined), nil
	}

	return fmt.Sprintf("%s / %s", seg1, seg2), nil
}

// resolveLocation resuelve una ubicación a coordenadas geográficas (latitud y longitud) y país utilizando Open-Meteo y Nominatim.
func resolveLocation(raw string) (float64, float64, string, error) {
	if lat, lon, ok := parseWeatherCoordinates(raw); ok {
		return lat, lon, "", nil
	}

	// 1. US ZIP Code
	if match, _ := regexp.MatchString(`^[0-9]{5}$`, raw); match {
		apiURL := "https://geocoding-api.open-meteo.com/v1/search?name=" + url.QueryEscape(raw) + "&count=1&language=en&format=json"
		if lat, lon, country, err := fetchGeo(apiURL); err == nil {
			return lat, lon, country, nil
		}
	}

	// 2. Nominatim fallback para códigos internacionales o generales
	uaURL := fmt.Sprintf("https://nominatim.openstreetmap.org/search?q=%s&format=json&limit=1", url.QueryEscape(raw))
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", uaURL, nil)
	req.Header.Set("User-Agent", "supermon-weather-go/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, "", fmt.Errorf("conectar con el servicio de geolocalización: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("el servicio de geolocalización respondió con código %d", resp.StatusCode)
	}

	var results []struct {
		Lat     string `json:"lat"`
		Lon     string `json:"lon"`
		Country string `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, "", fmt.Errorf("respuesta inválida del servicio de geolocalización: %w", err)
	}
	if len(results) == 0 {
		return 0, 0, "", fmt.Errorf("no se encontró la ubicación %q", raw)
	}
	lat, err := strconv.ParseFloat(results[0].Lat, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("latitud inválida en la respuesta de geolocalización")
	}
	lon, err := strconv.ParseFloat(results[0].Lon, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("longitud inválida en la respuesta de geolocalización")
	}
	return lat, lon, results[0].Country, nil

}

// parseWeatherCoordinates analiza una cadena de coordenadas en formato "lat,lon" y devuelve latitud, longitud y un indicador de éxito.
func parseWeatherCoordinates(raw string) (float64, float64, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil || math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		return 0, 0, false
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil || math.IsNaN(lon) || math.IsInf(lon, 0) || lon < -180 || lon > 180 {
		return 0, 0, false
	}
	return lat, lon, true
}

// fetchGeo obtiene las coordenadas geográficas y el país desde la API de geocodificación de Open-Meteo.
func fetchGeo(apiURL string) (float64, float64, string, error) {
	resp, err := weatherHTTPClient.Get(apiURL)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("el servicio de geolocalización respondió con código %d", resp.StatusCode)
	}

	var geoResp OpenMeteoGeoResponse
	if err := json.NewDecoder(resp.Body).Decode(&geoResp); err != nil || len(geoResp.Results) == 0 {
		return 0, 0, "", fmt.Errorf("no results")
	}

	return geoResp.Results[0].Latitude, geoResp.Results[0].Longitude, geoResp.Results[0].Country, nil
}

// condFromWmo determina la descripción del tiempo basada en el código WMO y la cobertura nubosa.
func condFromWmo(wmo int, cloudCov int, isDay int) string {
	switch wmo {
	case 95, 96, 99:
		return "Thunderstorms"
	case 80, 81, 82, 61, 63, 65, 66, 67, 51, 53, 55, 56, 57:
		return "Rain"
	case 71, 73, 75, 77, 85, 86:
		return "Snow"
	case 45, 48:
		return "Fog"
	}
	if cloudCov <= 10 {
		if isDay == 1 {
			return "Sunny"
		}
		return "Clear"
	} else if cloudCov <= 35 {
		if isDay == 1 {
			return "Mostly Sunny"
		}
		return "Mostly Clear"
	} else if cloudCov <= 65 {
		return "Partly Cloudy"
	} else if cloudCov < 90 {
		return "Mostly Cloudy"
	}
	return "Overcast"
}

// toCardinal convierte un ángulo en grados a su dirección cardinal correspondiente.
func toCardinal(deg float64) string {
	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE", "S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	index := int(math.Mod(math.Floor((deg+11.25)/22.5), 16))
	return directions[index]
}

// getNWSAlerts obtiene las alertas meteorológicas activas de NWS para una ubicación dada.
func getNWSAlerts(lat, lon float64) string {
	apiURL := fmt.Sprintf("https://api.weather.gov/alerts/active?point=%f,%f", lat, lon)
	client := &http.Client{Timeout: 4 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "supermon-weather/1.0 (you@example.com)")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var alerts NWSAlertsResponse
	if json.NewDecoder(resp.Body).Decode(&alerts) != nil || len(alerts.Features) == 0 {
		return ""
	}

	var events []string
	seen := make(map[string]bool)
	for _, feature := range alerts.Features {
		ev := feature.Properties.Event
		if ev != "" && !seen[ev] {
			seen[ev] = true
			events = append(events, ev)
		}
	}

	if len(events) > 3 {
		events = events[:3]
	}

	return strings.Join(events, ", ")
}
