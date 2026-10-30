package controllers

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

const minPasswordLen = 8

// apiError crea un error HTTP de Echo con el código y el mensaje indicados.
func apiError(code int, msg string) error {
	return echo.NewHTTPError(code, map[string]any{"status": "error", "message": msg})
}

// setAdminPassword genera el hash bcrypt y lo guarda; devuelve errores de hash o de base de datos.
func setAdminPassword(db *sql.DB, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE admin SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = 1`, string(hash))
	return err
}

var reUsuario = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

// AdminConfigured indica si el administrador ya definió su contraseña.
func AdminConfigured(db *sql.DB) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(1) FROM admin`).Scan(&n)
	return n > 0, err
}

// ClearAdminPassword borra la contraseña; la web vuelve a pedir una nueva (reset desde root).
func ClearAdminPassword(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM terminal_credentials`); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.Exec(`DELETE FROM admin`); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// AdminStatus informa si hay contraseña definida, para que la web ofrezca crearla o iniciar sesión.
func AdminStatus(c echo.Context) error {
	ok, err := AdminConfigured(config.App.DB)
	if err != nil {
		return apiError(http.StatusInternalServerError, "Error de base de datos")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "ok", "configured": ok})
}

// SetupAdminPassword define la primera contraseña; solo funciona mientras no exista ninguna.
func SetupAdminPassword(c echo.Context) error {
	var req struct {
		User string `json:"user"`
		New  string `json:"new"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	if !reUsuario.MatchString(req.User) {
		return apiError(http.StatusBadRequest, "El usuario debe tener de 3 a 32 caracteres (letras, números, . _ -)")
	}
	if len(req.New) < minPasswordLen {
		return apiError(http.StatusBadRequest, "La contraseña debe tener al menos 8 caracteres")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.New), bcrypt.DefaultCost)
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudo guardar la contraseña")
	}
	res, err := config.App.DB.Exec(`INSERT OR IGNORE INTO admin (id, username, password_hash) VALUES (1, ?, ?)`, req.User, string(hash))
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudo guardar la contraseña")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apiError(http.StatusConflict, "La contraseña ya está definida")
	}
	tok, err := GenerateAdminToken()
	if err != nil {
		return apiError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "ok", "token": tok})
}

// checkAdminPassword compara la contraseña con el hash almacenado y devuelve false si no puede validarla.
func checkAdminPassword(db *sql.DB, password string) bool {
	var hash string
	if err := db.QueryRow(`SELECT password_hash FROM admin WHERE id = 1`).Scan(&hash); err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// AdminLogin valida usuario y contraseña y devuelve el token único de administrador.
func AdminLogin(c echo.Context) error {
	var req struct {
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	var user, hash string
	_ = config.App.DB.QueryRow(`SELECT username, password_hash FROM admin WHERE id = 1`).Scan(&user, &hash)
	// bcrypt se ejecuta siempre para no revelar si el usuario existe por el tiempo de respuesta.
	okPass := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) == nil
	if hash == "" || !okPass || !strings.EqualFold(user, req.User) {
		time.Sleep(time.Second) // frena la fuerza bruta
		return apiError(http.StatusUnauthorized, "Credenciales inválidas")
	}
	tok, err := GenerateAdminToken()
	if err != nil {
		return apiError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "ok", "token": tok})
}

// ChangeAdminPassword cambia la contraseña tras verificar la actual.
func ChangeAdminPassword(c echo.Context) error {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	if len(req.New) < minPasswordLen {
		return apiError(http.StatusBadRequest, "La nueva contraseña debe tener al menos 8 caracteres")
	}
	if !checkAdminPassword(config.App.DB, req.Current) {
		time.Sleep(time.Second)
		return apiError(http.StatusUnauthorized, "Contraseña actual incorrecta")
	}
	if err := setAdminPassword(config.App.DB, req.New); err != nil {
		return apiError(http.StatusInternalServerError, "no se pudo guardar la contraseña")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
}
