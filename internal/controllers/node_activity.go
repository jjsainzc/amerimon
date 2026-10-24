package controllers

import (
	"amerimon/internal/config"
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

const (
	defaultActivityPollInterval = 5 * time.Second
	minimumActivityPollInterval = 5 * time.Second
)

type AllStarLinkNodeActivity struct {
	LocalNode       string    `json:"local_node"`
	RemoteNode      string    `json:"remote_node"`
	UltimaActividad time.Time `json:"ultima_actividad"`
}

type AsteriskConnectionHistory struct {
	ID                uint64     `json:"id"`
	InstanceID        string     `json:"instance_id"`
	RemoteIP          *string    `json:"remote_ip,omitempty"`
	LocalNode         string     `json:"local_node"`
	RemoteNode        string     `json:"remote_node"`
	ConnectionType    string     `json:"connection_type"`
	RemoteCallsign    *string    `json:"remote_callsign"`
	RemoteDescription *string    `json:"remote_description"`
	Direction         *string    `json:"direction,omitempty"`
	Mode              *string    `json:"mode,omitempty"`
	LinkState         *string    `json:"link_state,omitempty"`
	FirstSeenAt       time.Time  `json:"first_seen_at"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	DisconnectedAt    *time.Time `json:"disconnected_at,omitempty"`
	DisconnectReason  *string    `json:"disconnect_reason,omitempty"`
}

// GetAsteriskConnectionHistory devuelve el historial de conexiones para generar reportes.
// GetAsteriskConnectionHistory godoc
// @Summary Consultar historial de conexiones Asterisk
// @Tags AllStarLink
// @Security BearerAuth
// @Produce json
// @Success 200
// @Failure 401
// @Router /allstarlink/connections/history [get]
func GetAsteriskConnectionHistory(c echo.Context) error {
	if config.App.DB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status":  "error",
			"message": "La base de datos no está disponible",
		})
	}

	query := `SELECT id, instance_id, remote_ip, local_node, remote_node, connection_type, remote_callsign, remote_description, direction, mode, link_state,
		first_seen_at, last_seen_at, disconnected_at, disconnect_reason
		FROM asterisk_connection_history`
	conditions := make([]string, 0, 6)
	args := make([]any, 0, 7)
	for _, filter := range []struct {
		name   string
		column string
		valid  func(string) bool
	}{
		{name: "local_node", column: "local_node", valid: isNumericNode},
		{name: "remote_node", column: "remote_node", valid: isValidRemoteNodeID},
	} {
		value := strings.TrimSpace(c.QueryParam(filter.name))
		if value == "" {
			continue
		}
		if !filter.valid(value) {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"status":  "error",
				"message": fmt.Sprintf("El parámetro %s no es válido", filter.name),
			})
		}
		conditions = append(conditions, filter.column+" = ?")
		args = append(args, value)
	}

	connectionType := strings.ToLower(strings.TrimSpace(c.QueryParam("connection_type")))
	if connectionType != "" {
		if connectionType != "iax" && connectionType != "iaxrpt" && connectionType != "echolink" {
			return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "connection_type debe ser iax, iaxrpt o echolink"})
		}
		conditions = append(conditions, "connection_type = ?")
		args = append(args, connectionType)
	}

	switch status := strings.ToLower(strings.TrimSpace(c.QueryParam("status"))); status {
	case "", "all":
	case "active":
		conditions = append(conditions, "disconnected_at IS NULL")
	case "disconnected":
		conditions = append(conditions, "disconnected_at IS NOT NULL")
	default:
		return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "status debe ser active, disconnected o all"})
	}

	var fromTime, toTime *time.Time
	toExclusive := false
	for _, filter := range []struct {
		name string
	}{
		{name: "from"},
		{name: "to"},
	} {
		value := strings.TrimSpace(c.QueryParam(filter.name))
		if value == "" {
			continue
		}
		parsed, operator, err := parseConnectionHistoryBound(value, filter.name == "to")
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": fmt.Sprintf("%s debe tener formato YYYY-MM-DD o RFC3339", filter.name)})
		}
		conditions = append(conditions, "first_seen_at "+operator+" ?")
		args = append(args, parsed.UTC())
		if filter.name == "from" {
			fromTime = &parsed
		} else {
			toTime = &parsed
			toExclusive = operator == "<"
		}
	}
	if fromTime != nil && toTime != nil && (fromTime.After(*toTime) || (toExclusive && !fromTime.Before(*toTime))) {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "from debe ser anterior o igual a to"})
	}

	limit := 500
	if value := strings.TrimSpace(c.QueryParam("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 5000 {
			return c.JSON(http.StatusBadRequest, map[string]any{"status": "error", "message": "limit debe estar entre 1 y 5000"})
		}
		limit = parsed
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ") + " AND disconnected_at IS NOT NULL "
	} else {
		query += " WHERE disconnected_at IS NOT NULL "
	}

	// query += " ORDER BY first_seen_at DESC LIMIT ?"
	query += " ORDER BY disconnected_at DESC LIMIT ?"

	args = append(args, limit)
	rows, err := config.App.DB.QueryContext(c.Request().Context(), query, args...)
	if err != nil {
		log.Printf("allstarlink history: no se pudo consultar el historial: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo consultar el historial de conexiones"})
	}
	defer rows.Close()

	history := make([]AsteriskConnectionHistory, 0)
	for rows.Next() {
		var item AsteriskConnectionHistory
		var remoteIP, callsign, description, direction, mode, linkState, disconnectReason sql.NullString
		var disconnectedAt sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.InstanceID, &remoteIP, &item.LocalNode, &item.RemoteNode, &item.ConnectionType,
			&callsign, &description, &direction, &mode, &linkState, &item.FirstSeenAt, &item.LastSeenAt, &disconnectedAt, &disconnectReason,
		); err != nil {
			log.Printf("allstarlink history: no se pudo interpretar un registro: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo consultar el historial de conexiones"})
		}
		item.RemoteIP = nullStringPointer(remoteIP)
		item.RemoteCallsign = connectionHistoryCallsign(item.RemoteNode, callsign)
		item.RemoteDescription = nullStringPointer(description)
		item.Direction = nullStringPointer(direction)
		item.Mode = nullStringPointer(mode)
		item.LinkState = nullStringPointer(linkState)
		item.DisconnectReason = nullStringPointer(disconnectReason)
		if disconnectedAt.Valid {
			item.DisconnectedAt = &disconnectedAt.Time
		}
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("allstarlink history: error al recorrer registros: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": "error", "message": "No se pudo consultar el historial de conexiones"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": "success", "data": history})
}

// nullStringPointer convierte un sql.NullString válido en puntero y devuelve nil si contiene NULL.
func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

// connectionHistoryCallsign devuelve el indicativo almacenado o, para identificadores no numéricos, el propio identificador.
func connectionHistoryCallsign(remoteNode string, callsign sql.NullString) *string {
	if callsign.Valid && callsign.String != "" {
		return &callsign.String
	}
	if !isNumericNode(remoteNode) {
		return &remoteNode
	}
	return nil
}

// parseConnectionHistoryBound analiza una fecha o RFC3339 y devuelve el operador SQL adecuado o un error si el formato no es válido.
func parseConnectionHistoryBound(value string, upperBound bool) (time.Time, string, error) {
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		parsed = parsed.UTC()
		if upperBound {
			return parsed.AddDate(0, 0, 1), "<", nil
		}
		return parsed, ">=", nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, "", err
	}
	if upperBound {
		return parsed.UTC(), "<=", nil
	}
	return parsed.UTC(), ">=", nil
}

// GetAllStarLinkNodeActivity Devuelve las marcas de tiempo del último audio guardadas.
// GetAllStarLinkNodeActivity godoc
// @Summary Consultar actividad de nodos AllStarLink
// @Tags AllStarLink
// @Security BearerAuth
// @Produce json
// @Success 200
// @Failure 401
// @Router /allstarlink/activity [get]
func GetAllStarLinkNodeActivity(c echo.Context) error {
	if config.App.DB == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status":  "error",
			"message": "La base de datos no está disponible",
		})
	}

	query := `SELECT local_node, remote_node, ultima_actividad FROM allstarlink_node_activity`
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 2)
	for _, filter := range []struct {
		name   string
		column string
	}{{"local_node", "local_node"}, {"remote_node", "remote_node"}} {
		value := strings.TrimSpace(c.QueryParam(filter.name))
		if value == "" {
			continue
		}
		valid := isValidRemoteNodeID(value)
		if filter.name == "local_node" {
			valid = isNumericNode(value)
		}
		if !valid {
			return c.JSON(http.StatusBadRequest, map[string]any{
				"status":  "error",
				"message": fmt.Sprintf("El parámetro %s debe ser numérico", filter.name),
			})
		}
		conditions = append(conditions, filter.column+" = ?")
		args = append(args, value)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY ultima_actividad DESC, local_node, remote_node"

	rows, err := config.App.DB.QueryContext(c.Request().Context(), query, args...)
	if err != nil {
		log.Printf("allstarlink activity: no se pudieron consultar actividades: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "No se pudieron consultar las actividades de AllStarLink",
		})
	}
	defer rows.Close()

	activities := make([]AllStarLinkNodeActivity, 0)
	for rows.Next() {
		var activity AllStarLinkNodeActivity
		if err := rows.Scan(&activity.LocalNode, &activity.RemoteNode, &activity.UltimaActividad); err != nil {
			log.Printf("allstarlink activity: no se pudo interpretar una actividad: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"status":  "error",
				"message": "No se pudieron consultar las actividades de AllStarLink",
			})
		}
		activities = append(activities, activity)
	}
	if err := rows.Err(); err != nil {
		log.Printf("allstarlink activity: error al recorrer actividades: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "No se pudieron consultar las actividades de AllStarLink",
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data":   activities,
	})
}

// StartAllStarLinkActivityScheduler almacena periódicamente la última hora de audio
// reportada por Asterisk SawStat para cada nodo remoto conectado.
func StartAllStarLinkActivityScheduler() {
	if config.App.DB == nil {
		log.Printf("allstarlink activity: sondeo deshabilitado porque no hay base de datos")
		return
	}

	interval := defaultActivityPollInterval
	if configured := strings.TrimSpace(config.CurrentAMI().ActivityPollInterval); configured != "" {
		parsed, err := time.ParseDuration(configured)
		if err != nil || parsed < minimumActivityPollInterval {
			log.Printf("allstarlink activity: intervalo %q inválido; se usa %s", configured, defaultActivityPollInterval)
		} else {
			interval = parsed
		}
	}

	go func() {
		echoLinkCache := make(map[string]AstDBEntry)
		pollAllStarLinkNodeActivity(echoLinkCache)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			pollAllStarLinkNodeActivity(echoLinkCache)
		}
	}()
}

// pollAllStarLinkNodeActivity consulta SawStat y persiste la última actividad de cada nodo remoto.
func pollAllStarLinkNodeActivity(echoLinkCache map[string]AstDBEntry) {
	instances, err := allStarLinkActivityInstances()
	if err != nil {
		log.Printf("allstarlink activity: no se pudieron cargar nodos AMI: %v", err)
		return
	}
	if len(instances) == 0 {
		return
	}
	astdbEntries, privateEntries := loadAsteriskHistoryNodeMetadata()

	for _, instance := range instances {
		xstat, err := config.ExecuteAMIRptStatus(instance.Host, instance.Port, instance.User, instance.Secret, instance.Node, "XStat", 5*time.Second)
		if err != nil {
			log.Printf("allstarlink history: XStat para nodo local %s falló: %v", instance.Node, err)
		} else {
			connections := parseRPTConnections(xstat, "")
			for _, connection := range connections {
				lookupNumber, isEchoLink := echoLinkLookupNumber(connection.Node)
				if !isEchoLink {
					continue
				}
				cacheKey := net.JoinHostPort(instance.Host, instance.Port) + ":" + connection.Node
				entry, found := echoLinkCache[cacheKey]
				if !found {
					command := "echolink dbget nodename " + lookupNumber
					raw, lookupErr := config.ExecuteAMICommand(instance.Host, instance.Port, instance.User, instance.Secret, command, 5*time.Second)
					if lookupErr != nil {
						log.Printf("allstarlink history: lookup EchoLink para nodo %s falló: %v", connection.Node, lookupErr)
					} else if entry, found = parseEchoLinkDBResponse(connection.Node, lookupNumber, raw); found {
						echoLinkCache[cacheKey] = entry
					}
				}
				if found {
					astdbEntries[connection.Node] = entry
				}
			}
			if err := recordAsteriskConnectionHistory(instance, connections, astdbEntries, privateEntries, time.Now().UTC()); err != nil {
				log.Printf("allstarlink history: no se pudo guardar el historial del nodo local %s: %v", instance.Node, err)
			}
		}

		sawstat, err := config.ExecuteAMIRptStatus(instance.Host, instance.Port, instance.User, instance.Secret, instance.Node, "SawStat", 5*time.Second)
		if err != nil {
			log.Printf("allstarlink activity: SawStat para nodo local %s falló: %v", instance.Node, err)
			continue
		}

		activities := parseRPTLastActivity(sawstat, time.Now())
		for remoteNode, activity := range activities {
			_, err := config.App.DB.Exec(
				`INSERT INTO allstarlink_node_activity (local_node, remote_node, ultima_actividad)
				 VALUES (?, ?, ?)
				 ON CONFLICT (local_node, remote_node) DO UPDATE SET ultima_actividad = MAX(ultima_actividad, excluded.ultima_actividad)`,
				instance.Node, remoteNode, activity.UTC(),
			)
			if err != nil {
				log.Printf("allstarlink activity: no se pudo guardar actividad del nodo %s en %s: %v", remoteNode, instance.Node, err)
			}
		}
	}
}

// recordAsteriskConnectionHistory actualiza, crea o cierra sesiones de conexión en una transacción y devuelve errores SQL.
func recordAsteriskConnectionHistory(instance allmonInstance, connections []rptConnection, astdbEntries, privateEntries map[string]AstDBEntry, observedAt time.Time) error {
	if config.App.DB == nil {
		return nil
	}
	// SQLite guarda las fechas como texto; en UTC se comparan correctamente.
	observedAt = observedAt.UTC()

	ctx := context.Background()
	tx, err := config.App.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	instanceID := net.JoinHostPort(instance.Host, instance.Port)
	rows, err := tx.QueryContext(ctx,
		`SELECT id, remote_node FROM asterisk_connection_history
		 WHERE instance_id = ? AND local_node = ? AND disconnected_at IS NULL`,
		instanceID, instance.Node,
	)
	if err != nil {
		return err
	}
	openSessions := make(map[string][]uint64)
	for rows.Next() {
		var id uint64
		var remoteNode string
		if err := rows.Scan(&id, &remoteNode); err != nil {
			rows.Close()
			return err
		}
		openSessions[remoteNode] = append(openSessions[remoteNode], id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	activeConnections := make(map[string]rptConnection, len(connections))
	for _, connection := range connections {
		activeConnections[connection.Node] = connection
	}
	for remoteNode, connection := range activeConnections {
		connectionType := asteriskConnectionType(remoteNode)
		callsign, description := asteriskHistoryNodeInfo(remoteNode, astdbEntries, privateEntries)
		if ids := openSessions[remoteNode]; len(ids) > 0 {
			for _, id := range ids {
				if _, err := tx.ExecContext(ctx,
					`UPDATE asterisk_connection_history
					 SET remote_ip = COALESCE(?, remote_ip), connection_type = ?, remote_callsign = COALESCE(?, remote_callsign),
					     remote_description = COALESCE(?, remote_description), direction = ?, mode = ?, link_state = ?, last_seen_at = ?
					 WHERE id = ? AND disconnected_at IS NULL`,
					nullableString(connection.RemoteIP), connectionType, nullableString(callsign), nullableString(description), nullableString(connection.Direction), nullableString(connection.Mode), nullableString(connection.Link), observedAt, id,
				); err != nil {
					return err
				}
			}
			delete(openSessions, remoteNode)
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO asterisk_connection_history
			 (instance_id, remote_ip, local_node, remote_node, connection_type, remote_callsign, remote_description, direction, mode, link_state, first_seen_at, last_seen_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			instanceID, nullableString(connection.RemoteIP), instance.Node, remoteNode, connectionType, nullableString(callsign), nullableString(description),
			nullableString(connection.Direction), nullableString(connection.Mode), nullableString(connection.Link), observedAt, observedAt,
		); err != nil {
			return err
		}
	}

	for _, ids := range openSessions {
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx,
				`UPDATE asterisk_connection_history SET disconnected_at = ? WHERE id = ? AND disconnected_at IS NULL`,
				observedAt, id,
			); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// loadAsteriskHistoryNodeMetadata carga AstDB y la lista privada, registrando errores y devolviendo mapas vacíos cuando fallan.
func loadAsteriskHistoryNodeMetadata() (map[string]AstDBEntry, map[string]AstDBEntry) {
	astdbEntries := make(map[string]AstDBEntry)
	if path := strings.TrimSpace(config.App.Sources.AstDBPath); path != "" {
		entries, err := readAstDBEntries(path)
		if err != nil {
			log.Printf("allstarlink history: no se pudo leer AstDB: %v", err)
		} else {
			astdbEntries = entries
		}
	}

	privatePath := privateNodesFilePath()
	privateEntries := make(map[string]AstDBEntry)
	if _, err := os.Stat(privatePath); err == nil {
		entries, readErr := readAstDBEntries(privatePath)
		if readErr != nil {
			log.Printf("allstarlink history: no se pudo leer la lista de nodos privados: %v", readErr)
		} else {
			privateEntries = entries
		}
	} else if !os.IsNotExist(err) {
		log.Printf("allstarlink history: no se pudo acceder a la lista de nodos privados: %v", err)
	}
	return astdbEntries, privateEntries
}

// asteriskHistoryNodeInfo devuelve el indicativo y la descripción del nodo, con un texto alternativo si faltan datos.
func asteriskHistoryNodeInfo(remoteNode string, astdbEntries, privateEntries map[string]AstDBEntry) (string, string) {
	entry := lookupNodeInfo(remoteNode, astdbEntries, privateEntries)
	callsign := entry.Callsign
	if callsign == "" && !isNumericNode(remoteNode) {
		callsign = remoteNode
	}
	information := allStarLinkNodeInformation(remoteNode, entry)
	if strings.TrimSpace(information) == "" {
		information = fallbackAsteriskNodeDescription(remoteNode)
	}
	return callsign, information
}

// fallbackAsteriskNodeDescription genera una descripción según el tipo de conexión y el identificador remoto.
func fallbackAsteriskNodeDescription(remoteNode string) string {
	switch asteriskConnectionType(remoteNode) {
	case "echolink":
		return "EchoLink " + remoteNode
	case "iaxrpt":
		return "IAXRpt " + remoteNode
	default:
		return "Nodo IAX " + remoteNode
	}
}

// asteriskConnectionType clasifica el identificador remoto como iax, iaxrpt o echolink.
func asteriskConnectionType(remoteNode string) string {
	if _, isEchoLink := echoLinkLookupNumber(remoteNode); isEchoLink {
		return "echolink"
	}
	if !isNumericNode(remoteNode) {
		return "iaxrpt"
	}
	return "iax"
}

// nullableString devuelve nil para texto vacío y el valor original para guardar NULL en SQL cuando corresponde.
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// allStarLinkActivityInstances obtiene los nodos locales y sus credenciales AMI.
func allStarLinkActivityInstances() ([]allmonInstance, error) {
	instances, configured, err := loadAllmonInstances()
	if err != nil || configured {
		return instances, err
	}

	ami := config.CurrentAMI()
	if ami.Host == "" || ami.Port == "" || ami.User == "" || ami.Secret == "" {
		return nil, fmt.Errorf("la configuración AMI de Asterisk está incompleta")
	}
	return reconcileLocalAllmonInstances(nil)
}

// parseRPTLastActivity convierte los tiempos de SawStat en marcas horarias por nodo remoto.
func parseRPTLastActivity(sawstat string, observedAt time.Time) map[string]time.Time {
	activities := make(map[string]time.Time)
	maxSeconds := int64(math.MaxInt64 / int64(time.Second))
	for _, line := range config.ParseAMIOutputToLines(sawstat) {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "Conn:" || !isValidRemoteNodeID(fields[1]) {
			continue
		}
		seconds, err := parseNonNegativeSeconds(fields[3], maxSeconds)
		if err == nil {
			activities[fields[1]] = observedAt.Add(-time.Duration(seconds) * time.Second)
		}
	}
	return activities
}

// parseNonNegativeSeconds analiza segundos y rechaza valores negativos o fuera del límite.
func parseNonNegativeSeconds(value string, maximum int64) (int64, error) {
	var seconds int64
	if _, err := fmt.Sscan(value, &seconds); err != nil || seconds < 0 || seconds > maximum {
		return 0, fmt.Errorf("invalid seconds value %q", value)
	}
	return seconds, nil
}

// loadLastNodeActivities carga de la base de datos la actividad guardada para las instancias indicadas.
func loadLastNodeActivities(instances []allmonInstance) map[string]time.Time {
	activities := make(map[string]time.Time)
	if config.App.DB == nil || len(instances) == 0 {
		return activities
	}

	localNodes := make([]string, 0, len(instances))
	seen := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		if _, exists := seen[instance.Node]; exists {
			continue
		}
		seen[instance.Node] = struct{}{}
		localNodes = append(localNodes, instance.Node)
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(localNodes)), ",")
	args := make([]any, len(localNodes))
	for i, node := range localNodes {
		args[i] = node
	}
	rows, err := config.App.DB.Query(
		`SELECT local_node, remote_node, ultima_actividad
		 FROM allstarlink_node_activity WHERE local_node IN (`+placeholders+")",
		args...,
	)
	if err != nil {
		log.Printf("allstarlink activity: no se pudieron leer actividades: %v", err)
		return activities
	}
	defer rows.Close()

	for rows.Next() {
		var localNode, remoteNode string
		var activity time.Time
		if err := rows.Scan(&localNode, &remoteNode, &activity); err != nil {
			log.Printf("allstarlink activity: no se pudo interpretar una actividad: %v", err)
			continue
		}
		activities[localNode+":"+remoteNode] = activity
	}
	if err := rows.Err(); err != nil {
		log.Printf("allstarlink activity: error al recorrer actividades: %v", err)
	}
	return activities
}
