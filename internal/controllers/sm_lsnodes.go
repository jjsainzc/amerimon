package controllers

import (
	"fmt"
	htmlstd "html"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

// Estructuras para organizar los datos de las tablas
type StatItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type LStatRow struct {
	Node       string `json:"node"`
	Peer       string `json:"peer"`
	Reconnects string `json:"reconnects"`
	Direction  string `json:"direction"`
	Time       string `json:"connect_time"`
	State      string `json:"connect_state"`
}

type NodeRow struct {
	Node     string `json:"node"`
	Callsign string `json:"callsign"`
	Info     string `json:"info"`
	Location string `json:"location"`
	Direct   bool   `json:"direct"`
}

type RegistryRow struct {
	Host      string `json:"host"`
	Username  string `json:"username"`
	Perceived string `json:"perceived"`
	Refresh   string `json:"refresh"`
	State     string `json:"state"`
}

type AllStarNodesResponse struct {
	Node           int           `json:"node"`
	NodeInfo       NodeRow       `json:"node_info"`
	MyIP           string        `json:"my_ip"`
	UpdatedAt      time.Time     `json:"updated_at"`
	RefreshSeconds int           `json:"refresh_seconds"`
	Stats          []StatItem    `json:"stats"`
	ConnectedNodes []NodeRow     `json:"connected_nodes"`
	Connections    []LStatRow    `json:"connections"`
	IAXRegistry    []RegistryRow `json:"iax_registry"`
	RPTRegistry    []RegistryRow `json:"rpt_registry"`
}

// HandleAllStarNodes devuelve el estado y los nodos conectados de un nodo AllStar en HTML o JSON.
// HandleAllStarNodes godoc
// @Summary Estado y nodos conectados de un nodo AllStar
// @Description Consulta Asterisk por AMI (rpt nodes, stats y lstats, registros IAX y RPT). Con format=json devuelve AllStarNodesResponse; con html, una página con refresco automático.
// @Tags AllStarLink
// @Produce json
// @Produce html
// @Param nodes query int false "Número de nodo (1-999999); por compatibilidad también se acepta node" default(1999)
// @Param format query string false "Formato de respuesta" Enums(html, json) default(html)
// @Param command query string false "Comando mostrado en la página HTML"
// @Success 200 {object} AllStarNodesResponse
// @Failure 400 {string} string "Nodo o formato inválido"
// @Router /sm_lsnodes [get]
func HandleAllStarNodes(c echo.Context) error {
	// 1. Obtener y validar el nodo. Se conserva "node" por compatibilidad.
	nodeStr := c.QueryParam("nodes")
	if nodeStr == "" {
		nodeStr = c.QueryParam("node")
	}
	if nodeStr == "" {
		nodeStr = "1999" // Nodo de ejemplo por defecto si no se provee
	}
	node, err := strconv.Atoi(nodeStr)
	if err != nil || node <= 0 || node > 999999 {
		return c.String(http.StatusBadRequest, "Invalid node number")
	}

	format := strings.ToLower(strings.TrimSpace(c.QueryParam("format")))
	if format == "" {
		format = "html"
	}
	if format != "html" && format != "json" {
		return c.String(http.StatusBadRequest, "Invalid format: use html or json")
	}

	command := c.QueryParam("command")

	// 2. Consultar Asterisk a través de AMI, evitando depender de sudo y del socket local.
	connectedNodesOut := executeAllStarAMICommand(fmt.Sprintf("rpt nodes %d", node))
	nodeStatusOut := executeAllStarAMICommand(fmt.Sprintf("rpt stats %d", node))
	nodeLStatusOut := executeAllStarAMICommand(fmt.Sprintf("rpt lstats %d", node))
	iaxRegistryOut := executeAllStarAMICommand("iax2 show registry")
	rptRegistryOut := executeAllStarAMICommand("rpt show registrations")
	myIP := fetchPublicIP()

	// 3. Leer base de datos astdb.txt para buscar el nodo principal
	var nodeInfo NodeRow
	astDBPath := config.App.Sources.AstDBPath
	if astDBPath == "" {
		astDBPath = "/var/www/html/supermon/astdb.txt"
	}
	astdbEntries, err := readAstDBEntries(astDBPath)
	if err != nil {
		log.Printf("sm_lsnodes: error leyendo %s: %v", astDBPath, err)
		astdbEntries = map[string]AstDBEntry{}
	}
	localNodeInfo := lookupNodeInfo(strconv.Itoa(node), astdbEntries, nil)
	nodeInfo = NodeRow{
		Node: localNodeInfo.Nodo, Callsign: localNodeInfo.Callsign,
		Info: localNodeInfo.Descripcion, Location: localNodeInfo.Ubicacion,
	}
	if nodeInfo.Node == "" {
		nodeInfo.Node = strconv.Itoa(node)
	}

	// 4. Procesar el estado del nodo (stats)
	statArray := parseAllStarStats(nodeStatusOut)

	// 5. Procesar lstats (conexiones activas detalladas)
	lstatArray := parseAllStarLStats(nodeLStatusOut)
	directNodes := make(map[string]struct{}, len(lstatArray))
	for _, row := range lstatArray {
		directNodes[row.Node] = struct{}{}
	}
	connectedNodes := enrichAllStarConnectedNodes(parseAllStarConnectedNodes(connectedNodesOut, directNodes), astdbEntries)

	// 6. Calcular tiempo de refresco HTML
	delay := 180
	if strings.Contains(command, "*") {
		delay = 120
	}

	now := time.Now()
	response := AllStarNodesResponse{
		Node: node, NodeInfo: nodeInfo, MyIP: myIP, UpdatedAt: now,
		RefreshSeconds: delay, Stats: statArray, ConnectedNodes: connectedNodes,
		Connections: lstatArray, IAXRegistry: parseRegistryRows(iaxRegistryOut),
		RPTRegistry: parseRegistryRows(rptRegistryOut),
	}
	if response.Stats == nil {
		response.Stats = []StatItem{}
	}
	if response.ConnectedNodes == nil {
		response.ConnectedNodes = []NodeRow{}
	}
	if response.Connections == nil {
		response.Connections = []LStatRow{}
	}
	if response.IAXRegistry == nil {
		response.IAXRegistry = []RegistryRow{}
	}
	if response.RPTRegistry == nil {
		response.RPTRegistry = []RegistryRow{}
	}
	if format == "json" {
		return c.JSON(http.StatusOK, response)
	}

	// 7. Construcción de la respuesta HTML final
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta http-equiv="refresh" content="%d;url=/mon/sm_lsnodes?nodes=%d&format=html">
<title>AllStarLink Connected Nodes and Status</title>
<style type="text/css">
table.status { background-color: #D6C7B7; font-size:.6em; font-family: Verdana, Arial, sans-serif; margin-left:0; margin-right:3%%; margin-bottom:2%%; border: 1px solid black; padding: .6em; }
table.nodes { font-size:.6em; font-family: Verdana, Arial, sans-serif; margin-bottom:2%%; border: 1px solid black; padding: .6em; }
th { text-align:left; font-weight: bold; height:1.4em; font-size:1.0em; text-decoration:underline; }
div.clear { clear:both; }
div.main { width: 99%%; margin: 0 auto; }
div.left { position:absolute; top:90px; left:10px; width:35%%; font-family: Verdana, Arial, sans-serif; }
div.right { position:absolute; top:90px; left:380px; width:599px; height:85%%; overflow:auto; }
p.header { font-size:1.1em; font-family: Verdana, Arial, sans-serif; margin:0; text-align:center; }
p.header2 { font-size:.7em; font-family: Verdana, Arial, sans-serif; margin:0; text-align:center; }
</style>
</head>
<body>
<div id="container" class="main">
<div id="header">
<p class="header">Status for %s - Node %d</p>
<p class="header2">Last update - %02d/%02d/%4d %02d:%02d:%02d &nbsp; &nbsp; My IP - %s</p>
</div>`, response.RefreshSeconds, response.Node, response.NodeInfo.Callsign, response.Node, now.Month(), now.Day(), now.Year(), now.Hour(), now.Minute(), now.Second(), response.MyIP)

	if command != "" {
		html += fmt.Sprintf("<p>Command = %s</p>", command)
	}

	html += fmt.Sprintf("<p style=\"text-align:center\"><a href=\"http://stats.AllStarLink.org/getstatus.cgi?%d\">View this Node Graphically</a></p>", response.Node)

	// Tabla izquierda (Stats)
	html += "<div id=\"left_table\" class=\"left\"><table class=\"status\" cellspacing=\"0\" cellpadding=\"0\" width=\"360\" bgcolor=\"powderblue\">"
	for _, item := range response.Stats {
		html += fmt.Sprintf("<tr><td>%s</td><td>%s</td></tr>", item.Key, item.Value)
	}
	html += "</table></div>"

	// Tabla derecha (Nodes conectados)
	html += "<div id=\"right\" class=\"right\"><table class=\"nodes\" cellspacing=\"0\" cellpadding=\"0\" width=\"100%\">"
	html += "<tr bgcolor=\"lightblue\"><th>Node</th><th>Callsign</th><th>Info</th><th>Location</th></tr>"
	html += fmt.Sprintf("<tr bgcolor=\"#FF90AF\"><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>", response.NodeInfo.Node, response.NodeInfo.Callsign, response.NodeInfo.Info, response.NodeInfo.Location)
	for _, connectedNode := range response.ConnectedNodes {
		displayNodeID := connectedNode.Node
		if connectedNode.Direct {
			displayNodeID += "*"
		}
		html += fmt.Sprintf("<tr bgcolor=\"%s\"><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>", allStarConnectedNodeColor(connectedNode), displayNodeID, connectedNode.Callsign, connectedNode.Info, connectedNode.Location)
	}
	html += "</table>"

	// Tabla LStats
	html += "<table class=\"nodes\" cellspacing=\"0\" cellpadding=\"0\" width=\"100%\" bgcolor=\"#00BFFF\">"
	html += "<tr><th>Node</th><th>Peer</th><th>Reconnects</th><th>Direction</th><th>Connect Time</th><th>Connect State</th></tr>"
	for _, row := range response.Connections {
		html += fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
			row.Node, row.Peer, row.Reconnects, row.Direction, row.Time, row.State)
	}
	html += "</table>"

	// Registros IAX y RPT como tablas de columnas tipadas.
	html += renderRegistryTable("IAX Registry", response.IAXRegistry)
	html += renderRegistryTable("RPT Registrations", response.RPTRegistry)
	html += "</div></div></body></html>"

	return c.HTML(http.StatusOK, html)
}

// executeAllStarAMICommand ejecuta un comando por AMI y devuelve sus líneas unidas; registra errores y devuelve vacío si falla.
func executeAllStarAMICommand(command string) string {
	ami := config.App.AMI
	if ami.Host == "" || ami.Port == "" || ami.User == "" || ami.Secret == "" {
		log.Printf("sm_lsnodes: configuración AMI incompleta; no se ejecutó %q", command)
		return ""
	}

	raw, err := config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, command, 5*time.Second)
	if err != nil {
		log.Printf("sm_lsnodes: error ejecutando %q por AMI: %v", command, err)
		return ""
	}
	return strings.Join(config.ParseAMIOutputToLines(raw), "\n")
}

// fetchPublicIP consulta icanhazip.com y devuelve la IP pública o una cadena vacía si la consulta falla.
func fetchPublicIP() string {
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("https://icanhazip.com")
	if err != nil {
		log.Printf("sm_lsnodes: no se pudo consultar la IP pública: %v", err)
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("sm_lsnodes: no se pudo leer la IP pública: %v", err)
		return ""
	}
	return strings.TrimSpace(string(body))
}

// parseAllStarLStats convierte la salida de rpt lstats en filas de conexión y omite las líneas no reconocidas.
func parseAllStarLStats(raw string) []LStatRow {
	var rows []LStatRow
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 5 && (fields[2] == "IN" || fields[2] == "OUT") {
			fields = []string{fields[0], "", fields[1], fields[2], fields[3], fields[4]}
		}
		if len(fields) < 6 || (fields[3] != "IN" && fields[3] != "OUT") {
			continue
		}

		state := fields[5]
		for _, knownState := range []string{"ESTABLISHED", "CONNECTING"} {
			if strings.HasPrefix(state, knownState) && allDigits(state[len(knownState):]) {
				state = knownState
				break
			}
		}
		rows = append(rows, LStatRow{
			Node:       fields[0],
			Peer:       fields[1],
			Reconnects: fields[2],
			Direction:  fields[3],
			Time:       fields[4],
			State:      state,
		})
	}
	return rows
}

// parseAllStarStats extrae pares clave-valor de la salida de rpt stats.
func parseAllStarStats(raw string) []StatItem {
	items := make([]StatItem, 0)
	for index, line := range strings.Split(raw, "\n") {
		if index < 2 {
			continue
		}
		line = strings.ReplaceAll(line, ".", "")
		line = strings.ReplaceAll(line, ",", "")
		line = strings.ReplaceAll(line, ": ", "&")
		parts := strings.Split(line, "&")
		for partIndex := 0; partIndex < len(parts)-1; partIndex += 2 {
			items = append(items, StatItem{
				Key: strings.TrimSpace(parts[partIndex]), Value: strings.TrimSpace(parts[partIndex+1]),
			})
		}
	}
	return items
}

// parseAllStarConnectedNodes interpreta la salida de rpt nodes y marca los nodos presentes en directNodes.
func parseAllStarConnectedNodes(raw string, directNodes map[string]struct{}) []NodeRow {
	nodes := make([]NodeRow, 0)
	lines := strings.Split(raw, "\n")
	if len(lines) > 3 {
		raw = strings.Join(lines[3:], " ")
	}
	raw = strings.ReplaceAll(raw, ",", " ")
	for _, token := range strings.Fields(raw) {
		if strings.EqualFold(token, "Output:") || strings.EqualFold(token, "NONE") || len(token) <= 1 {
			continue
		}
		// The legacy script removes the one-character link-type prefix (usually T).
		nodeNumber := token[1:]
		if len(nodeNumber) > 10 {
			nodeNumber = nodeNumber[:10]
		}
		if !allDigits(nodeNumber) {
			nodes = append(nodes, NodeRow{
				Node: "User", Callsign: nodeNumber,
				Info: "IaxRpt, Web Txcvr, or -P = Phone Portal", Location: "-----",
			})
			continue
		}
		_, direct := directNodes[nodeNumber]
		nodes = append(nodes, NodeRow{
			Node: nodeNumber, Callsign: "No-Info", Info: "Node not in database",
			Location: "-----", Direct: direct,
		})
	}
	return nodes
}

// enrichAllStarConnectedNodes completa cada nodo conectado con datos de EchoLink o AstDB.
func enrichAllStarConnectedNodes(nodes []NodeRow, astdbEntries map[string]AstDBEntry) []NodeRow {
	for index := range nodes {
		node := &nodes[index]
		if node.Node == "User" {
			continue
		}
		if lookupNumber, isEchoLink := echoLinkLookupNumber(node.Node); isEchoLink {
			command := "echolink dbget nodename " + lookupNumber
			if raw := executeAllStarAMICommand(command); raw != "" {
				if info, found := parseEchoLinkDBResponse(node.Node, lookupNumber, raw); found {
					applyAllStarNodeInfo(node, info)
					continue
				}
			}
		}
		applyAllStarNodeInfo(node, lookupNodeInfo(node.Node, astdbEntries, nil))
	}
	return nodes
}

// allStarConnectedNodeColor elige el color de presentación según el tipo de nodo.
func allStarConnectedNodeColor(node NodeRow) string {
	if node.Node == "User" {
		return "lightgray"
	}
	if _, isEchoLink := echoLinkLookupNumber(node.Node); isEchoLink {
		return "lightgreen"
	}
	return "yellow"
}

// applyAllStarNodeInfo copia los datos de AstDB en la fila o asigna los textos de ausencia de información.
func applyAllStarNodeInfo(node *NodeRow, info AstDBEntry) {
	if info.Callsign == "" {
		node.Callsign = "No-Info"
		node.Info = "Node not in database"
		node.Location = "-----"
		return
	}
	node.Callsign = info.Callsign
	node.Info = info.Descripcion
	node.Location = info.Ubicacion
}

// parseRegistryRows convierte las filas de salida de los registros IAX/RPT y descarta encabezados y separadores.
func parseRegistryRows(raw string) []RegistryRow {
	rows := make([]RegistryRow, 0)
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 5 || isRegistrySeparator(fields) || containsRegistryHeader(fields) {
			continue
		}
		rows = append(rows, RegistryRow{
			Host: fields[0], Username: fields[1], Perceived: fields[2],
			Refresh: fields[3], State: strings.Join(fields[4:], " "),
		})
	}
	return rows
}

// containsRegistryHeader indica si los primeros campos corresponden a un encabezado del registro.
func containsRegistryHeader(fields []string) bool {
	for index, field := range fields {
		if index >= 5 {
			break
		}
		switch strings.ToLower(field) {
		case "host", "username", "perceived", "refresh", "state":
			return true
		}
	}
	return false
}

// isRegistrySeparator indica si todos los campos de la fila contienen únicamente guiones o signos más.
func isRegistrySeparator(fields []string) bool {
	for _, field := range fields {
		if strings.Trim(field, "-+") != "" {
			return false
		}
	}
	return true
}

// renderRegistryTable genera una tabla HTML y escapa el título y los valores para mostrarlos como texto.
func renderRegistryTable(title string, rows []RegistryRow) string {
	var table strings.Builder
	table.WriteString("<table class=\"nodes\" cellspacing=\"0\" cellpadding=\"0\" width=\"100%\" bgcolor=\"#F0E68C\">")
	fmt.Fprintf(&table, "<tr><th colspan=\"5\">%s</th></tr>", htmlstd.EscapeString(title))
	table.WriteString("<tr><th>Host</th><th>Username</th><th>Perceived</th><th>Refresh</th><th>State</th></tr>")
	for _, row := range rows {
		fmt.Fprintf(&table, "<tr><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>",
			htmlstd.EscapeString(row.Host), htmlstd.EscapeString(row.Username),
			htmlstd.EscapeString(row.Perceived), htmlstd.EscapeString(row.Refresh),
			htmlstd.EscapeString(row.State))
	}
	table.WriteString("</table>")
	return table.String()
}

// allDigits indica si value no está vacío y contiene exclusivamente dígitos decimales.
func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
