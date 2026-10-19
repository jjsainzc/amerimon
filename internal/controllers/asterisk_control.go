package controllers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"amerimon/internal/config"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type SupermonCommand struct {
	ID                       string `json:"id"`
	Label                    string `json:"label"`
	CommandTemplate          string `json:"command_template"`
	PermanentCommandTemplate string `json:"permanent_command_template,omitempty"`
	RequiresRemote           bool   `json:"requires_remote_node"`
	SupportsPermanent        bool   `json:"supports_permanent"`
}

type SupermonCommandRequest struct {
	LocalNode      string `json:"local_node"`
	RemoteNode     string `json:"remote_node"`
	Action         string `json:"action"`
	Permanent      bool   `json:"permanent"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type NodeConnectionRequest struct {
	LocalNode  string `json:"local_node"`
	RemoteNode string `json:"remote_node"`
	Mode       string `json:"mode"`
}

type supermonCommandDefinition struct {
	SupermonCommand
	command string
}

// ListSupermonCommands devuelve los comandos Asterisk activos con sus plantillas de uso.
// ListSupermonCommands godoc
// @Summary Listar comandos disponibles de Supermon
// @Tags Asterisk
// @Security BearerAuth
// @Produce json
// @Success 200 {array} SupermonCommand
// @Failure 401
// @Failure 403
// @Router /asterisk/commands [get]
func ListSupermonCommands(c echo.Context) error {
	definitions, err := loadSupermonCommands(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudieron leer los comandos de Asterisk"})
	}
	commands := make([]SupermonCommand, 0, len(definitions))
	for _, definition := range definitions {
		command := definition.SupermonCommand
		command.CommandTemplate = supermonCommandTemplate(definition.ID, false, definition.command)
		if definition.SupportsPermanent {
			command.PermanentCommandTemplate = supermonCommandTemplate(definition.ID, true, definition.command)
		}
		commands = append(commands, command)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   commands,
	})
}

// loadSupermonCommands carga de la base de datos todos los comandos Asterisk activos.
func loadSupermonCommands(ctx context.Context) ([]supermonCommandDefinition, error) {
	if config.App.DB == nil {
		return nil, fmt.Errorf("base de datos no configurada")
	}
	rows, err := config.App.DB.QueryContext(ctx, "SELECT id, label, command FROM asterisk_commands WHERE active = 1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	commands := make([]supermonCommandDefinition, 0)
	for rows.Next() {
		var id string
		var label, command sql.NullString
		if err := rows.Scan(&id, &label, &command); err != nil {
			return nil, err
		}
		commands = append(commands, newSupermonCommandDefinition(id, label.String, command.String))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return commands, nil
}

// loadSupermonCommand busca la definición activa de una acción de Supermon.
func loadSupermonCommand(ctx context.Context, action string) (supermonCommandDefinition, error) {
	if config.App.DB == nil {
		return supermonCommandDefinition{}, fmt.Errorf("base de datos no configurada")
	}
	var id string
	var label, command sql.NullString
	query := "SELECT id, label, command FROM asterisk_commands WHERE id = ? AND active = 1"
	if isBuiltInSupermonConnectionAction(action) {
		query = "SELECT id, label, command FROM asterisk_commands WHERE id = ?"
	}
	err := config.App.DB.QueryRowContext(ctx, query, action).Scan(&id, &label, &command)
	if err != nil {
		return supermonCommandDefinition{}, err
	}
	return newSupermonCommandDefinition(id, label.String, command.String), nil
}

// isBuiltInSupermonConnectionAction indica si la acción es una conexión integrada de Supermon.
func isBuiltInSupermonConnectionAction(action string) bool {
	switch action {
	case "connect", "monitor", "localmonitor", "disconnect":
		return true
	default:
		return false
	}
}

// newSupermonCommandDefinition crea una definición y sustituye el comando de acciones integradas.
func newSupermonCommandDefinition(id, label, command string) supermonCommandDefinition {
	requiresRemote := isBuiltInSupermonConnectionAction(id)
	if requiresRemote {
		command = builtInSupermonConnectionCommand(id)
	}
	return supermonCommandDefinition{
		SupermonCommand: SupermonCommand{
			ID:                id,
			Label:             label,
			RequiresRemote:    requiresRemote,
			SupportsPermanent: requiresRemote,
		},
		command: command,
	}
}

// builtInSupermonConnectionCommand devuelve la plantilla AMI para una acción de conexión integrada.
func builtInSupermonConnectionCommand(action string) string {
	switch action {
	case "connect", "monitor", "localmonitor":
		return "rpt cmd %s ilink %d %s"
	case "disconnect":
		return "rpt cmd %s ilink 11 %s"
	default:
		return ""
	}
}

// supermonCommandTemplate genera la plantilla visible para ejecutar una acción de Supermon.
func supermonCommandTemplate(action string, permanent bool, fallback string) string {
	if isBuiltInSupermonConnectionAction(action) {
		command := strings.Replace(fallback, "%s", "{local_node}", 1)
		if action != "disconnect" {
			code := supermonConnectionCode(action, permanent)
			command = strings.Replace(command, "%d", fmt.Sprint(code), 1)
		}
		return strings.Replace(command, "%s", "{remote_node}", 1)
	}
	return strings.ReplaceAll(fallback, "%s", "{local_node}")
}

// supermonConnectionCode obtiene el código ilink según la acción y su permanencia.
func supermonConnectionCode(action string, permanent bool) int {
	code := 0
	switch action {
	case "connect":
		code = 3
		if permanent {
			code = 13
		}
	case "monitor":
		code = 2
		if permanent {
			code = 12
		}
	case "localmonitor":
		code = 8
		if permanent {
			code = 18
		}
	}
	return code
}

// ExecuteSupermonCommand valida una acción de Supermon, la ejecuta en Asterisk y responde al cliente.
// ExecuteSupermonCommand godoc
// @Summary Ejecutar un comando de Supermon
// @Tags Asterisk
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body SupermonCommandRequest true "Acción y nodos involucrados"
// @Success 200
// @Failure 401
// @Failure 403
// @Router /asterisk/commands [post]
func ExecuteSupermonCommand(c echo.Context) error {
	var request SupermonCommandRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "JSON inválido"})
	}
	if !isNumericNode(request.LocalNode) {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "local_node debe ser numérico"})
	}

	definition, err := loadSupermonCommand(c.Request().Context(), request.Action)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "acción desconocida"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo leer el comando de Asterisk"})
	}
	command, err := buildSupermonCommand(request, definition)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": err.Error()})
	}
	instance, allmonConfigured, err := findAllmonInstance(request.LocalNode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo leer la configuración de Allmon"})
	}
	if allmonConfigured && instance.Node == "" {
		return c.JSON(http.StatusNotFound, map[string]any{"status": "error", "message": "El nodo local no existe en allmon.ini"})
	}
	if !allmonConfigured {
		ami := config.App.AMI
		instance = allmonInstance{Node: request.LocalNode, Host: ami.Host, Port: ami.Port, User: ami.User, Secret: ami.Secret}
	}
	if instance.Host == "" || instance.Port == "" || instance.User == "" || (!allmonConfigured && instance.Secret == "") {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "La configuración AMI de Asterisk está incompleta"})
	}

	raw, err := config.ExecuteAMICommand(instance.Host, instance.Port, instance.User, instance.Secret, command, 5*time.Second)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{"status": "error", "command": command, "message": "No se pudo ejecutar el comando en Asterisk"})
	}
	if shouldWaitForRPTConnection(request.Action, request.Permanent) && !waitForRPTConnection(instance, request.RemoteNode, resolveRPTConnectionTimeout(request.TimeoutSeconds)) {
		return c.JSON(http.StatusGatewayTimeout, map[string]any{
			"status":  "error",
			"command": command,
			"message": fmt.Sprintf("No se pudo conectar el nodo %s en el servidor %s dentro del tiempo esperado.", request.RemoteNode, request.LocalNode),
		})
	}
	lines := config.ParseAMIOutputToLines(raw)
	if len(lines) == 0 {
		lines = []string{"No data returned"}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"action":  request.Action,
			"command": command,
			"lines":   lines,
		},
	})
}

// isRPTConnectionAction indica si la acción inicia una conexión RPT que debe confirmarse.
func isRPTConnectionAction(action string) bool {
	switch action {
	case "connect", "monitor", "localmonitor":
		return true
	default:
		return false
	}
}

// shouldWaitForRPTConnection determina si hay que esperar la conexión RPT tras ejecutar la acción.
func shouldWaitForRPTConnection(action string, permanent bool) bool {
	return isRPTConnectionAction(action) && !permanent
}

// resolveRPTConnectionTimeout normaliza el timeout solicitado y lo convierte a duración.
func resolveRPTConnectionTimeout(seconds int) time.Duration {
	if seconds < 1 {
		seconds = 15
	}
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

// waitForRPTConnection consulta XStat hasta confirmar el enlace o agotar el timeout.
func waitForRPTConnection(instance allmonInstance, remoteNode string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}

		queryTimeout := 5 * time.Second
		if remaining < queryTimeout {
			queryTimeout = remaining
		}
		xstat, err := config.ExecuteAMIRptStatus(instance.Host, instance.Port, instance.User, instance.Secret, instance.Node, "XStat", queryTimeout)
		if err == nil && hasEstablishedRPTConnection(xstat, remoteNode) {
			return true
		}

		remaining = time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		pause := time.Second
		if remaining < pause {
			pause = remaining
		}
		time.Sleep(pause)
	}
}

// hasEstablishedRPTConnection comprueba si XStat muestra el nodo remoto como establecido.
func hasEstablishedRPTConnection(xstat, remoteNode string) bool {
	for _, connection := range parseRPTConnections(xstat, "") {
		if connection.Node == remoteNode && connection.Link == "ESTABLISHED" {
			return true
		}
	}
	return false
}

// AddNodeConnection valida y ejecuta en Asterisk una conexión solicitada entre nodos.
// AddNodeConnection godoc
// @Summary Registrar una conexión de nodo Asterisk
// @Tags Asterisk
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body NodeConnectionRequest true "Nodos y modo de conexión"
// @Success 200
// @Failure 401
// @Failure 403
// @Router /asterisk/connections [post]
func AddNodeConnection(c echo.Context) error {
	var request NodeConnectionRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "JSON inválido"})
	}
	command, err := buildNodeConnectionCommand(request)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": err.Error()})
	}
	instance, allmonConfigured, err := findAllmonInstance(request.LocalNode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo leer la configuración de Allmon"})
	}
	if allmonConfigured && instance.Node == "" {
		return c.JSON(http.StatusNotFound, map[string]any{"status": "error", "message": "El nodo local no existe en allmon.ini"})
	}
	if !allmonConfigured {
		ami := config.App.AMI
		instance = allmonInstance{Node: request.LocalNode, Host: ami.Host, Port: ami.Port, User: ami.User, Secret: ami.Secret}
	}
	if instance.Host == "" || instance.Port == "" || instance.User == "" || (!allmonConfigured && instance.Secret == "") {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "La configuración AMI de Asterisk está incompleta"})
	}

	raw, err := config.ExecuteAMICommand(instance.Host, instance.Port, instance.User, instance.Secret, command, 5*time.Second)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]any{"status": "error", "command": command, "message": "No se pudo agregar la conexión en Asterisk"})
	}
	lines := config.ParseAMIOutputToLines(raw)
	if len(lines) == 0 {
		lines = []string{"No data returned"}
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"action":      "add_connection",
			"local_node":  request.LocalNode,
			"remote_node": request.RemoteNode,
			"mode":        request.Mode,
			"command":     command,
			"lines":       lines,
		},
	})
}

// buildNodeConnectionCommand valida los nodos y genera el comando ilink correspondiente.
func buildNodeConnectionCommand(request NodeConnectionRequest) (string, error) {
	if !isNumericNode(request.LocalNode) {
		return "", fmt.Errorf("local_node debe ser numérico")
	}
	if !isNumericNode(request.RemoteNode) {
		return "", fmt.Errorf("remote_node debe ser numérico")
	}
	modeCodes := map[string]int{
		"transceiver":   3,
		"permanent":     13,
		"monitor":       2,
		"local_monitor": 8,
	}
	code, found := modeCodes[request.Mode]
	if !found {
		return "", fmt.Errorf("mode debe ser transceiver, permanent, monitor o local_monitor")
	}
	return fmt.Sprintf("rpt cmd %s ilink %d %s", request.LocalNode, code, request.RemoteNode), nil
}

// buildSupermonCommand valida una petición y rellena la plantilla de comando de Supermon.
func buildSupermonCommand(request SupermonCommandRequest, definition supermonCommandDefinition) (string, error) {
	if !isNumericNode(request.LocalNode) {
		return "", fmt.Errorf("local_node debe ser numérico")
	}
	if definition.ID == "" || definition.ID != request.Action {
		return "", fmt.Errorf("acción desconocida")
	}
	if definition.RequiresRemote {
		if request.Action == "disconnect" {
			if !isValidRemoteNodeID(request.RemoteNode) {
				return "", fmt.Errorf("remote_node contiene caracteres no permitidos para esta acción")
			}
		} else if !isNumericNode(request.RemoteNode) {
			return "", fmt.Errorf("remote_node debe ser numérico para esta acción")
		}
	}
	if !definition.RequiresRemote && request.RemoteNode != "" {
		return "", fmt.Errorf("esta acción no acepta remote_node")
	}
	if request.Permanent && !definition.SupportsPermanent {
		return "", fmt.Errorf("esta acción no admite conexión permanente")
	}
	if strings.TrimSpace(definition.command) == "" {
		return "", fmt.Errorf("el comando de esta acción está vacío")
	}

	if definition.RequiresRemote {
		if request.Action == "disconnect" {
			command := strings.Replace(definition.command, "%s", request.LocalNode, 1)
			return strings.Replace(command, "%s", request.RemoteNode, 1), nil
		}
		code := supermonConnectionCode(request.Action, request.Permanent)
		command := strings.Replace(definition.command, "%s", request.LocalNode, 1)
		command = strings.Replace(command, "%d", fmt.Sprint(code), 1)
		return strings.Replace(command, "%s", request.RemoteNode, 1), nil
	}
	return strings.ReplaceAll(definition.command, "%s", request.LocalNode), nil
}
