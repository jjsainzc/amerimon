package routes

import (
	"net/http/httptest"
	"strings"
	"testing"

	"amerimon/internal/controllers"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

func TestPrimaryNodeRoutesRequireAdministrator(t *testing.T) {
	t.Setenv("AMERIMON_JWT_SECRET", "test-primary-node-secret")
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, controllers.CustomClaims{Role: "U"}).SignedString(controllers.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/mon/primary-node"}, {"PUT", "/mon/primary-node"},
		{"GET", "/mon/primary-node/ami"}, {"PUT", "/mon/primary-node/ami"},
		{"GET", "/mon/primary-node/access"}, {"PUT", "/mon/primary-node/access"}, {"DELETE", "/mon/primary-node/access"},
		{"POST", "/mon/primary-node/access/clear"},
		{"GET", "/mon/primary-node/access/export"},
		{"POST", "/mon/primary-node/access/import"},
		{"GET", "/mon/primary-node/public-access"}, {"PUT", "/mon/primary-node/public-access"}, {"DELETE", "/mon/primary-node/public-access"},
		{"GET", "/mon/audio/recordings"}, {"POST", "/mon/audio/recordings"},
		{"PATCH", "/mon/audio/recordings/1"}, {"GET", "/mon/audio/recordings/1/file"},
		{"DELETE", "/mon/audio/recordings/1"}, {"POST", "/mon/audio/recordings/1/library"},
		{"GET", "/mon/audio/tracks/1/file"},
		{"GET", "/mon/node-reconnections"}, {"POST", "/mon/node-reconnections"},
		{"PUT", "/mon/node-reconnections/1"}, {"DELETE", "/mon/node-reconnections/1"},
	} {
		for _, tc := range []struct {
			name, token string
			status      int
		}{
			{"anonymous", "", 401}, {"invalid", "invalid", 401}, {"non-admin", token, 403},
		} {
			t.Run(endpoint.method+endpoint.path+"/"+tc.name, func(t *testing.T) {
				e := echo.New()
				Setup(e, nil)
				req := httptest.NewRequest(endpoint.method, endpoint.path, strings.NewReader("{}"))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				if tc.token != "" {
					req.Header.Set(echo.HeaderAuthorization, "Bearer "+tc.token)
				}
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				if rec.Code != tc.status {
					t.Fatalf("got %d: %s", rec.Code, rec.Body)
				}
			})
		}
	}
}
