/*
=================================================================================

	DOCUMENTACIÓN DE VARIABLES Y COMPONENTES - NEXTION EDITOR

=================================================================================
Para tu pantalla de 3.5" (NX4832K035) con resolución 480x320 en Modo Oscuro.
Asegúrate de que la propiedad 'vscope' de cada componente esté configurada en 'global'.

---------------------------------------------------------------------------------
1. COMPONENTES DE TEXTO (SALIDAS DE INFORMACIÓN)
---------------------------------------------------------------------------------
  - t_nodo   : Muestra el número de tu nodo local. (Ej: "Nodo Local: 616262")
  - t_ip     : Dirección IP de la laptop/servidor Amerimon. (Ej: "IP: 192.168.0.177")
  - t_date   : Fecha del servidor en formato YYYY-MM-DD.
  - t_time   : Hora del servidor en formato HH:mm:ss (24 horas).
  - t_temp   : Temperatura actual del procesador. (Ej: "CPU: 45.2 °C")
  - t0       : Estado operativo de AllStarLink. (Ej: "Sistema Listo", "Transmitiendo...")
  - t_n1     : Indicativo o caller ID de la primera conexión activa.
  - t_n2     : Indicativo o caller ID de la segunda conexión activa.
  - t_n3     : Indicativo o caller ID de la tercera conexión activa.
  - t_n4     : Indicativo o caller ID de la cuarta conexión activa.

---------------------------------------------------------------------------------
2. COMPONENTES DE BOTÓN / INTERFACES TÁCTILES (ENTRADAS DESDE PANTALLA)
---------------------------------------------------------------------------------
Cada botón debe enviar su identificador de texto en el evento "Touch Release Event"
dentro de Nextion Editor usando el comando: print "COMANDO"

  - b_fav       : Botón para conectar al Nodo Favorito fijo.
    -> Enviar por evento: print "CONN_FAV"

  - b_disc_all  : Botón global rojo para colgar todos los enlaces (*70).
    -> Enviar por evento: print "DISC_ALL"

  - b_d1 a b_d4 : Botones transparentes/invisibles encimados exactamente sobre sus
    respectivas cajas de texto (t_n1 a t_n4). El script en Go maneja
    su visibilidad de manera automática mediante comandos 'vis'.
    -> Botón b_d1 encima de t_n1: print "DISC_N1"
    -> Botón b_d2 encima de t_n2: print "DISC_N2"
    -> Botón b_d3 encima de t_n3: print "DISC_N3"
    -> Botón b_d4 encima de t_n4: print "DISC_N4"

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
)

const (
	idleColor = 33840
	txColor   = 63488
	rxColor   = 1024
)

var (
	nodeNumber = regexp.MustCompile("^[1-9][0-9]{0,9}$")
	releases   = []string{
		"CONN_FAV", "DISC_ALL", "DISC_N1", "DISC_N2", "DISC_N3", "DISC_N4",
		"SYS_ARM_RESTART", "SYS_ARM_REBOOT", "SYS_ARM_OFF", "SYS_CONFIRM", "SYS_CANCEL",
	}
	systemActions = map[string]systemAction{
		"SYS_ARM_RESTART": {[]string{"restart", "asterisk.service"}, "Reiniciar Asterisk", "Reiniciando Asterisk"},
		"SYS_ARM_REBOOT":  {[]string{"reboot"}, "Reiniciar Linux", "Reiniciando Linux"},
		"SYS_ARM_OFF":     {[]string{"poweroff"}, "Apagar Linux", "Apagando Linux"},
	}
	nextionSIPChannelName = regexp.MustCompile(`^PJSIP/[A-Za-z0-9_.-]{1,64}-[A-Za-z0-9_.-]{1,64}$`)
)

type systemAction struct {
	args    []string
	label   string
	message string
}

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

type nextionSIPChannel struct {
	ID       string
	CallerID string
}

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

func (a *ami) nodeLabels(nodes []string) map[string]string {
	labels := make(map[string]string, len(nodes))
	if len(nodes) == 0 {
		return labels
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
	}
	return labels
}

func nextionNodeLabel(node string, info AstDBEntry, echoCallsign string) string {
	if _, isEchoLink := echoLinkLookupNumber(node); isEchoLink {
		callsign := strings.TrimSpace(info.Callsign)
		if callsign == "" {
			callsign = strings.TrimSpace(echoCallsign)
		}
		if callsign == "" {
			callsign = node
		}
		return nextionDisplayText("EchoLink " + callsign)
	}
	if callsign := strings.TrimSpace(info.Callsign); callsign != "" {
		return nextionDisplayText("IAXRpt " + callsign)
	}
	return nextionDisplayText("IAXRpt " + node)
}

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

type radioStatus struct {
	label  string
	color  int
	nodes  []string
	labels map[string]string
}

type radio interface {
	status() (radioStatus, error)
	link(method, node string) error
	close()
}

type ami struct {
	host, port, username, secret, node string
	echoLinkMu                         sync.Mutex
	echoLinkCallsigns                  map[string]string
	echoLinkLookups                    map[string]time.Time
}

func (a *ami) close() {}

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
	result := radioStatus{label: "Sistema Listo", color: idleColor}
	switch {
	case tx == "YES":
		result.label, result.color = "Transmitiendo...", txColor
	case rx == "YES":
		result.label, result.color = "Recibiendo...", rxColor
	}
	for _, item := range strings.Split(links, ",") {
		node := strings.TrimSpace(item)
		if nodeNumber.MatchString(node) && !hasNode(result.nodes, node) {
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
	result.labels = a.nodeLabels(result.nodes)
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
	}
	return result, nil
}

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

type demo struct {
	start time.Time
	nodes []string
}

func (d *demo) status() (radioStatus, error) {
	result := radioStatus{label: "Sistema Listo", color: idleColor, nodes: d.nodes}
	switch int(time.Since(d.start)/time.Second/4) % 4 {
	case 1:
		result.label, result.color = "Recibiendo...", rxColor
	case 2:
		result.label, result.color = "Transmitiendo...", txColor
	}
	return result, nil
}

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

func (d *demo) close() {}

func hasNode(nodes []string, node string) bool {
	for _, item := range nodes {
		if item == node {
			return true
		}
	}
	return false
}

type press struct {
	node string
	at   time.Time
}

type panel struct {
	radio                    radio
	local, favorite, network string
	ip, temp                 string
	status                   radioStatus
	online                   bool
	lastError                string
	slots                    [4]string
	slotLabels               [4]string
	held                     [4]press
	sent                     map[string]string
	incoming                 []byte
	executeSystem            func(systemAction) error
	pendingSystem            string
	pendingUntil             time.Time
	systemMessage            string
	messageUntil             time.Time
}

func (p *panel) refresh() {
	current, err := p.radio.status()
	p.online = err == nil
	if err != nil {
		if p.lastError != err.Error() {
			log.Printf("ASL3: %v", err)
		}
		p.lastError = err.Error()
		current = radioStatus{label: "ASL3 sin conexión", color: idleColor}
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
		p.held = [4]press{}
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

func (p *panel) confirmSystem() {
	command := p.pendingSystem
	p.pendingSystem = ""
	action, ok := systemActions[command]
	if !ok || time.Now().After(p.pendingUntil) {
		return
	}
	if p.executeSystem == nil {
		p.systemMessage = "Demo: " + action.label
		log.Print(p.systemMessage)
	} else if err := p.executeSystem(action); err != nil {
		p.systemMessage = "Acción del sistema fallida"
		log.Printf("%s: %v", action.label, err)
	} else {
		p.systemMessage = action.message
		if command == "SYS_ARM_RESTART" {
			p.radio.close()
		}
		log.Print(action.message)
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
				if len(fields) >= 1 && len(fields[0]) == 7 && fields[0][6] >= '1' && fields[0][6] <= '4' {
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

func (p *panel) render(w io.Writer) error {
	now := time.Now()
	heading := "ENLACES ACTIVOS"
	if extra := len(p.status.nodes) - len(p.slots); extra > 0 {
		heading += fmt.Sprintf("  +%d", extra)
	}
	color := strconv.Itoa(p.status.color)
	label := p.status.label
	if time.Now().Before(p.messageUntil) {
		label = p.systemMessage
	}
	values := [][2]string{
		{"t_nodo.txt", "=\"Nodo Local: " + p.local + "\""},
		{"t_ip.txt", "=\"IP: " + p.ip + "\""},
		{"t_date.txt", "=\"" + now.Format("2006-01-02") + "\""},
		{"t_time.txt", "=\"" + now.Format("15:04:05") + "\""},
		{"t_temp.txt", "=\"CPU: " + p.temp + "\""},
		{"t_links.txt", "=\"" + heading + "\""},
		{"b_estado.bco", "=" + color}, {"t0.bco", "=" + color},
		{"t0.txt", "=\"" + label + "\""},
	}
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

func cpuTemp() string {
	data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
	if err == nil {
		if raw, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			return fmt.Sprintf("%.1f °C", float64(raw)/1000)
		}
	}
	return "--"
}

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

func ttyIoctl(fd, request uintptr, arg unsafe.Pointer) error {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(arg))
	if err != 0 {
		return err
	}
	return nil
}

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

func (p *panel) run(port *os.File) error {
	p.sent, p.incoming, p.held = make(map[string]string), nil, [4]press{}
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
