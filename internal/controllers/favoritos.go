package controllers

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Mismo formato que acepta el campo "nodo remoto" de la conexión.
var reFavoritoNumero = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,31}$`)

type favorito struct {
	ID            int64  `json:"id"`
	Numero        string `json:"numero"`
	Descripcion   string `json:"descripcion"`
	DescripcionDB string `json:"descripcion_astdb,omitempty"`
}

// astdbTexto arma "indicativo descripción ubicación", igual que la información del monitor.
func astdbTexto(e AstDBEntry) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{e.Callsign, e.Descripcion, e.Ubicacion} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

// astdbEntries carga el archivo AstDB configurado y devuelve nil si la ruta falta o no puede leerse.
func astdbEntries() map[string]AstDBEntry {
	path := strings.TrimSpace(config.App.Sources.AstDBPath)
	if path == "" {
		return nil
	}
	entries, err := readAstDBEntries(path)
	if err != nil {
		return nil
	}
	return entries
}

// leerFavorito enlaza, normaliza y valida el favorito solicitado; devuelve errores HTTP ante datos inválidos.
func leerFavorito(c echo.Context) (favorito, error) {
	var f favorito
	if err := c.Bind(&f); err != nil {
		return f, apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	f.Numero, f.Descripcion = strings.TrimSpace(f.Numero), strings.TrimSpace(f.Descripcion)
	if !reFavoritoNumero.MatchString(f.Numero) {
		return f, apiError(http.StatusBadRequest, "El número del nodo no es válido")
	}
	if len(f.Descripcion) > 120 {
		return f, apiError(http.StatusBadRequest, "La descripción admite hasta 120 caracteres")
	}
	return f, nil
}

// errFavorito traduce conflictos de unicidad a HTTP 409 y otros errores SQL a HTTP 500.
func errFavorito(err error) error {
	if se, ok := err.(*sqlite.Error); ok && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return apiError(http.StatusConflict, "Ese número ya está en favoritos")
	}
	return apiError(http.StatusInternalServerError, "Error de base de datos")
}

// ListFavoritos devuelve los nodos favoritos ordenados por descripción.
func ListFavoritos(c echo.Context) error {
	rows, err := config.App.DB.QueryContext(c.Request().Context(),
		`SELECT id, numero, descripcion FROM favoritos ORDER BY descripcion COLLATE NOCASE, numero`)
	if err != nil {
		return errFavorito(err)
	}
	defer rows.Close()
	entries := astdbEntries()
	items := []favorito{}
	for rows.Next() {
		var f favorito
		if err := rows.Scan(&f.ID, &f.Numero, &f.Descripcion); err != nil {
			return errFavorito(err)
		}
		if e, ok := entries[f.Numero]; ok {
			f.DescripcionDB = astdbTexto(e)
		}
		items = append(items, f)
	}
	return c.JSON(http.StatusOK, items)
}

// BuscarAstdbFavorito busca números, indicativos y descripciones que coinciden con el texto.
func BuscarAstdbFavorito(c echo.Context) error {
	query := strings.TrimSpace(c.QueryParam("q"))
	if query == "" || len(query) > 120 || strings.ContainsAny(query, "\r\n\x00") {
		return apiError(http.StatusBadRequest, "Introduce un texto válido para buscar")
	}
	path := strings.TrimSpace(config.App.Sources.AstDBPath)
	if path == "" {
		return apiError(http.StatusInternalServerError, "La ruta de astdb.txt no está configurada")
	}
	results, err := buscarCoincidenciasAstDB(path, query)
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudo leer el archivo astdb.txt")
	}
	return c.JSON(http.StatusOK, results)
}

func buscarCoincidenciasAstDB(path, query string) ([]AstDBEntry, error) {
	entries, err := readAstDBEntries(path)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(query)
	results := make([]AstDBEntry, 0)
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Nodo), needle) ||
			strings.Contains(strings.ToLower(entry.Callsign), needle) ||
			strings.Contains(strings.ToLower(entry.Descripcion), needle) {
			results = append(results, entry)
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Nodo < results[j].Nodo })
	return results, nil
}

// AstdbFavorito devuelve la descripción del nodo en astdb.txt, para sugerirla al crear un favorito.
func AstdbFavorito(c echo.Context) error {
	numero := strings.TrimSpace(c.Param("numero"))
	if !reFavoritoNumero.MatchString(numero) {
		return apiError(http.StatusBadRequest, "El número del nodo no es válido")
	}
	desc := ""
	if e, ok := astdbEntries()[numero]; ok {
		desc = astdbTexto(e)
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "descripcion": desc})
}

// CreateFavorito valida y agrega un favorito, y devuelve el registro creado con su identificador.
func CreateFavorito(c echo.Context) error {
	f, err := leerFavorito(c)
	if err != nil {
		return err
	}
	res, err := config.App.DB.ExecContext(c.Request().Context(),
		`INSERT INTO favoritos (numero, descripcion) VALUES (?, ?)`, f.Numero, f.Descripcion)
	if err != nil {
		return errFavorito(err)
	}
	f.ID, _ = res.LastInsertId()
	return c.JSON(http.StatusCreated, f)
}

// UpdateFavorito valida y actualiza el favorito indicado, o devuelve 404 si no existe.
func UpdateFavorito(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, "Id inválido")
	}
	f, err := leerFavorito(c)
	if err != nil {
		return err
	}
	res, err := config.App.DB.ExecContext(c.Request().Context(),
		`UPDATE favoritos SET numero = ?, descripcion = ? WHERE id = ?`, f.Numero, f.Descripcion, id)
	if err != nil {
		return errFavorito(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apiError(http.StatusNotFound, "Favorito no encontrado")
	}
	f.ID = id
	return c.JSON(http.StatusOK, f)
}

// DeleteFavorito elimina el favorito indicado y devuelve 404 si no existe.
func DeleteFavorito(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return apiError(http.StatusBadRequest, "Id inválido")
	}
	res, err := config.App.DB.ExecContext(c.Request().Context(), `DELETE FROM favoritos WHERE id = ?`, id)
	if err != nil {
		return errFavorito(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apiError(http.StatusNotFound, "Favorito no encontrado")
	}
	return c.NoContent(http.StatusNoContent)
}
