package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"amerimon/internal/controllers"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

func TestSystemControlRouteRequiresAdministrator(t *testing.T) {
	t.Setenv("AMERIMON_JWT_SECRET", "test-system-control-secret")
	userToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, controllers.CustomClaims{Role: "U"}).SignedString(controllers.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		want        int
	}{
		{"anonymous", "", 401},
		{"invalid-token", "invalid", 401},
		{"non-admin", userToken, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			Setup(e, nil)
			req := httptest.NewRequest(http.MethodPost, "/mon/system/control", strings.NewReader(`{"action":"linux_poweroff","confirmation":"APAGAR LINUX"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			if tc.token != "" {
				req.Header.Set(echo.HeaderAuthorization, "Bearer "+tc.token)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
