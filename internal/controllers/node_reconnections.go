package controllers

// Este archivo implementa la configuración y ejecución de reconexiones automáticas de nodos.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// nodeReconnection representa una configuración de reconexión automática para un nodo, incluyendo el nodo fuente, el nodo objetivo, el intervalo de reconexión y el estado de la última verificación.
type nodeReconnection struct {
	ID              int64  `json:"id"`
	SourceNode      string `json:"source_node"`
	Node            string `json:"node"`
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"interval_seconds"`
	Action          string `json:"action"`
	Permanent       bool   `json:"permanent"`
	NextCheckAt     int64  `json:"next_check_at"`
	LastCheckAt     int64  `json:"last_check_at"`
	LastResult      string `json:"last_result"`
	LastDetails     string `json:"last_details"`
}

// nodeReconnectionMu protege el acceso concurrente a las configuraciones de reconexión de nodos.
var nodeReconnectionMu sync.Mutex

// nodeReconnectionCycleMu protege el ciclo de verificación de reconexiones de nodos.
var nodeReconnectionCycleMu sync.Mutex

// loadNodeReconnections carga las reconexiones de nodos desde la base de datos, devolviendo una lista de entradas y cualquier error encontrado.
func loadNodeReconnections(ctx context.Context) ([]nodeReconnection, error) {
	if config.App.DB == nil {
		return nil, fmt.Errorf("base de datos no disponible")
	}
	rows, err := config.App.DB.QueryContext(ctx, `SELECT id, source_node, node, enabled, interval_seconds,
		action, permanent, next_check_at, last_check_at, last_result, last_details
		FROM node_reconnections ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]nodeReconnection, 0)
	for rows.Next() {
		var entry nodeReconnection
		if err := rows.Scan(&entry.ID, &entry.SourceNode, &entry.Node, &entry.Enabled, &entry.IntervalSeconds,
			&entry.Action, &entry.Permanent, &entry.NextCheckAt, &entry.LastCheckAt,
			&entry.LastResult, &entry.LastDetails); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// ListNodeReconnections maneja la solicitud para listar todas las reconexiones de nodos.
func ListNodeReconnections(c echo.Context) error {
	entries, err := loadNodeReconnections(c.Request().Context())
	if err != nil {
		return reconnectionDBError(err)
	}
	return c.JSON(http.StatusOK, echo.Map{"entries": entries})
}

// reconnectionDBError maneja los errores de la base de datos relacionados con las reconexiones de nodos, devolviendo un error API adecuado.
func reconnectionDBError(err error) error {
	log.Printf("reconnections: base de datos: %v", err)
	if se, ok := err.(*sqlite.Error); ok && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return apiError(http.StatusConflict, "RECONNECT_DUPLICATE")
	}
	return apiError(http.StatusInternalServerError, "RECONNECT_DB_ERROR")
}

// readNodeReconnection lee y valida una entrada de reconexión de nodo desde la solicitud, devolviendo la entrada y cualquier error encontrado.
func readNodeReconnection(c echo.Context) (nodeReconnection, error) {
	var entry nodeReconnection
	if err := c.Bind(&entry); err != nil {
		return entry, apiError(http.StatusBadRequest, "RECONNECT_INVALID")
	}
	entry.Node = strings.TrimSpace(entry.Node)
	entry.SourceNode = strings.TrimSpace(entry.SourceNode)
	if !isNumericNode(entry.Node) || len(entry.Node) > 10 ||
		(entry.SourceNode != "" && (!isNumericNode(entry.SourceNode) || len(entry.SourceNode) > 10)) ||
		entry.IntervalSeconds < 1 || entry.IntervalSeconds > 86400 || !isRPTConnectionAction(entry.Action) {
		return entry, apiError(http.StatusBadRequest, "RECONNECT_INVALID")
	}
	return entry, nil
}

// SaveNodeReconnection maneja la solicitud para guardar una reconexión de nodo, validando la entrada y asegurando la exclusión mutua.
func SaveNodeReconnection(c echo.Context) error {
	entry, err := readNodeReconnection(c)
	if err != nil {
		return err
	}
	if !nodeReconnectionMu.TryLock() {
		return apiError(http.StatusConflict, "RECONNECT_BUSY")
	}
	defer nodeReconnectionMu.Unlock()
	if config.App.DB == nil {
		return reconnectionDBError(fmt.Errorf("base de datos no disponible"))
	}
	source := entry.SourceNode
	if source == "" {
		source, err = newPrimaryNodeControl().primary()
		if err != nil {
			log.Printf("reconnections: no se pudo identificar principal al guardar: %v", err)
			return apiError(http.StatusServiceUnavailable, "RECONNECT_PRIMARY_ERROR")
		}
	}
	if entry.Node == source {
		return apiError(http.StatusConflict, "RECONNECT_SELF")
	}
	if entry.Enabled {
		if _, err := resolveReconnectionSource(entry.SourceNode); err != nil {
			log.Printf("reconnections: fuente no disponible al guardar: %v", err)
			return c.JSON(http.StatusServiceUnavailable, echo.Map{
				"status": "error", "message": "RECONNECT_SOURCE_SAVE_ERROR",
				"source_node": source, "destination_node": entry.Node,
			})
		}
	}
	ctx := c.Request().Context()
	var id int64
	if c.Param("id") != "" {
		id, err = strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id < 1 {
			return apiError(http.StatusBadRequest, "RECONNECT_INVALID")
		}
	}
	// Las entradas anteriores conservan '' como alias del principal actual.
	primary, primaryErr := newPrimaryNodeControl().primary()
	var duplicate bool
	if primaryErr == nil {
		err = config.App.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM node_reconnections
			WHERE id != ? AND node = ? AND COALESCE(NULLIF(source_node, ''), ?) = ?)`,
			id, entry.Node, primary, source).Scan(&duplicate)
	} else {
		err = config.App.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM node_reconnections
			WHERE id != ? AND node = ? AND source_node = ?)`, id, entry.Node, source).Scan(&duplicate)
	}
	if err != nil {
		return reconnectionDBError(err)
	}
	if duplicate {
		return apiError(http.StatusConflict, "RECONNECT_DUPLICATE")
	}
	var result sql.Result
	if c.Param("id") == "" {
		result, err = config.App.DB.ExecContext(ctx, `INSERT INTO node_reconnections
			(source_node, node, enabled, interval_seconds, action, permanent) VALUES (?, ?, ?, ?, ?, ?)`,
			entry.SourceNode, entry.Node, entry.Enabled, entry.IntervalSeconds, entry.Action, entry.Permanent)
	} else {
		result, err = config.App.DB.ExecContext(ctx, `UPDATE node_reconnections
			SET source_node = ?, node = ?, enabled = ?, interval_seconds = ?, action = ?, permanent = ?,
			next_check_at = 0, last_check_at = 0, last_result = 'RECONNECT_PENDING', last_details = '' WHERE id = ?`,
			entry.SourceNode, entry.Node, entry.Enabled, entry.IntervalSeconds, entry.Action, entry.Permanent, id)
	}
	if err != nil {
		return reconnectionDBError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return reconnectionDBError(err)
	}
	if count == 0 {
		return apiError(http.StatusNotFound, "RECONNECT_NOT_FOUND")
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "ok"})
}

// DeleteNodeReconnection maneja la solicitud para eliminar una reconexión de nodo, asegurando la exclusión mutua.
func DeleteNodeReconnection(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return apiError(http.StatusBadRequest, "RECONNECT_INVALID")
	}
	if !nodeReconnectionMu.TryLock() {
		return apiError(http.StatusConflict, "RECONNECT_BUSY")
	}
	defer nodeReconnectionMu.Unlock()
	if config.App.DB == nil {
		return reconnectionDBError(fmt.Errorf("base de datos no disponible"))
	}
	result, err := config.App.DB.ExecContext(c.Request().Context(), "DELETE FROM node_reconnections WHERE id = ?", id)
	if err != nil {
		return reconnectionDBError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return reconnectionDBError(err)
	}
	if count == 0 {
		return apiError(http.StatusNotFound, "RECONNECT_NOT_FOUND")
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "ok"})
}

// StartNodeReconnectionScheduler inicia el programador de reconexiones de nodos, ejecutando ciclos de verificación periódicos.
func StartNodeReconnectionScheduler(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			runNodeReconnectionCycle(ctx, time.Now())
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

// runNodeReconnectionCycle ejecuta un ciclo de verificación de reconexiones de nodos, procesando todas las entradas que requieren acción.
func runNodeReconnectionCycle(ctx context.Context, now time.Time) {
	if ctx.Err() != nil || !nodeReconnectionCycleMu.TryLock() {
		return
	}
	defer nodeReconnectionCycleMu.Unlock()
	entries, err := loadNodeReconnections(ctx)
	if err != nil {
		log.Printf("reconnections: no se pudo consultar la lista: %v", err)
		return
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if !entry.Enabled || entry.NextCheckAt > now.Unix() {
			continue
		}
		runNodeReconnectionEntry(ctx, entry, now)
	}
}

// runNodeReconnectionEntry ejecuta la reconexión de un nodo específico si corresponde, actualizando los tiempos de verificación y manejando errores.
func runNodeReconnectionEntry(ctx context.Context, entry nodeReconnection, now time.Time) {
	nodeReconnectionMu.Lock()
	defer nodeReconnectionMu.Unlock()
	// Una edición o eliminación entre entradas invalida la copia cargada al comenzar el ciclo.
	err := config.App.DB.QueryRowContext(ctx, `SELECT source_node, node, enabled, interval_seconds, action, permanent, next_check_at
		FROM node_reconnections WHERE id = ?`, entry.ID).Scan(
		&entry.SourceNode, &entry.Node, &entry.Enabled, &entry.IntervalSeconds, &entry.Action, &entry.Permanent, &entry.NextCheckAt)
	if err == sql.ErrNoRows {
		return
	}
	if err != nil {
		log.Printf("reconnections: no se pudo releer nodo %s: %v", entry.Node, err)
		return
	}
	if !entry.Enabled || entry.NextCheckAt > now.Unix() || ctx.Err() != nil {
		return
	}
	// Reservar antes de AMI evita repetir inmediatamente tras un reinicio o un fallo.
	started := time.Now()
	next := started.Unix() + int64(entry.IntervalSeconds)
	if _, err := config.App.DB.ExecContext(ctx, "UPDATE node_reconnections SET next_check_at = ? WHERE id = ?", next, entry.ID); err != nil {
		log.Printf("reconnections: no se pudo reservar nodo %s: %v", entry.Node, err)
		return
	}
	result, details := checkNodeReconnection(ctx, entry)
	next = time.Now().Unix() + int64(entry.IntervalSeconds)
	if _, err := config.App.DB.ExecContext(ctx, `UPDATE node_reconnections
		SET next_check_at = ?, last_check_at = ?, last_result = ?, last_details = ? WHERE id = ?`,
		next, started.Unix(), result, details, entry.ID); err != nil {
		log.Printf("reconnections: no se pudo guardar resultado del nodo %s: %v", entry.Node, err)
	}
	if result != "RECONNECT_PRESENT" {
		log.Printf("reconnections: nodo=%s resultado=%s detalle=%s", entry.Node, result, details)
	}
}

// checkNodeReconnection realiza la verificación de reconexión para un nodo específico, devolviendo el resultado y los detalles.
func checkNodeReconnection(ctx context.Context, entry nodeReconnection) (string, string) {
	if !systemControlLock.TryLock() {
		return "RECONNECT_BUSY", ""
	}
	defer systemControlLock.Unlock()
	if !rptConnectionSafety.TryLock() {
		return "RECONNECT_BUSY", ""
	}
	defer rptConnectionSafety.Unlock()
	instance, err := resolveReconnectionSource(entry.SourceNode)
	if err != nil {
		return "RECONNECT_SOURCE_ERROR", err.Error()
	}
	source := instance.Node
	if source == entry.Node {
		return "RECONNECT_SELF", ""
	}
	raw, err := config.ExecuteAMIRptStatus(instance.Host, instance.Port, instance.User, instance.Secret, source, "XStat", 5*time.Second)
	if err != nil {
		return "RECONNECT_STATUS_ERROR", err.Error()
	}
	if err := addRPTVisibleLinks(make(rptLinkGraph), source, raw); err != nil {
		return "RECONNECT_STATUS_ERROR", err.Error()
	}
	for _, link := range parseRPTConnections(raw, "") {
		if link.Node == entry.Node {
			return "RECONNECT_PRESENT", ""
		}
	}
	// Sin modalidad deseada, la validación nunca desconecta ni cambia un enlace existente.
	if _, err := validateRPTConnection(ctx, instance, entry.Node); err != nil {
		return "RECONNECT_REJECTED", err.Error()
	}
	if ctx.Err() != nil {
		return "RECONNECT_BUSY", ""
	}
	request := SupermonCommandRequest{LocalNode: source, RemoteNode: entry.Node, Action: entry.Action, Permanent: entry.Permanent}
	command, err := buildSupermonCommand(request, newSupermonCommandDefinition(entry.Action, "", ""))
	if err != nil {
		return "RECONNECT_INVALID", err.Error()
	}
	started := time.Now()
	raw, err = config.ExecuteAMICommand(instance.Host, instance.Port, instance.User, instance.Secret, command, 5*time.Second)
	if err == nil {
		err = audioPlaybackOutputError(raw)
	}
	if err != nil {
		recordAsteriskOperation(ctx, entry.Action, source, entry.Node, "", entry.Permanent, command, "ami_error", "Reconexión automática: "+err.Error(), started)
		return "RECONNECT_COMMAND_ERROR", err.Error()
	}
	rememberPendingRPTLink(instance, entry.Node, 15*time.Second)
	recordAsteriskOperation(ctx, entry.Action, source, entry.Node, "", entry.Permanent, command, "not_confirmed", "Reconexión automática solicitada; se comprobará el enlace en el siguiente ciclo", started)
	return "RECONNECT_REQUESTED", ""
}

// resolveReconnectionSource determina la instancia de Allmon correspondiente al nodo fuente, devolviendo un error si no se encuentra o si la configuración está incompleta.
func resolveReconnectionSource(source string) (allmonInstance, error) {
	if source == "" {
		var err error
		source, err = newPrimaryNodeControl().primary()
		if err != nil {
			return allmonInstance{}, err
		}
	}
	instances, err := allStarLinkActivityInstances()
	if err != nil {
		return allmonInstance{}, err
	}
	for _, instance := range instances {
		if instance.Node != source {
			continue
		}
		if instance.Host == "" || instance.Port == "" || instance.User == "" {
			return allmonInstance{}, fmt.Errorf("la configuración AMI de la fuente %s está incompleta", source)
		}
		return instance, nil
	}
	return allmonInstance{}, fmt.Errorf("el nodo fuente %s ya no está en la lista de servidores AllStarLink", source)
}
