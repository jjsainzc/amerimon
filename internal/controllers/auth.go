package controllers

// Este archivo implementa la autenticación JWT y la autorización de solicitudes.

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
)

// CustomClaims son los datos almacenados en el payload del JWT.
type CustomClaims struct {
	Callsign string `json:"callsign"`
	Email    string `json:"email"`
	UserID   int    `json:"userid"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// JWTSecret devuelve la clave HMAC: AMERIMON_JWT_SECRET (desarrollo), la credencial
// de systemd o el archivo AMERIMON_JWT_SECRET_FILE (por defecto /etc/amerimon/jwt.secret).
func JWTSecret() []byte {
	if v := os.Getenv("AMERIMON_JWT_SECRET"); v != "" {
		return []byte(v)
	}
	paths := []string{}
	if d := os.Getenv("CREDENTIALS_DIRECTORY"); d != "" {
		paths = append(paths, filepath.Join(d, "jwt_secret"))
	}
	if f := os.Getenv("AMERIMON_JWT_SECRET_FILE"); f != "" {
		paths = append(paths, f)
	} else {
		paths = append(paths, "/etc/amerimon/jwt.secret")
	}
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return []byte(s)
			}
		}
	}
	return nil
}

// GenerateAdminToken firma el token único de administrador, sin expiración.
func GenerateAdminToken() (string, error) {
	secret := JWTSecret()
	if len(secret) == 0 {
		return "", errors.New("no se encontró el secreto JWT")
	}
	claims := CustomClaims{Callsign: "admin", UserID: 1, Role: "A"}
	claims.IssuedAt = jwt.NewNumericDate(time.Now())
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ValidarToken verifica la firma y la expiración del JWT.
func ValidarToken(tokenString string) (*CustomClaims, error) {
	secret := JWTSecret()
	if len(secret) == 0 {
		return nil, errors.New("secreto JWT no configurado")
	}
	if tokenString == "" {
		return nil, errors.New("token vacío")
	}

	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de firma no esperado: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errors.New("el token ha expirado")
		}
		return nil, fmt.Errorf("token inválido: %w", err)
	}
	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, errors.New("claims del token no válidos")
	}
	return claims, nil
}

// JWTMiddleware acepta el token por Authorization Bearer o por ?token= (WebSocket).
func JWTMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		var tokenStr string
		if h := c.Request().Header.Get("Authorization"); h != "" {
			parts := strings.Split(h, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				return echo.NewHTTPError(http.StatusUnauthorized, map[string]any{"status": "error", "message": "Formato Bearer inválido"})
			}
			tokenStr = parts[1]
		} else {
			tokenStr = c.QueryParam("token")
		}
		claims, err := ValidarToken(tokenStr)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnauthorized, map[string]any{"status": "error", "message": err.Error()})
		}
		c.Set("claims", claims)
		return next(c)
	}
}

// OptionalJWTMiddleware deja pasar sin token como usuario anónimo (rol "U"); si hay token, debe ser válido.
func OptionalJWTMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Request().Header.Get("Authorization") == "" && c.QueryParam("token") == "" {
			c.Set("claims", &CustomClaims{Role: "U"})
			return next(c)
		}
		return JWTMiddleware(next)(c)
	}
}

// AdminMiddleware exige role "A"; va después de JWTMiddleware.
func AdminMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		claims, err := GetClaims(c)
		if err != nil {
			return err
		}
		if claims.Role != "A" {
			return echo.NewHTTPError(http.StatusForbidden, map[string]any{"status": "error", "message": "Se requieren privilegios de administrador"})
		}
		return next(c)
	}
}

// GetClaims obtiene los claims almacenados en el contexto de Echo.
func GetClaims(c echo.Context) (*CustomClaims, error) {
	claims, ok := c.Get("claims").(*CustomClaims)
	if !ok || claims == nil {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Token o datos de sesión no encontrados")
	}
	return claims, nil
}
