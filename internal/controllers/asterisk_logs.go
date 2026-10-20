package controllers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

type AsteriskOperationLog struct {
	ID         int64  `json:"id"`
	CreatedAt  string `json:"created_at"`
	Action     string `json:"action"`
	LocalNode  string `json:"local_node"`
	RemoteNode string `json:"remote_node"`
	Mode       string `json:"mode"`
	Permanent  bool   `json:"permanent"`
	Command    string `json:"command"`
	Result     string `json:"result"`
	Details    string `json:"details"`
	DurationMS int64  `json:"duration_ms"`
}

// recordAsteriskOperation guarda la operación y su duración; limita los detalles y registra en log los fallos de persistencia.
func recordAsteriskOperation(ctx context.Context, action, localNode, remoteNode, mode string, permanent bool, command, result, details string, startedAt time.Time) {
	if config.App.DB == nil {
		log.Printf("asterisk log: no se pudo guardar acción=%s local=%s remoto=%s: base de datos no disponible", action, localNode, remoteNode)
		return
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	durationMS := time.Since(startedAt).Milliseconds()
	if len(details) > 2000 {
		details = details[:2000]
	}
	permanentValue := 0
	if permanent {
		permanentValue = 1
	}
	_, err := config.App.DB.ExecContext(ctx, `
		INSERT INTO asterisk_operation_logs
		(created_at, action, local_node, remote_node, mode, permanent, command, result, details, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		createdAt, action, localNode, remoteNode, mode, permanentValue, command, result, details, durationMS)
	if err != nil {
		log.Printf("asterisk log: no se pudo guardar acción=%s local=%s remoto=%s: %v", action, localNode, remoteNode, err)
	}
}

// GetAsteriskOperationLogs devuelve las operaciones recientes de Asterisk y valida el límite solicitado.
func GetAsteriskOperationLogs(c echo.Context) error {
	if config.App.DB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"status": "error", "message": "La base de datos no está disponible."})
	}
	limit := 500
	if value := strings.TrimSpace(c.QueryParam("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "limit debe ser un entero positivo."})
		}
		limit = parsed
	}
	if limit > 2000 {
		limit = 2000
	}
	rows, err := config.App.DB.QueryContext(c.Request().Context(), `
		SELECT id, created_at, action, local_node, remote_node, mode, permanent, command, result, details, duration_ms
		FROM asterisk_operation_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo consultar el registro de Asterisk."})
	}
	defer rows.Close()
	logs := make([]AsteriskOperationLog, 0)
	for rows.Next() {
		var entry AsteriskOperationLog
		var permanent int
		if err := rows.Scan(&entry.ID, &entry.CreatedAt, &entry.Action, &entry.LocalNode, &entry.RemoteNode, &entry.Mode, &permanent, &entry.Command, &entry.Result, &entry.Details, &entry.DurationMS); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo leer el registro de Asterisk."})
		}
		entry.Permanent = permanent != 0
		logs = append(logs, entry)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo recorrer el registro de Asterisk."})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "success", "data": logs})
}
