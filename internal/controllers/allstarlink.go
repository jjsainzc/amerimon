package controllers

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

type AllStarLinkNode struct {
	Protocolo       string              `json:"protocolo"`
	Categoria       string              `json:"categoria,omitempty"`
	NodoLocal       string              `json:"nodo_local"`
	Nodo            string              `json:"nodo"`
	Contacto        string              `json:"contacto,omitempty"`
	Informacion     string              `json:"informacion"`
	Callsign        string              `json:"callsign"`
	Descripcion     string              `json:"descripcion"`
	Ubicacion       string              `json:"ubicacion"`
	Recibido        string              `json:"recibido"`
	Enlace          string              `json:"enlace"`
	Direccion       string              `json:"direccion"`
	Conectado       string              `json:"conectado"`
	Modo            string              `json:"modo"`
	ModoCodigo      string              `json:"modo_codigo"`
	UltimaActividad *time.Time          `json:"ultima_actividad"`
	Latencia        string              `json:"latencia,omitempty"`
	Acciones        []AllStarLinkAction `json:"acciones,omitempty"`
}

type AllStarLinkAction struct {
	ID           string                  `json:"id"`
	Label        string                  `json:"label"`
	Method       string                  `json:"method"`
	Endpoint     string                  `json:"endpoint"`
	Payload      *SupermonCommandRequest `json:"payload,omitempty"`
	Presentation string                  `json:"presentation,omitempty"`
	IframeURL    string                  `json:"iframe_url,omitempty"`
}

type AllStarLinkNodeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type AllStarLinkLocalTable struct {
	NodoLocal         string                  `json:"nodo_local"`
	ServidorConectado bool                    `json:"servidor_conectado"`
	NodosDisponibles  []AllStarLinkNodeOption `json:"nodos_disponibles"`
	Acciones          []AllStarLinkAction     `json:"acciones,omitempty"`
}

// GetAllStarLinkNodes Devuelve los datos de los nodos AllStar conectados como filas de una tabla JSON.
// GetAllStarLinkNodes godoc
// @Summary Listar nodos AllStarLink
// @Tags AllStarLink
// @Security BearerAuth
// @Produce json
// @Success 200
// @Failure 401
// @Router /allstarlink/nodes [get]
func GetAllStarLinkNodes(c echo.Context) error {
	localNode := strings.TrimSpace(c.QueryParam("local_node"))
	if localNode != "" && !isNumericNode(localNode) {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":  "error",
			"message": "El parámetro local_node debe ser numérico",
		})
	}
	rptConfigPath := config.App.AMI.RPTConfigPath
	if rptConfigPath == "" {
		rptConfigPath = "/etc/asterisk/rpt.conf"
	}
	primaryNode, err := readPrimaryRPTNode(rptConfigPath)
	if err != nil {
		log.Printf("allstarlink: no se pudo leer el nodo principal de rpt.conf: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "No se pudo obtener el nodo principal de Asterisk",
		})
	}
	amiInstances, allmonConfigured, err := loadAllmonInstances()
	if err != nil {
		log.Printf("allstarlink: no se pudo leer allmon.ini: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"message": "No se pudo leer la configuración de Allmon",
		})
	}
	if allmonConfigured {
		if len(amiInstances) == 0 {
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"status":  "error",
				"message": "allmon.ini no contiene instancias Nodes válidas",
			})
		}
		if localNode != "" {
			filtered := make([]allmonInstance, 0, 1)
			for _, instance := range amiInstances {
				if instance.Node == localNode {
					filtered = append(filtered, instance)
				}
			}
			if len(filtered) == 0 {
				return c.JSON(http.StatusNotFound, map[string]any{
					"status":  "error",
					"message": "El nodo local no existe en allmon.ini",
				})
			}
			amiInstances = filtered
		}
	} else {
		ami := config.App.AMI
		if ami.Host == "" || ami.Port == "" || ami.User == "" || ami.Secret == "" {
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"status":  "error",
				"message": "La configuración AMI de Asterisk está incompleta",
			})
		}
		amiInstances, err = reconcileLocalAllmonInstances(nil)
		if err != nil {
			log.Printf("allstarlink: no se pudieron obtener los nodos locales: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{
				"status": "error", "message": "No se pudieron obtener los nodos locales de Asterisk",
			})
		}
		if localNode != "" {
			filtered := make([]allmonInstance, 0, 1)
			for _, instance := range amiInstances {
				if instance.Node == localNode {
					filtered = append(filtered, instance)
				}
			}
			if len(filtered) == 0 {
				return c.JSON(http.StatusNotFound, map[string]any{
					"status": "error", "message": "El nodo local no existe en rpt.conf",
				})
			}
			amiInstances = filtered
		}
	}

	astdbPath := config.App.Sources.AstDBPath
	astdbEntries := make(map[string]AstDBEntry)
	if astdbPath != "" {
		astdbEntries, err = readAstDBEntries(astdbPath)
		if err != nil {
			log.Printf("allstarlink: no se pudo leer astdb.txt: %v", err)
			astdbEntries = make(map[string]AstDBEntry)
		}
	}
	privateNodesPath := privateNodesFilePath()
	privateEntries := make(map[string]AstDBEntry)
	if _, err := os.Stat(privateNodesPath); err == nil {
		privateEntries, err = readAstDBEntries(privateNodesPath)
		if err != nil {
			log.Printf("allstarlink: no se pudo leer privatenodes.txt: %v", err)
			privateEntries = make(map[string]AstDBEntry)
		}
	} else if !os.IsNotExist(err) {
		log.Printf("allstarlink: no se pudo acceder a privatenodes.txt: %v", err)
	}

	nodesByNumber := make(map[string]*AllStarLinkNode)
	nodeOrder := make([]string, 0)
	echoLinkCache := make(map[string]AstDBEntry)
	echoLinkLookedUp := make(map[string]bool)
	serverReachable := make(map[string]bool, len(amiInstances))
	for _, amiInstance := range amiInstances {
		localNode := amiInstance.Node
		xstat, err := config.ExecuteAMIRptStatus(amiInstance.Host, amiInstance.Port, amiInstance.User, amiInstance.Secret, localNode, "XStat", 5*time.Second)
		if err != nil {
			log.Printf("allstarlink: XStat para nodo local %s falló: %v", localNode, err)
			continue
		}
		serverReachable[localNode] = true
		sawstat, err := config.ExecuteAMIRptStatus(amiInstance.Host, amiInstance.Port, amiInstance.User, amiInstance.Secret, localNode, "SawStat", 5*time.Second)
		if err != nil {
			log.Printf("allstarlink: SawStat para nodo local %s falló: %v", localNode, err)
			sawstat = ""
		}

		connections := parseRPTConnections(xstat, sawstat)
		for _, connection := range connections {
			key := localNode + ":" + connection.Node
			if _, exists := nodesByNumber[key]; exists {
				continue
			}
			info := lookupNodeInfo(connection.Node, astdbEntries, privateEntries)
			echoLinkNode, isEchoLink := echoLinkLookupNumber(connection.Node)
			echoLinkKey := localNode + ":" + connection.Node
			if info.Callsign == "" && isEchoLink {
				if !echoLinkLookedUp[echoLinkKey] {
					echoLinkLookedUp[echoLinkKey] = true
					command := "echolink dbget nodename " + echoLinkNode
					raw, lookupErr := config.ExecuteAMICommand(amiInstance.Host, amiInstance.Port, amiInstance.User, amiInstance.Secret, command, 5*time.Second)
					if lookupErr != nil {
						log.Printf("allstarlink: lookup EchoLink para nodo %s falló: %v", connection.Node, lookupErr)
					} else if entry, found := parseEchoLinkDBResponse(connection.Node, echoLinkNode, raw); found {
						echoLinkCache[echoLinkKey] = entry
					}
				}
				if entry, found := echoLinkCache[echoLinkKey]; found {
					info = entry
				}
			}
			information := allStarLinkNodeInformation(connection.Node, info)
			mode := rptModeLabel(connection.Mode)
			actions := []AllStarLinkAction{}
			if isNumericNode(connection.Node) {
				actions = append(actions, disconnectAllStarLinkAction(localNode, connection.Node))
			}
			nodesByNumber[key] = &AllStarLinkNode{
				Protocolo: "iax", Categoria: allStarLinkNodeCategory(connection.Node),
				NodoLocal: localNode, Nodo: connection.Node,
				Informacion: information,
				Callsign:    info.Callsign, Descripcion: info.Descripcion, Ubicacion: info.Ubicacion,
				Recibido: connection.Received, Enlace: connection.Link,
				Direccion: connection.Direction, Conectado: connection.Connected,
				Modo: mode, ModoCodigo: connection.Mode,
				Acciones: actions,
			}
			nodeOrder = append(nodeOrder, key)
		}
	}

	pjsipConfigPath := config.App.AMI.PJSIPConfigPath
	if pjsipConfigPath == "" {
		pjsipConfigPath = "/etc/asterisk/pjsip.conf"
	}
	pjsipCallerIDs, err := readPJSIPEndpointCallerIDs(pjsipConfigPath)
	if err != nil {
		log.Printf("allstarlink: no se pudieron leer callerid de PJSIP: %v", err)
		pjsipCallerIDs = map[string]string{}
	}

	for _, amiInstance := range amiInstances {
		channelsOutput, queryErr := config.ExecuteAMICommand(amiInstance.Host, amiInstance.Port, amiInstance.User, amiInstance.Secret, "core show channels concise", 5*time.Second)
		if queryErr != nil {
			log.Printf("allstarlink: no se pudieron consultar canales activos para nodo local %s: %v", amiInstance.Node, queryErr)
			continue
		}
		serverReachable[amiInstance.Node] = true
		activeSIPEndpoints := parseActiveSIPEndpoints(channelsOutput)

		// "pjsip list contacts" existe en todas las versiones con PJSIP; "show contacts" solo en algunas.
		pjsipOutput, queryErr := config.ExecuteAMICommand(amiInstance.Host, amiInstance.Port, amiInstance.User, amiInstance.Secret, "pjsip list contacts", 5*time.Second)
		if queryErr != nil && isMissingAMICommand(queryErr) {
			// Sin res_pjsip cargado no existe ningún comando pjsip.
			continue
		}
		if queryErr != nil {
			pjsipOutput, queryErr = config.ExecuteAMICommand(amiInstance.Host, amiInstance.Port, amiInstance.User, amiInstance.Secret, "pjsip show contacts", 5*time.Second)
		}
		if queryErr != nil && isMissingAMICommand(queryErr) {
			continue
		}
		if queryErr != nil {
			log.Printf("allstarlink: no se pudieron consultar contactos PJSIP para nodo local %s: %v", amiInstance.Node, queryErr)
			continue
		}
		serverReachable[amiInstance.Node] = true
		for _, contact := range parseConnectedSIPContacts(pjsipOutput) {
			if _, active := activeSIPEndpoints[contact.Endpoint]; !active {
				continue
			}
			key := "sip:" + amiInstance.Node + ":" + contact.Endpoint + ":" + contact.URI
			if _, exists := nodesByNumber[key]; exists {
				continue
			}
			callsign := pjsipCallerIDs[contact.Endpoint]
			nodesByNumber[key] = &AllStarLinkNode{
				Protocolo: "sip", Categoria: "sip",
				NodoLocal: amiInstance.Node, Nodo: contact.Endpoint, Contacto: contact.URI,
				Informacion: callsign, Callsign: callsign, Enlace: contact.Status,
				Latencia: contact.Latency,
			}
			nodeOrder = append(nodeOrder, key)
		}
	}

	resultados := make([]AllStarLinkNode, 0, len(nodesByNumber))
	for _, nodeNumber := range nodeOrder {
		resultados = append(resultados, *nodesByNumber[nodeNumber])
	}
	activities := loadLastNodeActivities(amiInstances)
	for i := range resultados {
		if activity, found := activities[resultados[i].NodoLocal+":"+resultados[i].Nodo]; found {
			resultados[i].UltimaActividad = &activity
		}
	}
	tablas := buildAllStarLinkLocalTables(amiInstances, resultados, serverReachable)

	return c.JSON(http.StatusOK, map[string]any{
		"status": "success",
		"data": map[string]any{
			"count":          len(resultados),
			"resultados":     resultados,
			"tablas":         tablas,
			"nodo_principal": primaryNode,
		},
	})
}

// isMissingAMICommand indica que Asterisk respondió "No such command".
func isMissingAMICommand(err error) bool {
	return err != nil && strings.Contains(err.Error(), "No such command")
}

// allStarLinkNodeInformation compone el texto mostrado para nodos EchoLink, IAXRpt o entradas de AstDB.
func allStarLinkNodeInformation(node string, info AstDBEntry) string {
	if _, isEchoLink := echoLinkLookupNumber(node); isEchoLink {
		information := "EchoLink " + node
		if info.Callsign != "" {
			information = fmt.Sprintf("%s [EchoLink %s]", info.Callsign, node)
		}
		if info.Descripcion != "" {
			information += " - " + info.Descripcion
		}
		if info.Ubicacion != "" {
			information += " (" + info.Ubicacion + ")"
		}
		return information
	}
	if info.Callsign == "" && !isNumericNode(node) {
		return "IAXRpt " + node
	}
	parts := make([]string, 0, 3)
	for _, value := range []string{info.Callsign, info.Descripcion, info.Ubicacion} {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " ")
}

// buildAllStarLinkLocalTables agrupa los nodos conectados y su estado por nodo local.
func buildAllStarLinkLocalTables(instances []allmonInstance, nodes []AllStarLinkNode, serverReachable map[string]bool) []AllStarLinkLocalTable {
	tables := make([]AllStarLinkLocalTable, 0, len(instances))
	tableByLocalNode := make(map[string]int, len(instances))
	for _, instance := range instances {
		if _, exists := tableByLocalNode[instance.Node]; exists {
			continue
		}
		tableByLocalNode[instance.Node] = len(tables)
		tables = append(tables, AllStarLinkLocalTable{
			NodoLocal: instance.Node, ServidorConectado: serverReachable[instance.Node],
			NodosDisponibles: make([]AllStarLinkNodeOption, 0),
			Acciones:         []AllStarLinkAction{lsNodesPopupAction(instance.Node)},
		})
	}
	for _, node := range nodes {
		if node.Protocolo != "iax" || !isNumericNode(node.Nodo) {
			continue
		}
		index, exists := tableByLocalNode[node.NodoLocal]
		if !exists {
			continue
		}
		label := node.Nodo
		if node.Informacion != "" {
			label += " - " + node.Informacion
		}
		tables[index].NodosDisponibles = append(tables[index].NodosDisponibles, AllStarLinkNodeOption{
			Value: node.Nodo, Label: label,
		})
	}
	return tables
}

// disconnectAllStarLinkAction crea la acción de API para desconectar un nodo remoto.
func disconnectAllStarLinkAction(localNode, remoteNode string) AllStarLinkAction {
	return AllStarLinkAction{
		ID:       "disconnect",
		Label:    "Desconectar",
		Method:   http.MethodPost,
		Endpoint: "/mon/asterisk/commands",
		Payload: &SupermonCommandRequest{
			LocalNode:  localNode,
			RemoteNode: remoteNode,
			Action:     "disconnect",
		},
	}
}

// lsNodesPopupAction crea la acción que abre LsNodes para el nodo local indicado.
func lsNodesPopupAction(localNode string) AllStarLinkAction {
	query := url.Values{"node": []string{localNode}}
	return AllStarLinkAction{
		ID:           "lsnodes",
		Label:        "Ver LsNodes",
		Method:       http.MethodGet,
		Endpoint:     "/mon/sm_lsnodes",
		Presentation: "popup_iframe",
		IframeURL:    "/mon/sm_lsnodes?" + query.Encode(),
	}
}

// echoLinkLookupNumber obtiene el número de consulta DB asociado a un nodo EchoLink.
func echoLinkLookupNumber(node string) (string, bool) {
	if len(node) < 5 || node[0] != '3' {
		return "", false
	}
	lookup, err := strconv.ParseUint(node[1:], 10, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatUint(lookup, 10), true
}

// allStarLinkNodeCategory conserva las categorías del contrato del listado.
func allStarLinkNodeCategory(node string) string {
	if len(node) == 4 && isNumericNode(node) {
		return "nodo_privado"
	}
	if _, isEchoLink := echoLinkLookupNumber(node); isEchoLink {
		return "echolink"
	}
	return "iax"
}

// parseEchoLinkDBResponse extrae de la respuesta AMI la entrada del nodo EchoLink solicitado.
func parseEchoLinkDBResponse(node, lookupNumber, raw string) (AstDBEntry, bool) {
	for _, line := range config.ParseAMIOutputToLines(raw) {
		fields := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(fields) < 3 || strings.TrimSpace(fields[0]) != lookupNumber {
			continue
		}
		return AstDBEntry{
			Nodo:      node,
			Callsign:  strings.TrimSpace(fields[1]),
			Ubicacion: strings.TrimSpace(fields[2]),
		}, true
	}
	return AstDBEntry{}, false
}

// readPJSIPEndpointCallerIDs obtiene los caller ID de las secciones endpoint de un pjsip.conf.
func readPJSIPEndpointCallerIDs(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	callerIDs := make(map[string]string)
	section := ""
	sectionType := ""
	callerID := ""
	storeEndpoint := func() {
		if section != "" && strings.EqualFold(sectionType, "endpoint") && callerID != "" {
			callerIDs[section] = callerID
		}
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			storeEndpoint()
			sectionEnd := strings.IndexByte(line, ']')
			if sectionEnd < 0 {
				section, sectionType, callerID = "", "", ""
				continue
			}
			section = strings.TrimSpace(line[1:sectionEnd])
			sectionType, callerID = "", ""
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "type":
			sectionType = strings.TrimSpace(value)
		case "callerid":
			callerID = normalizePJSIPCallerID(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	storeEndpoint()
	return callerIDs, nil
}

// normalizePJSIPCallerID quita comentarios y extensiones entre ángulos del caller ID.
func normalizePJSIPCallerID(value string) string {
	value = strings.TrimSpace(value)
	if comment := strings.Index(value, " ;"); comment >= 0 {
		value = strings.TrimSpace(value[:comment])
	}
	if openAngle := strings.LastIndex(value, "<"); openAngle >= 0 {
		value = strings.TrimSpace(value[:openAngle])
	}
	return strings.Trim(strings.TrimSpace(value), "\"'")
}

// readLocalRPTNodes lee los identificadores numéricos de las secciones de rpt.conf.
func readLocalRPTNodes(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	nodes := make([]string, 0)
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "[") {
			continue
		}
		sectionEnd := strings.IndexByte(line, ']')
		if sectionEnd < 0 {
			continue
		}
		node := strings.TrimSpace(line[1:sectionEnd])
		if !isNumericNode(node) {
			continue
		}
		if _, exists := seen[node]; exists {
			continue
		}
		seen[node] = struct{}{}
		nodes = append(nodes, node)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func readPrivateNodeEntries() (map[string]AstDBEntry, error) {
	entries, err := readAstDBEntries(privateNodesFilePath())
	if os.IsNotExist(err) {
		return map[string]AstDBEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("leer privatenodes.txt: %w", err)
	}
	return entries, nil
}

func isPrivateMonitorNode(node string, entries map[string]AstDBEntry) bool {
	_, listed := entries[node]
	return listed || allStarLinkNodeCategory(node) == "nodo_privado"
}

// readPrimaryRPTNode exige una única entrada loopback pública en [nodes], sin depender del orden.
func readPrimaryRPTNode(path string) (string, error) {
	privateNodes, err := readPrivateNodeEntries()
	if err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	primary := ""
	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if comment := strings.IndexAny(line, ";#"); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}
		if strings.HasPrefix(line, "[") {
			if end := strings.IndexByte(line, ']'); end >= 0 {
				section = strings.ToLower(strings.TrimSpace(line[1:end]))
			}
			continue
		}
		if section != "nodes" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		node := strings.TrimSpace(key)
		if !isNumericNode(node) {
			continue
		}
		if isPrivateMonitorNode(node, privateNodes) {
			continue
		}
		expected := "radio@127.0.0.1/" + node + ",NONE"
		if strings.EqualFold(strings.TrimSpace(value), expected) {
			if primary != "" && primary != node {
				return "", fmt.Errorf("rpt.conf contiene varios nodos principales posibles: %s y %s", primary, node)
			}
			primary = node
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return primary, nil
}

type rptConnection struct {
	Node, RemoteIP, Direction, Connected, Link, Mode, Received string
}

type sipContact struct {
	Endpoint, URI, Status, Latency string
}

// parseConnectedSIPContacts extrae los contactos SIP registrados de la salida de Asterisk.
func parseConnectedSIPContacts(raw string) []sipContact {
	contacts := make([]sipContact, 0)
	seen := make(map[string]struct{})
	for _, line := range config.ParseAMIOutputToLines(raw) {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] != "Contact:" {
			continue
		}
		endpoint, uri, ok := strings.Cut(strings.Trim(fields[1], "<>"), "/")
		if !ok || endpoint == "" || uri == "" {
			continue
		}
		statusIndex := len(fields) - 1
		latency := ""
		if len(fields) >= 5 {
			statusIndex = len(fields) - 2
			latency = fields[len(fields)-1]
		}
		status := fields[statusIndex]
		if !isRegisteredSIPStatus(status) {
			continue
		}
		key := endpoint + "/" + uri
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		contacts = append(contacts, sipContact{
			Endpoint: endpoint,
			URI:      uri,
			Status:   status,
			Latency:  latency,
		})
	}
	return contacts
}

// isRegisteredSIPStatus indica si el estado PJSIP representa un contacto registrado.
func isRegisteredSIPStatus(status string) bool {
	switch strings.ToLower(status) {
	case "avail", "reachable", "nonqual":
		return true
	default:
		return false
	}
}

// parseActiveSIPEndpoints devuelve los endpoints PJSIP con un canal activo en Asterisk.
func parseActiveSIPEndpoints(raw string) map[string]struct{} {
	endpoints := make(map[string]struct{})
	for _, line := range config.ParseAMIOutputToLines(raw) {
		fields := strings.Split(strings.TrimSpace(line), "!")
		if len(fields) < 5 || !strings.EqualFold(strings.TrimSpace(fields[4]), "Up") {
			continue
		}

		channel := strings.TrimSpace(fields[0])
		const prefix = "PJSIP/"
		if !strings.HasPrefix(channel, prefix) {
			continue
		}
		channel = strings.TrimPrefix(channel, prefix)
		separator := strings.LastIndex(channel, "-")
		if separator <= 0 || separator == len(channel)-1 {
			continue
		}
		endpoints[channel[:separator]] = struct{}{}
	}
	return endpoints
}

// parseRPTConnections combina XStat y SawStat para construir el estado de cada enlace RPT.
func parseRPTConnections(xstat, sawstat string) []rptConnection {
	modes := make(map[string]string)
	connections := make([]rptConnection, 0)
	for _, line := range config.ParseAMIOutputToLines(xstat) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "LinkedNodes:" {
			for _, linked := range strings.Split(strings.Join(fields[1:], ""), ",") {
				if len(linked) > 1 {
					modes[linked[1:]] = linked[:1]
				}
			}
			continue
		}
		if fields[0] != "Conn:" || !isValidRemoteNodeID(fields[1]) {
			continue
		}
		connection := rptConnection{Node: fields[1]}
		if len(fields) >= 3 {
			connection.RemoteIP = fields[2]
		}
		if len(fields) >= 7 {
			connection.Direction, connection.Connected, connection.Link = fields[4], fields[5], fields[6]
		} else if len(fields) >= 5 {
			connection.Direction, connection.Connected = fields[3], fields[4]
		}
		connections = append(connections, connection)
	}

	received := make(map[string]string)
	for _, line := range config.ParseAMIOutputToLines(sawstat) {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "Conn:" && isValidRemoteNodeID(fields[1]) {
			received[fields[1]] = formatReceivedTime(fields[3])
		}
	}

	for i := range connections {
		connection := &connections[i]
		connection.Mode = modes[connection.Node]
		if connection.Mode == "" {
			connection.Mode = "L"
		}
		if connection.Link == "" {
			if connection.Mode == "C" {
				connection.Link = "CONNECTING"
			} else {
				connection.Link = "ESTABLISHED"
			}
		}
		connection.Received = received[connection.Node]
		if connection.Received == "" {
			connection.Received = "Never"
		}
	}
	return connections
}

// formatReceivedTime convierte segundos recibidos a HH:MM:SS o devuelve Never si no son válidos.
func formatReceivedTime(seconds string) string {
	if seconds == "-1" {
		return "Never"
	}
	var elapsed int64
	if _, err := fmt.Sscan(seconds, &elapsed); err != nil || elapsed < 0 {
		return "Never"
	}
	return fmt.Sprintf("%03d:%02d:%02d", elapsed/3600, (elapsed/60)%60, elapsed%60)
}

// rptModeLabel traduce el código de modo RPT a una etiqueta legible.
func rptModeLabel(mode string) string {
	switch mode {
	case "T":
		return "Transceive"
	case "R":
		return "Receive Only"
	default:
		return "Local Monitor"
	}
}

// isNumericNode comprueba que el identificador del nodo contenga solo dígitos.
func isNumericNode(node string) bool {
	if node == "" {
		return false
	}
	for _, digit := range node {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// isValidRemoteNodeID valida que el identificador remoto use solo caracteres permitidos.
func isValidRemoteNodeID(node string) bool {
	if node == "" || len(node) > 20 || strings.TrimSpace(node) != node {
		return false
	}
	for _, char := range node {
		if (char < '0' || char > '9') &&
			(char < 'A' || char > 'Z') &&
			(char < 'a' || char > 'z') && char != '-' && char != '_' {
			return false
		}
	}
	return true
}

// lookupNodeInfo busca la información del nodo, priorizando la lista de nodos privados.
func lookupNodeInfo(node string, astdbEntries, privateEntries map[string]AstDBEntry) AstDBEntry {
	if len(node) == 4 {
		if entry, found := privateEntries[node]; found {
			return entry
		}
	}
	if entry, found := astdbEntries[node]; found {
		return entry
	}
	return AstDBEntry{Nodo: node}
}

// readAstDBEntries carga un archivo delimitado por barras verticales en un mapa de nodos.
func readAstDBEntries(path string) (map[string]AstDBEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries := make(map[string]AstDBEntry)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "|")
		if len(fields) < 2 {
			continue
		}

		entry := AstDBEntry{Nodo: strings.TrimSpace(fields[0]), Callsign: strings.TrimSpace(fields[1])}
		if len(fields) > 2 {
			entry.Descripcion = strings.TrimSpace(fields[2])
		}
		if len(fields) > 3 {
			entry.Ubicacion = strings.TrimSpace(fields[3])
		}
		entries[entry.Nodo] = entry
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
