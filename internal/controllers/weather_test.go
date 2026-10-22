package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestParseWeatherCoordinates(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLat   float64
		wantLon   float64
		wantValid bool
	}{
		{name: "browser coordinates", input: " 37.7749, -122.4194 ", wantLat: 37.7749, wantLon: -122.4194, wantValid: true},
		{name: "equator", input: "0,0", wantValid: true},
		{name: "invalid latitude", input: "91,0"},
		{name: "invalid longitude", input: "0,-181"},
		{name: "not coordinates", input: "San Francisco"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, valid := parseWeatherCoordinates(tt.input)
			if valid != tt.wantValid || (valid && (lat != tt.wantLat || lon != tt.wantLon)) {
				t.Fatalf("parseWeatherCoordinates(%q) = (%v, %v, %v), want (%v, %v, %v)",
					tt.input, lat, lon, valid, tt.wantLat, tt.wantLon, tt.wantValid)
			}
		})
	}
}

func TestGetWeatherRejectsInvalidQuery(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{name: "invalid mode", query: "?location=0%2C0&mode=K", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mon/weather"+tt.query, nil)
			recorder := httptest.NewRecorder()
			ctx := echo.New().NewContext(req, recorder)

			err := GetWeather(ctx)
			httpErr, ok := err.(*echo.HTTPError)
			if !ok || httpErr.Code != tt.wantStatus {
				t.Fatalf("GetWeather() error = %v, want HTTP status %d", err, tt.wantStatus)
			}
		})
	}
}

func TestGetWeatherFallbackLocation(t *testing.T) {
	origSetting, origGeo, origFetch := weatherLocationSetting, weatherGeoIPLookup, weatherFetch
	t.Cleanup(func() { weatherLocationSetting, weatherGeoIPLookup, weatherFetch = origSetting, origGeo, origFetch })

	tests := []struct {
		name         string
		query        string
		setting      string
		remoteAddr   string
		wantLocation string
		wantGeoIP    string
		wantGeoCall  bool
	}{
		{name: "browser location wins", query: "?location=1%2C2", setting: "Quito", wantLocation: "1,2"},
		{name: "configured location", setting: "Quito", wantLocation: "Quito"},
		{name: "public visitor IP", remoteAddr: "8.8.8.8:1234", wantLocation: "10.5000,-66.9000", wantGeoIP: "8.8.8.8", wantGeoCall: true},
		{name: "private visitor IP uses server IP", remoteAddr: "192.168.1.5:1234", wantLocation: "10.5000,-66.9000", wantGeoIP: "", wantGeoCall: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			geoCalled, gotIP, gotLocation := false, "", ""
			weatherLocationSetting = func(context.Context) string { return tt.setting }
			weatherGeoIPLookup = func(ip string) (*geoLocation, error) {
				geoCalled, gotIP = true, ip
				return &geoLocation{Latitude: 10.5, Longitude: -66.9}, nil
			}
			weatherFetch = func(location, _ string) (string, error) {
				gotLocation = location
				return "ok", nil
			}

			req := httptest.NewRequest(http.MethodGet, "/mon/weather"+tt.query, nil)
			if tt.remoteAddr != "" {
				req.RemoteAddr = tt.remoteAddr
			}
			recorder := httptest.NewRecorder()
			if err := GetWeather(echo.New().NewContext(req, recorder)); err != nil {
				t.Fatalf("GetWeather() error = %v", err)
			}
			if gotLocation != tt.wantLocation || geoCalled != tt.wantGeoCall || gotIP != tt.wantGeoIP {
				t.Fatalf("location=%q geoCalled=%v ip=%q, want %q %v %q", gotLocation, geoCalled, gotIP, tt.wantLocation, tt.wantGeoCall, tt.wantGeoIP)
			}
		})
	}
}
