package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestClientIPExtraction(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		realIP     string
		want       string
	}{
		{"direct connection ignores spoofed headers", "198.51.100.10:1234", "203.0.113.1", "203.0.113.2", "198.51.100.10"},
		{"private connection is not a trusted proxy", "192.168.1.10:1234", "203.0.113.1", "", "192.168.1.10"},
		{"local proxy uses nearest client", "127.0.0.1:1234", "203.0.113.1, 192.168.1.10", "", "192.168.1.10"},
		{"local proxy rejects malformed chain", "127.0.0.1:1234", "invalid, 192.168.1.10", "", "192.168.1.10"},
		{"local connection without proxy header", "127.0.0.1:1234", "", "203.0.113.2", "127.0.0.1"},
	}

	e := newHTTPRouter()
	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, c.RealIP())
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			req.Header.Set(echo.HeaderXForwardedFor, tt.forwarded)
			req.Header.Set(echo.HeaderXRealIP, tt.realIP)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if got := rec.Body.String(); got != tt.want {
				t.Fatalf("client IP = %q, want %q", got, tt.want)
			}
		})
	}
}
