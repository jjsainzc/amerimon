package controllers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type geoLocation struct {
	IP          string  `json:"ip"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	City        string  `json:"city"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	TimeZone    string  `json:"time_zone"`
}

type ipAPIResponse struct {
	Status      string  `json:"status"`
	Message     string  `json:"message"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	City        string  `json:"city"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Timezone    string  `json:"timezone"`
	Query       string  `json:"query"`
}

var geoClient = &http.Client{Timeout: 5 * time.Second}

func lookupGeoIP(ip string) (*geoLocation, error) {
	resp, err := geoClient.Get("http://ip-api.com/json/" + ip + "?lang=es")
	if err != nil {
		return nil, fmt.Errorf("error al conectar con el servicio de IP: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("el servicio remoto respondió con código %d", resp.StatusCode)
	}
	var d ipAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("respuesta inválida del servicio de IP: %w", err)
	}
	if d.Status != "success" {
		return nil, fmt.Errorf("ip-api: %s", d.Message)
	}
	return &geoLocation{IP: d.Query, Country: d.Country, CountryCode: d.CountryCode, City: d.City,
		Latitude: d.Lat, Longitude: d.Lon, TimeZone: d.Timezone}, nil
}

// GeoIP consulta la ubicación de una IP pública (solo IPs, no hostnames).
func GeoIP(c echo.Context) error {
	ip, err := netip.ParseAddr(c.Param("ip"))
	if err != nil {
		return apiError(http.StatusBadRequest, "IP inválida")
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
		return c.JSON(http.StatusOK, echo.Map{"status": "success", "hostname": "localhost", "data": geoLocation{
			IP: ip.String(), Country: "Localhost / Red Privada", CountryCode: "LOCAL", City: "Localhost", TimeZone: "UTC",
		}})
	}

	data, err := lookupGeoIP(ip.String())
	if err != nil {
		return apiError(http.StatusBadGateway, err.Error())
	}

	hostname := "--"
	if names, err := net.LookupAddr(ip.String()); err == nil && len(names) > 0 {
		hostname = strings.TrimSuffix(names[0], ".")
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "hostname": hostname, "data": data})
}
