// Este archivo implementa la comunicación y actualización de la pantalla Nextion.

/*
=================================================================================

	DOCUMENTACIÓN DE VARIABLES Y COMPONENTES - NEXTION EDITOR

=================================================================================
Para tu pantalla de 3.5" (NX4832K035) con resolución 480x320 en Modo Oscuro.
Asegúrate de que la propiedad 'vscope' de cada componente esté configurada en 'global'.

El firmware es único para todos los idiomas: Amerimon envía los textos de los
botones y títulos por serie según el idioma elegido en la cabecera de la web
(va_lang.val=0 para español y 1 para inglés; la pantalla usa va_lang solo para
sus textos internos de respaldo cuando pierde la conexión con el servidor).

---------------------------------------------------------------------------------
1. COMPONENTES DE TEXTO (SALIDAS DE INFORMACIÓN)
---------------------------------------------------------------------------------
  - t_nodo   : Muestra el número de tu nodo local. (Ej: "Nodo: 616262")
  - t_ip     : Dirección IP de la laptop/servidor Amerimon. (Ej: "IP: 192.168.0.177")
  - t_date   : Fecha del servidor en formato YYYY-MM-DD.
  - t_time   : Hora del servidor en formato HH:mm:ss (24 horas).
  - t_temp   : Temperatura actual del procesador. (Ej: "CPU: 45.2 °C")
  - t_volt   : Voltaje del sistema.
  - t_cpu    : Porcentaje de uso del procesador.
  - t_mem    : Porcentaje de memoria ocupada.
  - t_dsk    : Porcentaje de disco ocupado en la partición raíz.
  - t0       : Indicativo o número y descripción del origen del audio; vacío sin actividad.
  - t_n1 a t_n8 : Indicativo o caller ID de las primeras ocho conexiones activas.
  - t_actions, t_links, t_hint : Títulos y ayuda táctil; sus textos llegan por serie.

---------------------------------------------------------------------------------
2. COMPONENTES DE BOTÓN / INTERFACES TÁCTILES (ENTRADAS DESDE PANTALLA)
---------------------------------------------------------------------------------
Cada botón debe enviar su identificador de texto en el evento "Touch Release Event"
dentro de Nextion Editor usando el comando: print "COMANDO"
(los botones b_d1 a b_d8 requieren además un "Touch Press Event"; ver abajo).

  - b_fav       : Botón para conectar al Nodo Favorito fijo.
    -> Enviar por evento: print "CONN_FAV"
    -> Su texto (.txt) lo envía Amerimon según el idioma elegido en la web,
    igual que b_disc_all, b_restart, b_reboot, b_poweroff, b_admin_no y b_admin_ok.

  - b_disc_all  : Botón global rojo para colgar todos los enlaces (*70).
    -> Enviar por evento: print "DISC_ALL"

  - b_d1 a b_d8 : Botones transparentes/invisibles encimados exactamente sobre sus
    respectivas cajas de texto (t_n1 a t_n8). El script en Go maneja
    su visibilidad de manera automática mediante comandos 'vis'.
    Cada botón necesita DOS eventos: el controlador ignora "DISC_N<i>" si antes
    no recibió el "HOLD_N<i>" de la misma fila (pulsación con vigencia de 8 s).
    -> "Touch Press Event" de b_d1 (mantener pulsada la fila; terminar con LF):
         printh 0A
         prints "HOLD_N1 ",0
         prints t_n1.txt,0
         printh 0A
    -> "Touch Release Event" de b_d1: print "DISC_N1"
    -> Botones b_d2 a b_d8: igual, con "HOLD_N2"/t_n2/"DISC_N2" hasta
       "HOLD_N8"/t_n8/"DISC_N8" según su número de fila.

---------------------------------------------------------------------------------
3. INDICADORES DINÁMICOS Y LUCES PILOTO (PROPIEDADES DE COLOR RGB565)
---------------------------------------------------------------------------------
  - b_estado : Componente gráfico ancho o botón de fondo en la barra inferior que
    funciona como luz de estado PTT/COS modificando su color (.bco):
    -> 33840 : Gris Oscuro (Modo Espera / IDLE)
    -> 63488 : Rojo Intenso (Transmitiendo / TX)
    -> 1024  : Verde Radiofónico (Recibiendo / RX)

=================================================================================
*/
package controllers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"amerimon/internal/config"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

const (
	idleColor = 33840
	txColor   = 63488
	rxColor   = 1024
)

// nodeNumber es una expresión regular que valida los números de nodo permitidos (1 a 10 dígitos).
var (
	nodeNumber = regexp.MustCompile("^[1-9][0-9]{0,9}$")
	releases   = []string{
		"CONN_FAV", "DISC_ALL", "DISC_N1", "DISC_N2", "DISC_N3", "DISC_N4",
		"DISC_N5", "DISC_N6", "DISC_N7", "DISC_N8",
		"SYS_ARM_RESTART", "SYS_ARM_REBOOT", "SYS_ARM_OFF", "SYS_CONFIRM", "SYS_CANCEL",
	}
	systemActions = map[string]systemAction{
		"SYS_ARM_RESTART": {args: []string{"restart", "asterisk.service"}},
		"SYS_ARM_REBOOT":  {args: []string{"reboot"}},
		"SYS_ARM_OFF":     {args: []string{"poweroff"}},
	}
	nextionSIPChannelName = regexp.MustCompile(`^PJSIP/[A-Za-z0-9_.-]{1,64}-[A-Za-z0-9_.-]{1,64}$`)
)

// systemAction representa una acción del sistema que puede ejecutarse, incluyendo los argumentos necesarios para el comando.
type systemAction struct {
	args []string
}

// nextionTexts agrupa los textos que Amerimon envía a la pantalla según el idioma
// elegido en la web. langValue alimenta va_lang.val (0 español, 1 inglés), que la
// pantalla usa solo para sus textos internos de respaldo.
type nextionTexts struct {
	langValue       string
	favorite        string
	disconnectAll   string
	hint            string
	restartAsterisk string
	rebootLinux     string
	poweroffLinux   string
	actions         string
	links           string
	cancel          string
	confirm         string
	nodePrefix      string
	offline         string
	actionFailed    string
	restarting      string
	rebooting       string
	poweringOff     string
}

// nextionLanguages define los textos de la interfaz Nextion para cada idioma soportado, incluyendo español e inglés.
var nextionLanguages = map[string]nextionTexts{
	"es": {
		langValue: "0", favorite: "Conectar el favorito", disconnectAll: "Desconectar todos",
		hint: "Toca para desconectar", restartAsterisk: "Reiniciar Asterisk",
		rebootLinux: "Reiniciar Linux", poweroffLinux: "Apagar Linux",
		actions: "Acciones", links: "Enlaces activos", cancel: "Cancelar", confirm: "Confirmar",
		nodePrefix: "Nodo: ", offline: "ASL3 sin conexión", actionFailed: "Acción del sistema fallida",
		restarting: "Reiniciando Asterisk", rebooting: "Reiniciando Linux", poweringOff: "Apagando Linux",
	},
	"en": {
		langValue: "1", favorite: "Connect the favorite", disconnectAll: "Disconnect all",
		hint: "Tap to disconnect", restartAsterisk: "Restart Asterisk",
		rebootLinux: "Reboot Linux", poweroffLinux: "Power off Linux",
		actions: "Actions", links: "Active links", cancel: "Cancel", confirm: "Confirm",
		nodePrefix: "Node: ", offline: "ASL3 offline", actionFailed: "System action failed",
		restarting: "Restarting Asterisk", rebooting: "Rebooting Linux", poweringOff: "Powering off Linux",
	},
}

// nextionLanguageTexts devuelve los textos de la interfaz Nextion correspondientes al idioma especificado, usando español como valor predeterminado si el idioma no está disponible.
func nextionLanguageTexts(language string) nextionTexts {
	if texts, ok := nextionLanguages[language]; ok {
		return texts
	}
	return nextionLanguages["es"]
}

// systemLabel devuelve la etiqueta correspondiente a un comando del sistema según el idioma de la pantalla.
func (t nextionTexts) systemLabel(command string) string {
	switch command {
	case "SYS_ARM_RESTART":
		return t.restartAsterisk
	case "SYS_ARM_REBOOT":
		return t.rebootLinux
	case "SYS_ARM_OFF":
		return t.poweroffLinux
	}
	return command
}

// systemMessage devuelve el mensaje correspondiente a un comando del sistema según el idioma de la pantalla.
func (t nextionTexts) systemMessage(command string) string {
	switch command {
	case "SYS_ARM_RESTART":
		return t.restarting
	case "SYS_ARM_REBOOT":
		return t.rebooting
	case "SYS_ARM_OFF":
		return t.poweringOff
	}
	return command
}

// executeSystem ejecuta un comando del sistema utilizando systemctl y sudo, con un tiempo de espera de 2 segundos.
func executeSystem(action systemAction) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := append([]string{"-n", "/usr/bin/systemctl", "--no-block"}, action.args...)
	output, err := exec.CommandContext(ctx, "/usr/bin/sudo", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// nextionSIPChannel representa un canal SIP activo junto con su identificador de llamada.
type nextionSIPChannel struct {
	ID       string
	CallerID string
}

// parseNextionSIPChannels analiza la salida cruda de AMI y devuelve una lista de canales SIP activos junto con sus identificadores de llamada.
// El parámetro callerIDs es un mapa que asocia los endpoints SIP con sus identificadores de llamada conocidos.
func parseNextionSIPChannels(raw string, callerIDs map[string]string) []nextionSIPChannel {
	channels := make([]nextionSIPChannel, 0)
	seen := make(map[string]struct{})
	for _, line := range config.ParseAMIOutputToLines(raw) {
		fields := strings.Split(strings.TrimSpace(line), "!")
		if len(fields) < 5 || !strings.EqualFold(strings.TrimSpace(fields[4]), "Up") {
			continue
		}
		channel := strings.TrimSpace(fields[0])
		if !nextionSIPChannelName.MatchString(channel) {
			continue
		}
		endpoint := strings.TrimPrefix(channel, "PJSIP/")
		endpoint = endpoint[:strings.LastIndex(endpoint, "-")]
		id := "SIP:" + channel
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}

		callerID := ""
		if len(fields) > 7 {
			callerID = strings.TrimSpace(fields[7])
			if callerID == "<unknown>" {
				callerID = ""
			}
		}
		if callerID == "" {
			callerID = callerIDs[endpoint]
		}
		if callerID == "" {
			callerID = endpoint
		}
		channels = append(channels, nextionSIPChannel{ID: id, CallerID: nextionDisplayText(callerID)})
	}
	return channels
}

// nodeLabels devuelve las etiquetas y etiquetas de actividad para una lista de nodos, utilizando la información de la base de datos de Asterisk y los nodos privados.
func (a *ami) nodeLabels(nodes []string) (map[string]string, map[string]string) {
	labels := make(map[string]string, len(nodes))
	activityLabels := make(map[string]string, len(nodes))
	if len(nodes) == 0 {
		return labels, activityLabels
	}
	astdb := map[string]AstDBEntry{}
	astdbPath := strings.TrimSpace(config.App.Sources.AstDBPath)
	if astdbPath == "" {
		astdbPath = "/var/www/html/supermon/astdb.txt"
	}
	if entries, err := readAstDBEntries(astdbPath); err == nil {
		astdb = entries
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("nextion: no se pudo leer %s: %v", astdbPath, err)
	}
	privateNodes, err := readAstDBEntries(privateNodesFilePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("nextion: no se pudieron leer los indicativos de nodos privados: %v", err)
	}
	for _, node := range nodes {
		echoNumber, isEchoLink := echoLinkLookupNumber(node)
		info := lookupNodeInfo(node, astdb, privateNodes)
		echoCallsign := ""
		if isEchoLink && strings.TrimSpace(info.Callsign) == "" {
			echoCallsign = a.echoLinkCallsign(node, echoNumber)
		}
		labels[node] = nextionNodeLabel(node, info, echoCallsign)
		activityLabels[node] = nextionActivityLabel(labels[node], info)
	}
	return labels, activityLabels
}

// nextionActivityLabel genera la etiqueta de actividad para un nodo, combinando la etiqueta del nodo con la descripción de la base de datos de Asterisk.
func nextionActivityLabel(label string, info AstDBEntry) string {
	description := nextionDisplayText(info.Descripcion)
	if description == "" {
		return label
	}
	return nextionDisplayText(label + " - " + description)
}

// nextionNodeLabel genera la etiqueta de un nodo para mostrar en la interfaz Nextion, utilizando el indicativo EchoLink si está disponible, el número de nodo si es numérico, o el indicativo del nodo si está definido.
func nextionNodeLabel(node string, info AstDBEntry, echoCallsign string) string {
	if _, isEchoLink := echoLinkLookupNumber(node); isEchoLink {
		callsign := strings.TrimSpace(info.Callsign)
		if callsign == "" {
			callsign = strings.TrimSpace(echoCallsign)
		}
		if callsign == "" {
			callsign = node
		}
		return nextionDisplayText(callsign)
	}
	if isNumericNode(node) {
		return nextionDisplayText(node)
	}
	if callsign := strings.TrimSpace(info.Callsign); callsign != "" {
		return nextionDisplayText(callsign)
	}
	return nextionDisplayText(node)
}

// echoLinkCallsign consulta y devuelve el indicativo EchoLink de un nodo, utilizando la caché y limitando las consultas a cada 30 segundos.
func (a *ami) echoLinkCallsign(node, lookupNumber string) string {
	a.echoLinkMu.Lock()
	if callsign, ok := a.echoLinkCallsigns[node]; ok {
		a.echoLinkMu.Unlock()
		return callsign
	}
	if lastQuery := a.echoLinkLookups[node]; time.Since(lastQuery) < 30*time.Second {
		a.echoLinkMu.Unlock()
		return ""
	}
	if a.echoLinkLookups == nil {
		a.echoLinkLookups = make(map[string]time.Time)
	}
	a.echoLinkLookups[node] = time.Now()
	a.echoLinkMu.Unlock()

	command := "echolink dbget nodename " + lookupNumber
	raw, err := config.ExecuteAMICommand(a.host, a.port, a.username, a.secret, command, 2*time.Second)
	if err != nil {
		log.Printf("nextion: no se pudo consultar el indicativo EchoLink %s: %v", node, err)
		return ""
	}
	entry, found := parseEchoLinkDBResponse(node, lookupNumber, raw)
	if !found || strings.TrimSpace(entry.Callsign) == "" {
		return ""
	}
	a.echoLinkMu.Lock()
	if a.echoLinkCallsigns == nil {
		a.echoLinkCallsigns = make(map[string]string)
	}
	a.echoLinkCallsigns[node] = entry.Callsign
	a.echoLinkMu.Unlock()
	return entry.Callsign
}

// pjsipCallerIDs lee y devuelve un mapa de identificadores de llamada (caller ID) de los endpoints PJSIP, utilizando la configuración de AMI y el archivo pjsip.conf.
func (a *ami) pjsipCallerIDs() map[string]string {
	path := strings.TrimSpace(config.CurrentAMI().PJSIPConfigPath)
	if path == "" {
		path = "/etc/asterisk/pjsip.conf"
	}
	callerIDs, err := readPJSIPEndpointCallerIDs(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("nextion: no se pudieron leer los caller ID de PJSIP: %v", err)
	}
	return callerIDs
}

// nextionDisplayText formatea un valor de texto para mostrarlo en la pantalla Nextion, eliminando caracteres no válidos y reemplazando caracteres fuera del rango con '?'.
func nextionDisplayText(value string) string {
	var result strings.Builder
	for _, char := range strings.TrimSpace(value) {
		switch {
		case char == '"' || char < 0x20 || char == 0x7f || char == 0xff:
			continue
		case char > 0xff:
			result.WriteByte('?')
		default:
			result.WriteRune(char)
		}
	}
	return result.String()
}

// radioStatus representa el estado del radio, incluyendo la etiqueta de estado, color, nodos conectados y etiquetas de actividad de los nodos.
type radioStatus struct {
	label  string
	color  int
	nodes  []string
	labels map[string]string
}

// radio define la interfaz que debe implementar cualquier tipo de radio, incluyendo métodos para obtener el estado, enlazar nodos y cerrar la conexión.
type radio interface {
	status() (radioStatus, error)
	link(method, node string) error
	close()
}

// ami implementa la interfaz radio para interactuar con Asterisk mediante AMI.
type ami struct {
	host, port, username, secret, node string
	echoLinkMu                         sync.Mutex
	echoLinkCallsigns                  map[string]string
	echoLinkLookups                    map[string]time.Time
}

// close cierra la conexión con el radio, liberando cualquier recurso asociado.
func (a *ami) close() {}

// status obtiene el estado del radio, incluyendo el estado de transmisión, recepción y enlaces conectados, así como las etiquetas de actividad de los nodos.
func (a *ami) status() (radioStatus, error) {
	raw, err := config.ExecuteAMIRptStatus(a.host, a.port, a.username, a.secret, a.node, "NodeStat", 2*time.Second)
	if err != nil {
		return radioStatus{}, err
	}
	values := make(map[string]string)
	for _, line := range config.ParseAMIOutputToLines(raw) {
		if key, value, ok := strings.Cut(line, ":"); ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	tx, rx := values["TransmitterKeyed"], values["SignalOnInput"]
	links, haveLinks := values["NodesCurrentlyConnectedToUs"]
	if (tx != "YES" && tx != "NO") || (rx != "YES" && rx != "NO") || !haveLinks {
		return radioStatus{}, errors.New("NodeStat no devolvió PTT, COS y enlaces directos")
	}
	result := radioStatus{color: idleColor}
	sawStat, err := config.ExecuteAMIRptStatus(a.host, a.port, a.username, a.secret, a.node, "SawStat", 2*time.Second)
	if err != nil {
		return radioStatus{}, fmt.Errorf("recepción de enlaces Nextion: %w", err)
	}
	receivingLinks, connectedLinks, err := parseRPTReceiveState(sawStat, a.node, time.Now().UTC())
	if err != nil {
		return radioStatus{}, fmt.Errorf("recepción de enlaces Nextion: %w", err)
	}
	receiving := rx == "YES" || len(receivingLinks) > 0
	for _, item := range strings.Split(links, ",") {
		node := strings.TrimSpace(item)
		if (nodeNumber.MatchString(node) || (!isNumericNode(node) && isValidRemoteNodeID(node))) &&
			!hasNode(result.nodes, node) {
			result.nodes = append(result.nodes, node)
		}
	}
	for _, node := range connectedLinks {
		if !hasNode(result.nodes, node) {
			result.nodes = append(result.nodes, node)
		}
	}
	sort.Slice(result.nodes, func(i, j int) bool {
		a, b := result.nodes[i], result.nodes[j]
		if len(a) == len(b) {
			return a < b
		}
		return len(a) < len(b)
	})
	labelNodes := append([]string(nil), result.nodes...)
	if (rx == "YES" || tx == "YES") && a.node != "" && !hasNode(labelNodes, a.node) {
		labelNodes = append(labelNodes, a.node)
	}
	var activityLabels map[string]string
	result.labels, activityLabels = a.nodeLabels(labelNodes)
	sources := make([]string, 0)
	if rx == "YES" && a.node != "" {
		sources = append(sources, activityLabels[a.node])
	}
	for _, link := range receivingLinks {
		source := activityLabels[link.RemoteNode]
		if source != "" && !hasNode(sources, source) {
			sources = append(sources, source)
		}
	}
	channels, err := config.ExecuteAMICommand(a.host, a.port, a.username, a.secret,
		"core show channels concise", 2*time.Second)
	if err != nil {
		log.Printf("nextion: no se pudieron consultar canales SIP activos: %v", err)
	} else {
		callerIDs := a.pjsipCallerIDs()
		for _, channel := range parseNextionSIPChannels(channels, callerIDs) {
			if hasNode(result.nodes, channel.ID) {
				continue
			}
			result.nodes = append(result.nodes, channel.ID)
			result.labels[channel.ID] = "SIP " + channel.CallerID
		}
		server := sipActivityServer{a.host, a.port, a.username, a.secret}
		sipReceiveActivity.RLock()
		for channel, talking := range sipReceiveActivity.byServer[server] {
			if talking.node != a.node {
				continue
			}
			label := result.labels["SIP:"+channel]
			if label == "" {
				continue
			}
			source := strings.TrimPrefix(label, "SIP ")
			receiving = true
			if !hasNode(sources, source) {
				sources = append(sources, source)
			}
		}
		sipReceiveActivity.RUnlock()
	}
	switch {
	case receiving:
		sort.Strings(sources)
		result.label, result.color = nextionDisplayText(strings.Join(sources, ", ")), rxColor
	case tx == "YES":
		result.label, result.color = activityLabels[a.node], txColor
	}
	return result, nil
}

// link envía un comando de enlace al nodo, ya sea para agregar, eliminar o limpiar enlaces, o para colgar un canal SIP específico.
func (a *ami) link(method, node string) error {
	if method == "1" && strings.HasPrefix(node, "SIP:") {
		channel := strings.TrimPrefix(node, "SIP:")
		if !nextionSIPChannelName.MatchString(channel) {
			return fmt.Errorf("canal SIP Nextion inválido")
		}
		_, err := config.ExecuteAMICommand(a.host, a.port, a.username, a.secret,
			"channel request hangup "+channel, 5*time.Second)
		return err
	}
	command := fmt.Sprintf("rpt cmd %s ilink %s", a.node, method)
	if node != "" {
		command += " " + node
	}
	_, err := config.ExecuteAMICommand(a.host, a.port, a.username, a.secret, command, 5*time.Second)
	return err
}

// demo es una implementación de radio de demostración que simula el estado de transmisión y recepción, así como los enlaces conectados.
type demo struct {
	start time.Time
	nodes []string
}

// demo es una implementación de radio de demostración que simula el estado de transmisión y recepción, así como los enlaces conectados.
func (d *demo) status() (radioStatus, error) {
	result := radioStatus{color: idleColor, nodes: d.nodes}
	switch int(time.Since(d.start)/time.Second/4) % 4 {
	case 1:
		result.color = rxColor
	case 2:
		result.color = txColor
	}
	if result.color != idleColor && len(d.nodes) > 0 {
		result.label = d.nodes[0]
	}
	return result, nil
}

// link simula la gestión de enlaces en la radio de demostración, permitiendo agregar, eliminar o limpiar enlaces según el método especificado.
func (d *demo) link(method, node string) error {
	switch method {
	case "3":
		if !hasNode(d.nodes, node) {
			d.nodes = append(d.nodes, node)
		}
	case "1":
		for i, item := range d.nodes {
			if item == node {
				d.nodes = append(d.nodes[:i], d.nodes[i+1:]...)
				break
			}
		}
	case "6":
		d.nodes = nil
	}
	return nil
}

// close cierra la radio de demostración, liberando cualquier recurso asociado.
func (d *demo) close() {}

// hasNode verifica si un nodo específico está presente en la lista de nodos proporcionada.
func hasNode(nodes []string, node string) bool {
	for _, item := range nodes {
		if item == node {
			return true
		}
	}
	return false
}

// press representa un evento de presión de un botón en el panel Nextion, incluyendo el nodo asociado y el momento en que se presionó.
type press struct {
	node string
	at   time.Time
}

// panel representa el estado del panel Nextion, incluyendo la radio conectada, nodos locales y favoritos, información del sistema y estado de los slots.
type panel struct {
	radio                    radio
	local, favorite, network string
	language                 string
	sendLabels               bool
	ip, temp                 string
	volt, cpu, memory, disk  string
	status                   radioStatus
	online                   bool
	lastError                string
	slots                    [8]string
	slotLabels               [8]string
	held                     [8]press
	sent                     map[string]string
	incoming                 []byte
	executeSystem            func(systemAction) error
	pendingSystem            string
	pendingUntil             time.Time
	systemMessage            string
	messageUntil             time.Time
}

// refresh actualiza el estado del panel, incluyendo la información de los nodos conectados y las etiquetas de los slots.
func (p *panel) refresh() {
	current, err := p.radio.status()
	p.online = err == nil
	if err != nil {
		if p.lastError != err.Error() {
			log.Printf("ASL3: %v", err)
		}
		p.lastError = err.Error()
		current = radioStatus{label: nextionLanguageTexts(p.language).offline, color: idleColor}
	} else {
		p.lastError = ""
	}
	p.status = current
	for i, node := range p.slots {
		if !hasNode(current.nodes, node) {
			p.slots[i] = ""
			p.slotLabels[i] = ""
		}
	}
	for _, node := range current.nodes {
		if hasNode(p.slots[:], node) {
			continue
		}
		for i := range p.slots {
			if p.slots[i] == "" {
				p.slots[i] = node
				p.slotLabels[i] = current.labels[node]
				if p.slotLabels[i] == "" {
					p.slotLabels[i] = node
				}
				break
			}
		}
	}
}

// handle procesa un comando recibido por el panel, gestionando acciones del sistema, conexiones y desconexiones de nodos, y la lógica de los botones de retención (hold).
func (p *panel) handle(command, node string) {
	if _, ok := systemActions[command]; ok {
		p.pendingSystem = command
		p.pendingUntil = time.Now().Add(10 * time.Second)
		return
	}
	var method, target string
	switch command {
	case "PANEL_READY":
		clear(p.sent)
		p.held = [8]press{}
		p.pendingSystem = ""
		return
	case "SYS_CANCEL":
		p.pendingSystem = ""
		return
	case "SYS_CONFIRM":
		p.confirmSystem()
		return
	case "CONN_FAV":
		method, target = "3", p.favorite
	case "DISC_ALL":
		method = "6"
	default:
		i := int(command[len(command)-1] - '1') // El decodificador admite solo los sufijos 1–4.
		if strings.HasPrefix(command, "HOLD_") {
			p.held[i] = press{}
			if slot := p.slots[i]; slot != "" {
				p.held[i] = press{slot, time.Now()}
			}
			return
		}
		held := p.held[i]
		p.held[i] = press{}
		if held.node == "" || time.Since(held.at) > 8*time.Second {
			return
		}
		method, target = "1", held.node
	}

	p.refresh()
	if !p.online || (method == "1" && !hasNode(p.status.nodes, target)) ||
		(method == "3" && hasNode(p.status.nodes, target)) {
		return
	}
	if err := p.radio.link(method, target); err != nil {
		log.Printf("%s: %v", command, err)
	} else {
		log.Printf("ASL3 recibió %s %s", command, target)
	}
}

// confirmSystem confirma y ejecuta la acción del sistema pendiente, mostrando mensajes de estado y manejando errores.
func (p *panel) confirmSystem() {
	command := p.pendingSystem
	p.pendingSystem = ""
	action, ok := systemActions[command]
	if !ok || time.Now().After(p.pendingUntil) {
		return
	}
	texts := nextionLanguageTexts(p.language)
	if p.executeSystem == nil {
		p.systemMessage = "Demo: " + texts.systemLabel(command)
		log.Print(p.systemMessage)
	} else if err := p.executeSystem(action); err != nil {
		p.systemMessage = texts.actionFailed
		log.Printf("%s: %v", texts.systemLabel(command), err)
	} else {
		p.systemMessage = texts.systemMessage(command)
		if command == "SYS_ARM_RESTART" {
			p.radio.close()
		}
		log.Print(p.systemMessage)
	}
	p.messageUntil = time.Now().Add(4 * time.Second)
}

// Los Touch Release son cadenas ASCII sin separador. HOLD se delimita con LF
// y sendme se reconoce por la respuesta binaria estándar de Nextion.
func (p *panel) read(data []byte) {
	p.incoming = append(p.incoming, data...)
stream:
	for len(p.incoming) != 0 {
		head := p.incoming
		if head[0] == 0x66 {
			if len(head) < 5 {
				return
			}
			if bytes.Equal(head[:5], []byte{0x66, 0, 0xff, 0xff, 0xff}) {
				p.incoming = head[5:]
				p.handle("PANEL_READY", "")
				continue
			}
		}
		if bytes.HasPrefix(head, []byte("HOLD_N")) {
			if end := bytes.IndexByte(head, '\n'); end >= 0 && end <= 128 {
				fields := strings.Fields(string(head[:end]))
				p.incoming = head[end+1:]
				if len(fields) >= 1 && len(fields[0]) == 7 && fields[0][6] >= '1' && fields[0][6] <= '8' {
					p.handle(fields[0], strings.Join(fields[1:], " "))
				}
				continue
			}
			if len(head) <= 128 {
				return
			}
		}
		for _, token := range releases {
			if bytes.HasPrefix(head, []byte(token)) {
				p.incoming = head[len(token):]
				p.handle(token, "")
				continue stream
			}
			if bytes.HasPrefix([]byte(token), head) {
				return
			}
		}
		if bytes.HasPrefix([]byte("HOLD_N"), head) {
			return
		}
		p.incoming = head[1:]
	}
}

// nextion envía un comando a la pantalla Nextion, codificando los caracteres en ISO-8859-1 y agregando el terminador estándar de Nextion (0xff 0xff 0xff).
func nextion(w io.Writer, instruction string) error {
	// El proyecto usa ISO-8859-1, incluso para el signo de grados.
	var frame []byte
	for _, c := range instruction {
		if c > 255 {
			return fmt.Errorf("carácter fuera de ISO-8859-1: %q", c)
		}
		frame = append(frame, byte(c))
	}
	_, err := w.Write(append(frame, 0xff, 0xff, 0xff))
	return err
}

// render genera y envía los comandos necesarios para actualizar la pantalla Nextion con el estado actual del panel, incluyendo la información de los nodos, la fecha, la hora, la temperatura y otros valores del sistema.
func (p *panel) render(w io.Writer) error {
	now := time.Now()
	texts := nextionLanguageTexts(p.language)
	heading := texts.links
	if extra := len(p.status.nodes) - len(p.slots); extra > 0 {
		heading += fmt.Sprintf("  +%d", extra)
	}
	color := strconv.Itoa(p.status.color)
	label := p.status.label
	if time.Now().Before(p.messageUntil) {
		label = p.systemMessage
	}
	// Con las etiquetas desactivadas solo se envían los valores, para respetar
	// los textos de un diseño HMI propio.
	var values [][2]string
	if p.sendLabels {
		values = append(values, [][2]string{
			{"va_lang.val", "=" + texts.langValue},
			{"t_actions.txt", "=\"" + texts.actions + "\""},
			{"b_fav.txt", "=\"" + texts.favorite + "\""},
			{"b_disc_all.txt", "=\"" + texts.disconnectAll + "\""},
			{"t_hint.txt", "=\"" + texts.hint + "\""},
			{"b_restart.txt", "=\"" + texts.restartAsterisk + "\""},
			{"b_reboot.txt", "=\"" + texts.rebootLinux + "\""},
			{"b_poweroff.txt", "=\"" + texts.poweroffLinux + "\""},
			{"b_admin_no.txt", "=\"" + texts.cancel + "\""},
			{"b_admin_ok.txt", "=\"" + texts.confirm + "\""},
			{"t_links.txt", "=\"" + heading + "\""},
		}...)
	}
	values = append(values, [][2]string{
		{"t_nodo.txt", "=\"" + texts.nodePrefix + p.local + "\""},
		{"t_ip.txt", "=\"IP: " + p.ip + "\""},
		{"t_date.txt", "=\"" + now.Format("2006-01-02") + "\""},
		{"t_time.txt", "=\"" + now.Format("15:04:05") + "\""},
		{"t_temp.txt", "=\"Temp: " + p.temp + "\""},
		{"t_volt.txt", "=\"Volt: " + displayMetric(p.volt) + "\""},
		{"t_cpu.txt", "=\"CPU: " + displayMetric(p.cpu) + "\""},
		{"t_mem.txt", "=\"Mem: " + displayMetric(p.memory) + "\""},
		{"t_dsk.txt", "=\"Disk: " + displayMetric(p.disk) + "\""},
		{"b_estado.bco", "=" + color}, {"t0.bco", "=" + color},
		{"t0.txt", "=\"" + label + "\""},
	}...)
	for i, node := range p.slots {
		visible := "1"
		if node == "" {
			node, visible = "--", "0"
		} else if p.slotLabels[i] != "" {
			node = p.slotLabels[i]
		}
		text := fmt.Sprintf("t_n%d.txt", i+1)
		button := fmt.Sprintf("b_d%d", i+1)
		values = append(values, [2]string{text, "=\"" + node + "\""},
			[2]string{"vis " + button, "," + visible}, [2]string{"tsw " + button, "," + visible})
	}
	for _, value := range values {
		if p.sent[value[0]] == value[1] {
			continue
		}
		if err := nextion(w, value[0]+value[1]); err != nil {
			return err
		}
		p.sent[value[0]] = value[1]
	}
	return nil
}

// cpuTemp obtiene la temperatura de la CPU leyendo el archivo del sistema correspondiente y formateando el valor en grados Celsius.
func cpuTemp() string {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err == nil {
		if raw, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			return fmt.Sprintf("%.1f °C", float64(raw)/1000)
		}
	}
	return "--"
}

// displayMetric formatea un valor de métrica del sistema, devolviendo "--" si el valor está vacío o solo contiene espacios.
func displayMetric(value string) string {
	if strings.TrimSpace(value) == "" {
		return "--"
	}
	return value
}

// nextionSystemMetrics obtiene las métricas del sistema, incluyendo el voltaje, el uso de CPU, memoria y disco, formateando los valores para su visualización.
func nextionSystemMetrics() (volt, cpuUsage, memoryUsage, diskUsage string) {
	volt = displayMetric(readSystemVoltage())
	cpuUsage, memoryUsage, diskUsage = "--", "--", "--"
	if usage, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(usage) > 0 {
		cpuUsage = fmt.Sprintf("%.1f%%", usage[0])
	}
	if usage, err := mem.VirtualMemory(); err == nil {
		memoryUsage = fmt.Sprintf("%.1f%%", usage.UsedPercent)
	}
	if usage, err := disk.Usage("/"); err == nil {
		diskUsage = fmt.Sprintf("%.1f%%", usage.UsedPercent)
	}
	return
}

// ipAddress obtiene la dirección IP de la interfaz de red especificada o de la primera interfaz disponible si no se especifica ninguna.
func ipAddress(name string) string {
	interfaces, _ := net.Interfaces()
	fallback := "--"
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || (name != "" && iface.Name != name) {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, _, _ := net.ParseCIDR(address.String())
			if ip.To4() == nil || !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if name != "" || strings.HasPrefix(iface.Name, "e") || strings.HasPrefix(iface.Name, "wl") {
				return ip.String()
			}
			if fallback == "--" {
				fallback = ip.String()
			}
		}
	}
	return fallback
}

// ttyIoctl realiza una llamada al sistema ioctl en el descriptor de archivo especificado con la solicitud y el argumento proporcionados.
func ttyIoctl(fd, request uintptr, arg unsafe.Pointer) error {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(arg))
	if err != 0 {
		return err
	}
	return nil
}

// openSerial abre un puerto serie en la ruta especificada con la velocidad de baudios proporcionada, configurando los parámetros del terminal y manejando errores de apertura y configuración.
func openSerial(path string, baudRate int) (*os.File, error) {
	port, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	fd := port.Fd()
	configure := func() error {
		var term syscall.Termios
		if err := ttyIoctl(fd, syscall.TIOCEXCL, nil); err != nil {
			return err
		}
		if err := ttyIoctl(fd, syscall.TCGETS, unsafe.Pointer(&term)); err != nil {
			return err
		}
		term.Iflag, term.Oflag, term.Lflag = 0, 0, 0
		speed, ok := nextionBaudRates[baudRate]
		if !ok {
			return fmt.Errorf("velocidad serie no compatible: %d", baudRate)
		}
		term.Cflag = speed | syscall.CS8 | syscall.CREAD | syscall.CLOCAL
		term.Cc[syscall.VMIN], term.Cc[syscall.VTIME] = 0, 1
		if err := ttyIoctl(fd, syscall.TCSETS, unsafe.Pointer(&term)); err != nil {
			return err
		}
		return syscall.SetNonblock(int(fd), false)
	}
	if err := configure(); err != nil {
		port.Close()
		return nil, err
	}
	return port, nil
}

// run inicia el bucle principal del panel, gestionando la actualización de la interfaz, la lectura de datos del puerto serie y el envío de latidos al Nextion.
func (p *panel) run(port *os.File) error {
	p.sent, p.incoming, p.held = make(map[string]string), nil, [8]press{}
	p.pendingSystem = ""
	var nextRadio, nextSystem, nextBeat time.Time
	buffer := make([]byte, 256)
	for {
		now := time.Now()
		if !now.Before(nextRadio) {
			p.refresh()
			delay := time.Second
			if !p.online {
				delay = 2 * time.Second
			}
			nextRadio = time.Now().Add(delay)
		}
		if !now.Before(nextSystem) {
			p.ip, p.temp = ipAddress(p.network), cpuTemp()
			p.volt, p.cpu, p.memory, p.disk = nextionSystemMetrics()
			nextSystem = now.Add(5 * time.Second)
		}
		if !now.Before(nextBeat) {
			if err := nextion(port, "va_alive.val=4"); err != nil {
				return err
			}
			nextBeat = now.Add(time.Second)
		}
		if err := p.render(port); err != nil {
			return err
		}
		n, err := port.Read(buffer)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		p.read(buffer[:n])
	}
}
