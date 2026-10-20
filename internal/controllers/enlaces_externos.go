package controllers

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

type enlaceExterno struct {
	DashID      int64   `json:"dashid"`
	Descripcion *string `json:"descripcion"`
	URL         *string `json:"url"`
	Target      string  `json:"target"`
	IsActive    int     `json:"is_active"`
}

type enlaceExternoInput struct {
	Descripcion string `json:"descripcion"`
	URL         string `json:"url"`
	Target      string `json:"target"`
	IsActive    int    `json:"is_active"`
}

// parseEnlaceExterno enlaza, normaliza y valida los campos recibidos; devuelve un error HTTP si no cumplen el contrato.
func parseEnlaceExterno(c echo.Context) (enlaceExternoInput, error) {
	input := enlaceExternoInput{Target: "_blank", IsActive: 1}
	if err := c.Bind(&input); err != nil {
		return input, apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	input.Descripcion = strings.TrimSpace(input.Descripcion)
	input.URL = strings.TrimSpace(input.URL)
	input.Target = strings.TrimSpace(input.Target)
	if input.Target == "" {
		input.Target = "_blank"
	}
	if input.Descripcion == "" || utf8.RuneCountInString(input.Descripcion) > 255 {
		return input, apiError(http.StatusBadRequest, "La descripción es obligatoria y admite hasta 255 caracteres")
	}
	if utf8.RuneCountInString(input.URL) == 0 || utf8.RuneCountInString(input.URL) > 255 {
		return input, apiError(http.StatusBadRequest, "La URL es obligatoria y admite hasta 255 caracteres")
	}
	parsed, err := url.ParseRequestURI(input.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return input, apiError(http.StatusBadRequest, "La URL debe comenzar con http:// o https://")
	}
	if input.Target != "_blank" && input.Target != "_self" {
		return input, apiError(http.StatusBadRequest, "El destino debe ser _blank o _self")
	}
	if input.IsActive != 0 && input.IsActive != 1 {
		return input, apiError(http.StatusBadRequest, "is_active debe ser 0 o 1")
	}
	return input, nil
}

// queryExternalLinks consulta los enlaces ordenados y, si activeOnly es true, limita el resultado a los activos; devuelve errores de consulta o lectura.
func queryExternalLinks(c echo.Context, activeOnly bool) ([]enlaceExterno, error) {
	query := `SELECT dashid, descripcion, url, target, is_active FROM enlaces_externos`
	if activeOnly {
		query += ` WHERE is_active = 1`
	}
	query += ` ORDER BY descripcion COLLATE NOCASE, dashid`
	rows, err := config.App.DB.QueryContext(c.Request().Context(), query)
	if err != nil {
		return nil, fmt.Errorf("consultar enlaces externos: %w", err)
	}
	defer rows.Close()

	items := make([]enlaceExterno, 0)
	for rows.Next() {
		var item enlaceExterno
		if err := rows.Scan(&item.DashID, &item.Descripcion, &item.URL, &item.Target, &item.IsActive); err != nil {
			return nil, fmt.Errorf("leer enlace externo: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("recorrer enlaces externos: %w", err)
	}
	return items, nil
}

// ListEnlacesExternos expone solo los enlaces activos para todos los visitantes.
func ListEnlacesExternos(c echo.Context) error {
	items, err := queryExternalLinks(c, true)
	if err != nil {
		log.Printf("enlaces externos: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudieron cargar los enlaces externos")
	}
	return c.JSON(http.StatusOK, items)
}

// ListEnlacesExternosAdmin lista enlaces activos e inactivos para administradores.
func ListEnlacesExternosAdmin(c echo.Context) error {
	items, err := queryExternalLinks(c, false)
	if err != nil {
		log.Printf("enlaces externos: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudieron cargar los enlaces externos")
	}
	return c.JSON(http.StatusOK, items)
}

// CreateEnlaceExterno agrega un enlace externo.
func CreateEnlaceExterno(c echo.Context) error {
	input, err := parseEnlaceExterno(c)
	if err != nil {
		return err
	}
	result, err := config.App.DB.ExecContext(c.Request().Context(),
		`INSERT INTO enlaces_externos (descripcion, url, target, is_active) VALUES (?, ?, ?, ?)`,
		input.Descripcion, input.URL, input.Target, input.IsActive)
	if err != nil {
		log.Printf("enlaces externos: no se pudo crear: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudo crear el enlace externo")
	}
	id, err := result.LastInsertId()
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudo obtener el identificador del enlace")
	}
	return c.JSON(http.StatusCreated, enlaceExterno{
		DashID: id, Descripcion: &input.Descripcion, URL: &input.URL,
		Target: input.Target, IsActive: input.IsActive,
	})
}

// UpdateEnlaceExterno actualiza un enlace por dashid.
func UpdateEnlaceExterno(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return apiError(http.StatusBadRequest, "Id inválido")
	}
	input, err := parseEnlaceExterno(c)
	if err != nil {
		return err
	}
	result, err := config.App.DB.ExecContext(c.Request().Context(),
		`UPDATE enlaces_externos SET descripcion = ?, url = ?, target = ?, is_active = ? WHERE dashid = ?`,
		input.Descripcion, input.URL, input.Target, input.IsActive, id)
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudo actualizar el enlace externo")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudo comprobar la actualización del enlace")
	}
	if rows == 0 {
		return apiError(http.StatusNotFound, "Enlace externo no encontrado")
	}
	return c.JSON(http.StatusOK, enlaceExterno{
		DashID: id, Descripcion: &input.Descripcion, URL: &input.URL,
		Target: input.Target, IsActive: input.IsActive,
	})
}

// DeleteEnlaceExterno elimina un enlace por dashid.
func DeleteEnlaceExterno(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return apiError(http.StatusBadRequest, "Id inválido")
	}
	result, err := config.App.DB.ExecContext(c.Request().Context(),
		`DELETE FROM enlaces_externos WHERE dashid = ?`, id)
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudo eliminar el enlace externo")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudo comprobar la eliminación del enlace")
	}
	if rows == 0 {
		return apiError(http.StatusNotFound, "Enlace externo no encontrado")
	}
	return c.NoContent(http.StatusNoContent)
}
