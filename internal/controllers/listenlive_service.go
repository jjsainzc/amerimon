package controllers

import (
	"amerimon/internal/config"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// Validación estricta: los valores se interpolan en un comando de la CLI de Asterisk.
var (
	reExt  = regexp.MustCompile(`^[0-9A-Za-z_]{1,32}$`)
	reUUID = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`) // HandleEAGIConnection lee exactamente 36 bytes
)

type listenLiveRequest struct {
	Ext  string `json:"ext"`  // extensión PJSIP a espiar
	UUID string `json:"uuid"` // el mismo uuid que luego se usa en /listen-live/ws?uuid=
	// Opcional: sesión de la tabla listenlive a la que se asocia el uuid.
	ListenLiveID int64 `json:"listenliveid"`
}

// ListenLiveAsteriskStatus comprueba la conexión AMI sin originar llamadas.
// ListenLiveAsteriskStatus godoc
// @Summary Consultar el estado de Asterisk para escucha en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Produce json
// @Success 200
// @Failure 401
// @Router /listen-live/status [get]
func ListenLiveAsteriskStatus(c echo.Context) error {
	ami := config.App.AMI
	response, err := config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, "core show uptime", 5*time.Second)
	if err != nil || strings.Contains(response, "Response: Error") ||
		!(strings.Contains(response, "Response: Follows") || strings.Contains(response, "Response: Success")) {
		return c.JSON(http.StatusServiceUnavailable, echo.Map{
			"status":    "error",
			"connected": false,
			"message":   "No hay conexión con Asterisk",
		})
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "connected": true})
}

// StartListenLive origina el canal Local que espía la extensión (listen-live-spy)
// y envía el audio a Go por EAGI (listen-live-eagi, el uuid viaja como Exten).
// Debe llamarse antes de abrir el WebSocket. La llamada termina sola cuando el
// último WebSocket del uuid se cierra (ver HandleEAGIConnection).
// StartListenLive godoc
// @Summary Iniciar una sesión de escucha en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body listenLiveRequest true "Extensión y UUID de la sesión"
// @Success 200
// @Failure 400
// @Failure 401
// @Router /listen-live/start [post]
func StartListenLive(c echo.Context) error {
	var req listenLiveRequest
	if err := c.Bind(&req); err != nil || !reExt.MatchString(req.Ext) || !reUUID.MatchString(req.UUID) || req.ListenLiveID < 0 {
		return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "message": "ext o uuid inválidos"})
	}

	ami := config.App.AMI
	// /n evita la optimización del canal Local, que rompería el camino ChanSpy -> EAGI.
	channel := fmt.Sprintf("Local/%s@listen-live-spy/n", req.Ext)
	if err := config.ExecuteAMIOriginate(ami.Host, ami.Port, ami.User, ami.Secret, channel, "listen-live-eagi", req.UUID, 5*time.Second); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"status": "error", "message": err.Error()})
	}
	if config.App.UseDB && config.App.DB != nil {
		query := `UPDATE listenlive SET uuid = ? WHERE listenliveid = ?`
		args := []any{req.UUID, req.ListenLiveID}
		if req.ListenLiveID == 0 {
			// Sin id explícito: la sesión abierta del usuario (CreateListenLiveSession deja solo una).
			claims, err := GetClaims(c)
			if err != nil {
				return err
			}
			query = `UPDATE listenlive SET uuid = ? WHERE userid = ? AND ip_origen = ? AND fecha_fin IS NULL`
			args = []any{req.UUID, claims.UserID, c.RealIP()}
		}
		if _, err := config.App.DB.ExecContext(c.Request().Context(), query, args...); err != nil {
			return listenLiveDBError(c, err)
		}
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "data": nil, "message": "escucha iniciada"})
}

type stopListenLiveRequest struct {
	UUID         string `json:"uuid"`         // opcional si se envía listenliveid
	ListenLiveID int64  `json:"listenliveid"` // el uuid se toma de esta sesión
}

var reChannelName = regexp.MustCompile(`^[0-9A-Za-z_/@;.\-]{1,128}$`)

// StopListenLive (administrador) cierra los WebSockets del uuid, cuelga la llamada
// originada y marca fecha_fin en la sesión indicada.
// StopListenLive godoc
// @Summary Terminar una sesión de escucha en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body stopListenLiveRequest true "UUID de la sesión y, opcionalmente, id de la tabla listenlive"
// @Success 200
// @Failure 400
// @Failure 401
// @Router /listen-live/stop [post]
func StopListenLive(mgr *config.AudioStreamManager) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req stopListenLiveRequest
		if err := c.Bind(&req); err != nil || req.ListenLiveID < 0 || (req.UUID != "" && !reUUID.MatchString(req.UUID)) {
			return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "message": "uuid o listenliveid inválidos"})
		}

		if req.UUID == "" {
			if req.ListenLiveID == 0 || !config.App.UseDB || config.App.DB == nil {
				return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "message": "se requiere listenliveid o uuid"})
			}
			var uuid sql.NullString
			err := config.App.DB.QueryRowContext(c.Request().Context(),
				`SELECT uuid FROM listenlive WHERE listenliveid = ?`, req.ListenLiveID).Scan(&uuid)
			if err == sql.ErrNoRows {
				return listenLiveNotFound(c)
			}
			if err != nil {
				return listenLiveDBError(c, err)
			}
			if !uuid.Valid || !reUUID.MatchString(uuid.String) {
				return c.JSON(http.StatusConflict, echo.Map{"status": "error", "message": "la sesión no tiene uuid guardado"})
			}
			req.UUID = uuid.String
		}

		claims, err := GetClaims(c)
		if err != nil {
			return err
		}
		if claims.Role != "A" {
			// Solo puede terminar su propia sesión activa (mismo userid e IP de origen).
			var ownID int64
			err := config.App.DB.QueryRowContext(c.Request().Context(),
				`SELECT listenliveid FROM listenlive WHERE uuid = ? AND userid = ? AND ip_origen = ? AND fecha_fin IS NULL`,
				req.UUID, claims.UserID, c.RealIP()).Scan(&ownID)
			if err == sql.ErrNoRows {
				return c.JSON(http.StatusForbidden, echo.Map{"status": "error", "message": "la sesión no es suya"})
			}
			if err != nil {
				return listenLiveDBError(c, err)
			}
			req.ListenLiveID = ownID
		}

		log.Printf("[ListenLive] stop received source=stop uuid=%s listenliveid=%d", req.UUID, req.ListenLiveID)
		mgr.CloseClients(req.UUID)
		if mgr.CloseAudioConnection(req.UUID) {
			log.Printf("[ListenLive] closed EAGI TCP connection source=stop uuid=%s", req.UUID)
		} else {
			log.Printf("[ListenLive] no EAGI TCP connection registered source=stop uuid=%s", req.UUID)
		}

		hungUp, hangupErr := hangupListenLiveCalls("stop", req.UUID)
		if hangupErr != nil {
			log.Printf("[ListenLive] stop failed source=stop uuid=%s channels_hungup=%d error=%v", req.UUID, hungUp, hangupErr)
		}

		if config.App.UseDB && config.App.DB != nil {
			if _, err := config.App.DB.ExecContext(c.Request().Context(),
				`UPDATE listenlive SET fecha_fin = ? WHERE fecha_fin IS NULL AND (uuid = ? OR listenliveid = ?)`,
				time.Now().UTC(), req.UUID, req.ListenLiveID); err != nil {
				return listenLiveDBError(c, err)
			}
		}

		if hangupErr != nil {
			return c.JSON(http.StatusBadGateway, echo.Map{"status": "error", "message": "WebSocket cerrado, pero falló el hangup: " + hangupErr.Error()})
		}
		return c.JSON(http.StatusOK, echo.Map{"status": "success", "data": echo.Map{"channels_hungup": hungUp}, "message": "escucha terminada"})
	}
}

// hangupListenLiveCalls cuelga los canales cuyo contexto es listen-live-eagi y exten el uuid.
func hangupListenLiveCalls(source, uuid string) (int, error) {
	log.Printf("[ListenLive] hangup begin source=%s uuid=%s", source, uuid)
	ami := config.App.AMI
	out, err := config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, "core show channels concise", 5*time.Second)
	if err != nil {
		return 0, fmt.Errorf("consultar canales: %w", err)
	}
	channels := listenLiveChannels(out, uuid)
	if len(channels) == 0 {
		log.Printf("[ListenLive] hangup no channel match source=%s uuid=%s uuid_candidates=%q", source, uuid, listenLiveChannelDiagnostics(out, uuid))
		return 0, nil
	}
	log.Printf("[ListenLive] hangup channels found source=%s uuid=%s channels=%q", source, uuid, channels)
	for _, channel := range channels {
		command := "channel request hangup " + channel
		log.Printf("[ListenLive] hangup command sending source=%s uuid=%s command=%q", source, uuid, command)
		amiResponse, err := config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, command, 5*time.Second)
		if err != nil {
			return 0, fmt.Errorf("colgar canal %q: %w", channel, err)
		}
		if !strings.Contains(strings.ToLower(amiResponse), "requested hangup on channel") {
			return 0, fmt.Errorf("respuesta inesperada al colgar canal %q: %s", channel, strings.Join(config.ParseAMIOutputToLines(amiResponse), " "))
		}
		log.Printf("[ListenLive] hangup command response source=%s uuid=%s channel=%q response=%q", source, uuid, channel, strings.Join(config.ParseAMIOutputToLines(amiResponse), " "))
	}

	var remaining []string
	for attempt := 0; attempt < 4; attempt++ {
		time.Sleep(250 * time.Millisecond)
		out, err = config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, "core show channels concise", 5*time.Second)
		if err != nil {
			return len(channels), fmt.Errorf("verificar hangup: %w", err)
		}
		remaining = listenLiveRemainingChannels(out, channels)
		log.Printf("[ListenLive] hangup verification source=%s uuid=%s attempt=%d remaining=%q", source, uuid, attempt+1, remaining)
		if len(remaining) == 0 {
			log.Printf("[ListenLive] hangup confirmed source=%s uuid=%s channels=%q", source, uuid, channels)
			return len(channels), nil
		}
	}
	return len(channels), fmt.Errorf("hangup solicitado para %v pero siguen activos tras 1s: %v", channels, remaining)
}

// listenLiveChannels extrae los canales EAGI del UUID y sus pares Local presentes en la salida concisa de Asterisk.
func listenLiveChannels(output, uuid string) []string {
	liveChannels := make(map[string]struct{})
	lines := config.ParseAMIOutputToLines(output)
	for _, line := range lines {
		fields := strings.Split(line, "!")
		if len(fields) > 0 && reChannelName.MatchString(fields[0]) {
			liveChannels[fields[0]] = struct{}{}
		}
	}

	var channels []string
	seen := make(map[string]struct{})
	for _, line := range lines {
		// concise: canal!contexto!exten!prioridad!estado!app!datos!...
		fields := strings.Split(line, "!")
		if len(fields) >= 3 && fields[1] == "listen-live-eagi" && fields[2] == uuid && reChannelName.MatchString(fields[0]) {
			channel := fields[0]
			if _, exists := seen[channel]; !exists {
				channels = append(channels, channel)
				seen[channel] = struct{}{}
			}
			peer := localChannelPeer(channel)
			if _, exists := liveChannels[peer]; exists {
				if _, alreadyAdded := seen[peer]; !alreadyAdded {
					channels = append(channels, peer)
					seen[peer] = struct{}{}
				}
			}
		}
	}
	return channels
}

// localChannelPeer devuelve el otro miembro ;1/;2 de un canal Local o una cadena vacía si no corresponde.
func localChannelPeer(channel string) string {
	if strings.HasSuffix(channel, ";1") {
		return strings.TrimSuffix(channel, ";1") + ";2"
	}
	if strings.HasSuffix(channel, ";2") {
		return strings.TrimSuffix(channel, ";2") + ";1"
	}
	return ""
}

// listenLiveRemainingChannels filtra los canales esperados que siguen presentes en la salida de Asterisk.
func listenLiveRemainingChannels(output string, expected []string) []string {
	liveChannels := make(map[string]struct{})
	for _, line := range config.ParseAMIOutputToLines(output) {
		fields := strings.Split(line, "!")
		if len(fields) > 0 {
			liveChannels[fields[0]] = struct{}{}
		}
	}
	var remaining []string
	for _, channel := range expected {
		if _, exists := liveChannels[channel]; exists {
			remaining = append(remaining, channel)
		}
	}
	return remaining
}

// listenLiveChannelDiagnostics devuelve fragmentos de salida que coinciden con el UUID para facilitar el diagnóstico.
func listenLiveChannelDiagnostics(output, uuid string) []string {
	var candidates []string
	for _, line := range config.ParseAMIOutputToLines(output) {
		fields := strings.Split(line, "!")
		if len(fields) >= 3 && fields[2] == uuid {
			end := len(fields)
			if end > 6 {
				end = 6
			}
			candidates = append(candidates, strings.Join(fields[:end], "!"))
		}
	}
	return candidates
}

type ListenLiveSession struct {
	ID          uint32     `json:"listenliveid"`
	UserID      *int       `json:"userid"`
	Nombre      *string    `json:"nombre"`
	Callsign    *string    `json:"callsign"`
	IPOrigen    *string    `json:"ip_origen"`
	UUID        *string    `json:"uuid"`
	FechaInicio time.Time  `json:"fecha_inicio"`
	FechaFin    *time.Time `json:"fecha_fin"`
}

type updateListenLiveRequest struct {
	FechaFin *time.Time `json:"fecha_fin"`
}

// ListListenLiveSessions lista las sesiones abiertas para usuarios autenticados del monitor.
// ListListenLiveSessions godoc
// @Summary Listar sesiones de escucha en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Produce json
// @Param userid query int false "Filtrar por identificador de usuario"
// @Success 200
// @Failure 401
// @Router /listen-live/sessions [get]
func ListListenLiveSessions(c echo.Context) error {
	claims, err := GetClaims(c)
	if err != nil {
		return err
	}
	query := `SELECT l.listenliveid, l.userid, u.nombre, u.callsign, l.ip_origen, l.uuid, l.fecha_inicio, l.fecha_fin
		FROM listenlive l LEFT JOIN users u ON u.userid = l.userid
		WHERE l.fecha_fin IS NULL`
	args := []any{}
	if claims.Role == "A" {
		if userID := c.QueryParam("userid"); userID != "" {
			query += ` AND l.userid = ?`
			args = append(args, userID)
		}
	} else {
		query += ` AND l.userid = ? AND l.ip_origen = ?`
		args = append(args, claims.UserID, c.RealIP())
	}
	query += ` ORDER BY l.fecha_inicio DESC`

	rows, err := config.App.DB.QueryContext(c.Request().Context(), query, args...)
	if err != nil {
		return listenLiveDBError(c, err)
	}
	defer rows.Close()

	sessions := make([]ListenLiveSession, 0)
	for rows.Next() {
		item, err := scanListenLive(rows)
		if err != nil {
			return listenLiveDBError(c, err)
		}
		sessions = append(sessions, item)
	}
	if err := rows.Err(); err != nil {
		return listenLiveDBError(c, err)
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "data": sessions})
}

// GetListenLiveSession devuelve una sesión propia o cualquiera para administradores.
// GetListenLiveSession godoc
// @Summary Consultar una sesión de escucha en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Produce json
// @Param id path int true "Identificador de la sesión"
// @Success 200
// @Failure 401
// @Router /listen-live/sessions/{id} [get]
func GetListenLiveSession(c echo.Context) error {
	claims, err := GetClaims(c)
	if err != nil {
		return err
	}
	id, err := parseListenLiveID(c)
	if err != nil {
		return err
	}
	query := `SELECT l.listenliveid, l.userid, u.nombre, u.callsign, l.ip_origen, l.uuid, l.fecha_inicio, l.fecha_fin
		FROM listenlive l LEFT JOIN users u ON u.userid = l.userid WHERE l.listenliveid = ?`
	args := []any{id}
	if claims.Role != "A" {
		query += ` AND l.userid = ? AND l.ip_origen = ?`
		args = append(args, claims.UserID, c.RealIP())
	}
	item, err := scanListenLive(config.App.DB.QueryRowContext(c.Request().Context(), query, args...))
	if err == sql.ErrNoRows {
		return listenLiveNotFound(c)
	}
	if err != nil {
		return listenLiveDBError(c, err)
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "data": item})
}

// CreateListenLiveSession crea una sesión asociada al usuario autenticado.
// CreateListenLiveSession godoc
// @Summary Crear una sesión de escucha en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Accept json
// @Produce json
// @Success 200
// @Failure 400
// @Failure 401
// @Router /listen-live/sessions [post]
func CreateListenLiveSession(c echo.Context) error {
	claims, err := GetClaims(c)
	if err != nil {
		return err
	}
	tx, err := config.App.DB.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return listenLiveDBError(c, err)
	}
	defer tx.Rollback()

	// La conexión única de SQLite ya serializa la creación de sesiones.
	fechaFin := time.Now().UTC()
	if _, err := tx.ExecContext(c.Request().Context(),
		`UPDATE listenlive SET fecha_fin = ? WHERE userid = ? AND ip_origen = ? AND fecha_fin IS NULL`, fechaFin, claims.UserID, c.RealIP()); err != nil {
		return listenLiveDBError(c, err)
	}
	result, err := tx.ExecContext(c.Request().Context(),
		`INSERT INTO listenlive (userid, ip_origen) VALUES (?, ?)`, claims.UserID, c.RealIP())
	if err != nil {
		return listenLiveDBError(c, err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return listenLiveDBError(c, err)
	}
	item, err := scanListenLive(tx.QueryRowContext(c.Request().Context(),
		`SELECT l.listenliveid, l.userid, u.nombre, u.callsign, l.ip_origen, l.uuid, l.fecha_inicio, l.fecha_fin
		 FROM listenlive l LEFT JOIN users u ON u.userid = l.userid WHERE l.listenliveid = ?`, id))
	if err != nil {
		return listenLiveDBError(c, err)
	}
	if err := tx.Commit(); err != nil {
		return listenLiveDBError(c, err)
	}
	return c.JSON(http.StatusCreated, echo.Map{"status": "success", "data": item})
}

// UpdateListenLiveSession permite cerrar una sesión; no acepta cambiar usuario ni IP.
// UpdateListenLiveSession godoc
// @Summary Actualizar una sesión de escucha en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Identificador de la sesión"
// @Param body body updateListenLiveRequest true "Campos de la sesión a actualizar"
// @Success 200
// @Failure 400
// @Failure 401
// @Router /listen-live/sessions/{id} [patch]
func UpdateListenLiveSession(mgr *config.AudioStreamManager) echo.HandlerFunc {
	return func(c echo.Context) error {
		return updateListenLiveSession(c, mgr)
	}
}

// updateListenLiveSession cierra una sesión autorizada y, si tiene UUID, también el audio y la llamada de Asterisk.
func updateListenLiveSession(c echo.Context, mgr *config.AudioStreamManager) error {
	claims, err := GetClaims(c)
	if err != nil {
		return err
	}
	id, err := parseListenLiveID(c)
	if err != nil {
		return err
	}
	var input updateListenLiveRequest
	if err := c.Bind(&input); err != nil || input.FechaFin == nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "message": "fecha_fin es obligatoria y debe ser RFC3339"})
	}
	query := `UPDATE listenlive SET fecha_fin = ? WHERE listenliveid = ?`
	args := []any{input.FechaFin.UTC(), id}
	if claims.Role != "A" {
		query += ` AND userid = ? AND ip_origen = ?`
		args = append(args, claims.UserID, c.RealIP())
	}
	if _, err := config.App.DB.ExecContext(c.Request().Context(), query, args...); err != nil {
		return listenLiveDBError(c, err)
	}

	// Cerrar la sesión termina también el WebSocket y la llamada originada.
	var uuid sql.NullString
	uuidQuery := `SELECT uuid FROM listenlive WHERE listenliveid = ?`
	uuidArgs := []any{id}
	if claims.Role != "A" {
		uuidQuery += ` AND userid = ? AND ip_origen = ?`
		uuidArgs = append(uuidArgs, claims.UserID, c.RealIP())
	}
	if err := config.App.DB.QueryRowContext(c.Request().Context(), uuidQuery, uuidArgs...).Scan(&uuid); err != nil && err != sql.ErrNoRows {
		return listenLiveDBError(c, err)
	}
	if uuid.Valid && reUUID.MatchString(uuid.String) {
		log.Printf("[ListenLive] stop received source=session-patch uuid=%s listenliveid=%d", uuid.String, id)
		mgr.CloseClients(uuid.String)
		if mgr.CloseAudioConnection(uuid.String) {
			log.Printf("[ListenLive] closed EAGI TCP connection source=session-patch uuid=%s", uuid.String)
		} else {
			log.Printf("[ListenLive] no EAGI TCP connection registered source=session-patch uuid=%s", uuid.String)
		}
		if count, err := hangupListenLiveCalls("session-patch", uuid.String); err != nil {
			log.Printf("[ListenLive] stop failed source=session-patch uuid=%s channels_hungup=%d error=%v", uuid.String, count, err)
			return c.JSON(http.StatusBadGateway, echo.Map{"status": "error", "message": "sesión cerrada, pero falló el hangup: " + err.Error()})
		}
	}
	return GetListenLiveSession(c)
}

// DeleteListenLiveSession elimina una sesión propia o cualquiera para administradores.
// DeleteListenLiveSession godoc
// @Summary Eliminar una sesión de escucha en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Produce json
// @Param id path int true "Identificador de la sesión"
// @Success 200
// @Failure 401
// @Router /listen-live/sessions/{id} [delete]
func DeleteListenLiveSession(c echo.Context) error {
	claims, err := GetClaims(c)
	if err != nil {
		return err
	}
	id, err := parseListenLiveID(c)
	if err != nil {
		return err
	}
	query := `DELETE FROM listenlive WHERE listenliveid = ?`
	args := []any{id}
	if claims.Role != "A" {
		query += ` AND userid = ? AND ip_origen = ?`
		args = append(args, claims.UserID, c.RealIP())
	}
	result, err := config.App.DB.ExecContext(c.Request().Context(), query, args...)
	if err != nil {
		return listenLiveDBError(c, err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return listenLiveDBError(c, err)
	}
	if deleted == 0 {
		return listenLiveNotFound(c)
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "message": "Sesión eliminada"})
}

type listenLiveScanner interface {
	Scan(dest ...any) error
}

// scanListenLive convierte una fila SQL en sesión, asigna como punteros los valores nullable y devuelve el error de Scan.
func scanListenLive(scanner listenLiveScanner) (ListenLiveSession, error) {
	var item ListenLiveSession
	var userID sql.NullInt64
	var nombre sql.NullString
	var callsign sql.NullString
	var ipOrigen sql.NullString
	var uuid sql.NullString
	var fechaFin sql.NullTime
	err := scanner.Scan(&item.ID, &userID, &nombre, &callsign, &ipOrigen, &uuid, &item.FechaInicio, &fechaFin)
	if userID.Valid {
		value := int(userID.Int64)
		item.UserID = &value
	}
	if nombre.Valid {
		item.Nombre = &nombre.String
	}
	if callsign.Valid {
		item.Callsign = &callsign.String
	}
	if ipOrigen.Valid {
		item.IPOrigen = &ipOrigen.String
	}
	if uuid.Valid {
		item.UUID = &uuid.String
	}
	if fechaFin.Valid {
		item.FechaFin = &fechaFin.Time
	}
	return item, err
}

// parseListenLiveID valida y devuelve el identificador positivo de la ruta o un error HTTP 400.
func parseListenLiveID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, echo.Map{"status": "error", "message": "id inválido"})
	}
	return id, nil
}

// listenLiveNotFound responde con HTTP 404 para una sesión inexistente o no accesible.
func listenLiveNotFound(c echo.Context) error {
	return c.JSON(http.StatusNotFound, echo.Map{"status": "error", "message": "Sesión no encontrada"})
}

// listenLiveDBError responde con HTTP 500 e incluye el error de base de datos recibido.
func listenLiveDBError(c echo.Context, err error) error {
	return c.JSON(http.StatusInternalServerError, echo.Map{"status": "error", "message": "Error de base de datos: " + err.Error()})
}
