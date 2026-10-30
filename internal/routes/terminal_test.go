package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"amerimon/internal/controllers"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

func TestTerminalRouteRequiresAdministrator(t *testing.T) {
	t.Setenv("AMERIMON_JWT_SECRET", "test-terminal-secret")
	nonAdminToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, controllers.CustomClaims{Role: "U"}).
		SignedString(controllers.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}
	adminToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, controllers.CustomClaims{Role: "A"}).
		SignedString(controllers.JWTSecret())
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "anonymous", wantStatus: http.StatusUnauthorized},
		{name: "non-admin", token: nonAdminToken, wantStatus: http.StatusForbidden},
		{name: "admin", token: adminToken, wantStatus: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()
			Setup(e, nil)
			req := httptest.NewRequest(http.MethodGet, "/mon/terminal?token="+test.token, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != test.wantStatus {
				t.Fatalf("got %d, want %d: %s", rec.Code, test.wantStatus, rec.Body)
			}
		})
	}
}
