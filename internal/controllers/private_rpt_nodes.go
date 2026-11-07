package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

var (
	privateRPTNodeID     = regexp.MustCompile(`^[0-9]{4}$`)
	rptNodeSectionID     = regexp.MustCompile(`^[0-9]{4,7}$`)
	privateRPTKey        = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	privateRPTChannel    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*/[A-Za-z0-9_.:-]+$`)
	privateUSRPPorts     = regexp.MustCompile(`(?i)^USRP/[^:]+:([0-9]+):([0-9]+)$`)
	privateRPTServerFlag = regexp.MustCompile(`^\s*;\s*amerimon_show_as_server\s*=\s*(yes|no)\s*$`)
)

type privateRPTNode struct {
	ID           string            `json:"id"`
	Rxchannel    string            `json:"rxchannel"`
	Settings     map[string]string `json:"settings"`
	ShowAsServer bool              `json:"show_as_server"`
}

type privateRPTNodesRequest struct {
	Nodes []privateRPTNode `json:"nodes"`
}

type privateRPTNodeControl struct {
	path      string
	pjsipPath string
	ami       func(string) (string, error)
}

func privateRPTConfigPath() string {
	path := strings.TrimSpace(config.CurrentAMI().RPTConfigPath)
	if path == "" {
		return "/etc/asterisk/rpt.conf"
	}
	return path
}

func newPrivateRPTNodeControl() privateRPTNodeControl {
	primary := newPrimaryNodeControl()
	return privateRPTNodeControl{path: privateRPTConfigPath(), pjsipPath: newSIPUsersControl().pjsipPath, ami: primary.ami}
}

func GetPrivateRPTNodes(c echo.Context) error {
	return newPrivateRPTNodeControl().get(c)
}

func (p privateRPTNodeControl) get(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	content, err := os.ReadFile(p.path)
	if err != nil {
		logPrivateRPTError("no se pudo leer rpt.conf", err)
		return apiError(http.StatusInternalServerError, "No se pudo leer rpt.conf")
	}
	nodes, err := parsePrivateRPTNodes(string(content))
	if err != nil {
		logPrivateRPTError("no se pudo interpretar rpt.conf", err)
		return apiError(http.StatusInternalServerError, "No se pudo interpretar rpt.conf")
	}
	templateSettings, templateDescriptions := parsePrivateRPTTemplateSettings(string(content))
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, echo.Map{
		"status": "ok", "path": p.path, "nodes": nodes,
		"template_settings": templateSettings, "template_descriptions": templateDescriptions,
	})
}

func parsePrivateRPTTemplateSettings(content string) (map[string]string, map[string]string) {
	settings := make(map[string]string)
	descriptions := make(map[string]string)
	var comments []string
	section := ""
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if header := reAllmonHeader.FindStringSubmatch(line); header != nil {
			section = strings.TrimSpace(header[1])
			comments = nil
			continue
		}
		if section != "node-main" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ";") {
			comment := strings.TrimSpace(strings.TrimLeft(trimmed, ";"))
			if nodeAssignment.MatchString(comment) {
				comments = nil
			} else if comment != "" {
				comments = append(comments, comment)
			}
			continue
		}
		match := nodeAssignment.FindStringSubmatch(line)
		if match == nil {
			comments = nil
			continue
		}
		key := strings.ToLower(match[2])
		settings[key] = strings.TrimSpace(match[4])
		if inline := strings.TrimSpace(strings.TrimLeft(match[5], ";")); inline != "" {
			comments = append(comments, inline)
		}
		descriptions[key] = strings.Join(comments, "\n")
		comments = nil
	}
	return settings, descriptions
}

func SavePrivateRPTNodes(c echo.Context) error {
	return newPrivateRPTNodeControl().save(c)
}

func (p privateRPTNodeControl) save(c echo.Context) error {
	var request privateRPTNodesRequest
	if err := c.Bind(&request); err != nil || request.Nodes == nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida: falta la lista de nodos")
	}
	if len(request.Nodes) > 1000 {
		return apiError(http.StatusBadRequest, "Demasiados nodos")
	}
	if !systemControlLock.TryLock() {
		return apiError(http.StatusConflict, "Hay otra operación de configuración o mantenimiento en curso")
	}
	defer systemControlLock.Unlock()

	original, err := os.ReadFile(p.path)
	if err != nil {
		logPrivateRPTError("no se pudo leer rpt.conf", err)
		return apiError(http.StatusInternalServerError, "No se pudo leer rpt.conf")
	}
	updated, err := rewritePrivateRPTNodes(string(original), request.Nodes)
	if err != nil {
		return apiError(http.StatusBadRequest, err.Error())
	}
	if len(request.Nodes) > 0 {
		pjsipPath := p.pjsipPath
		if pjsipPath == "" {
			pjsipPath = filepath.Join(filepath.Dir(p.path), "pjsip.conf")
		}
		ids, err := readPJSIPSectionIDs(pjsipPath)
		if err != nil {
			logPrivateRPTError("no se pudieron comprobar las extensiones PJSIP", err)
			return apiError(http.StatusInternalServerError, "NODO_RPT_PJSIP_VERIFICACION_ERROR")
		}
		for _, node := range request.Nodes {
			if ids[strings.TrimSpace(node.ID)] {
				return apiError(http.StatusConflict, "NODO_RPT_PJSIP_NUMERO_DUPLICADO")
			}
		}
	}
	if updated != string(original) {
		if err := writePrimaryBackup(p.path, string(original)); err != nil {
			logPrivateRPTError("no se pudo crear el respaldo de rpt.conf", err)
			return apiError(http.StatusInternalServerError, "No se pudo crear el respaldo de rpt.conf")
		}
		current, err := os.ReadFile(p.path)
		if err != nil || string(current) != string(original) {
			return apiError(http.StatusConflict, "rpt.conf cambió durante la edición. Recarga la configuración antes de guardar")
		}
		if err := writePrimaryFile(p.path, updated); err != nil {
			logPrivateRPTError("no se pudo guardar rpt.conf", err)
			return apiError(http.StatusInternalServerError, "No se pudo guardar rpt.conf")
		}
	}
	output, err := p.ami("module reload app_rpt.so")
	if err != nil {
		logPrivateRPTError("la configuración se guardó, pero no se pudo recargar app_rpt.so", err)
		return apiError(http.StatusBadGateway, "La configuración se guardó, pero Asterisk no confirmó la recarga de app_rpt.so. Revisa AMI y vuelve a guardar para reintentar")
	}
	lowerOutput := strings.ToLower(output)
	if strings.Contains(lowerOutput, "no such command") || strings.Contains(lowerOutput, "no such module") ||
		strings.Contains(lowerOutput, "unable to") || strings.Contains(lowerOutput, "failed") ||
		!strings.Contains(lowerOutput, "reloaded successfully") {
		return apiError(http.StatusBadGateway, "La configuración se guardó, pero Asterisk rechazó la recarga de app_rpt.so. Revisa los registros y vuelve a guardar para reintentar")
	}
	status, err := p.ami("module show like app_rpt.so")
	if err != nil {
		logPrivateRPTError("la recarga se solicitó, pero no se pudo verificar el estado de app_rpt.so", err)
		return apiError(http.StatusBadGateway, "La recarga se solicitó, pero no se pudo verificar que app_rpt.so siga cargado. Revisa Asterisk antes de continuar")
	}
	if !privateRPTModuleLoaded(status) {
		return apiError(http.StatusBadGateway, "La recarga se solicitó, pero app_rpt.so no aparece cargado en Asterisk. Revisa los registros antes de continuar")
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "path": p.path, "reloaded": true, "module_loaded": true})
}

func privateRPTModuleLoaded(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "Output: "))
		if len(fields) > 0 && fields[0] == "app_rpt.so" {
			return true
		}
	}
	return false
}

func logPrivateRPTError(message string, err error) {
	// El contenido de rpt.conf puede incluir configuración sensible.
	log.Printf("private rpt nodes: %s (%T)", message, err)
}

func parsePrivateRPTNodes(content string) ([]privateRPTNode, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	sections := make(map[string][]string)
	nodeIDs := make(map[string]bool)
	section := ""
	for _, line := range lines {
		if header := reAllmonHeader.FindStringSubmatch(line); header != nil {
			section = strings.TrimSpace(header[1])
			if privateRPTNodeID.MatchString(section) {
				if _, exists := sections[section]; exists {
					return nil, fmt.Errorf("sección duplicada [%s]", section)
				}
				sections[section] = []string{}
			}
			continue
		}
		if section == "nodes" {
			match := nodeAssignment.FindStringSubmatch(line)
			if match != nil && privateRPTNodeID.MatchString(match[2]) {
				nodeIDs[match[2]] = true
			}
		} else if privateRPTNodeID.MatchString(section) {
			sections[section] = append(sections[section], line)
		}
	}
	ids := make([]string, 0, len(nodeIDs)+len(sections))
	seen := make(map[string]bool)
	for id := range nodeIDs {
		ids = append(ids, id)
		seen[id] = true
	}
	for id := range sections {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	nodes := make([]privateRPTNode, 0, len(ids))
	for _, id := range ids {
		settings := make(map[string]string)
		for _, line := range sections[id] {
			match := nodeAssignment.FindStringSubmatch(line)
			if match == nil {
				continue
			}
			key := strings.ToLower(match[2])
			value := strings.TrimSpace(strings.SplitN(match[4], ";", 2)[0])
			settings[key] = value
		}
		rxchannel := settings["rxchannel"]
		delete(settings, "rxchannel")
		showAsServer := false
		for _, line := range sections[id] {
			if flag := privateRPTServerFlag.FindStringSubmatch(line); flag != nil {
				showAsServer = flag[1] == "yes"
			}
		}
		nodes = append(nodes, privateRPTNode{ID: id, Rxchannel: rxchannel, Settings: settings, ShowAsServer: showAsServer})
	}
	return nodes, nil
}

func rewritePrivateRPTNodes(content string, requested []privateRPTNode) (string, error) {
	desired := make(map[string]privateRPTNode, len(requested))
	rxchannels := make(map[string]string, len(requested))
	usrpPorts := make(map[string]string)
	existingRxchannels := rptNodeRxchannels(content)
	for _, node := range requested {
		node.ID = strings.TrimSpace(node.ID)
		node.Rxchannel = strings.TrimSpace(node.Rxchannel)
		if !privateRPTNodeID.MatchString(node.ID) {
			return "", errors.New("el número de cada nodo privado debe tener exactamente 4 dígitos")
		}
		if len(node.Rxchannel) > 256 || !privateRPTChannel.MatchString(node.Rxchannel) {
			return "", fmt.Errorf("rxchannel inválido para el nodo %s", node.ID)
		}
		rxKey := strings.ToLower(node.Rxchannel)
		if other, exists := rxchannels[rxKey]; exists {
			return "", fmt.Errorf("los nodos %s y %s no pueden tener el mismo rxchannel", other, node.ID)
		}
		rxchannels[rxKey] = node.ID
		if match := privateUSRPPorts.FindStringSubmatch(node.Rxchannel); match != nil {
			ports := match[1] + ":" + match[2]
			if other, exists := usrpPorts[ports]; exists {
				return "", fmt.Errorf("los nodos USRP %s y %s deben usar puertos distintos", other, node.ID)
			}
			usrpPorts[ports] = node.ID
		}
		if _, exists := desired[node.ID]; exists {
			return "", fmt.Errorf("el nodo %s aparece más de una vez", node.ID)
		}
		if len(node.Settings) > 100 {
			return "", fmt.Errorf("demasiadas opciones para el nodo %s", node.ID)
		}
		settingKeys := make(map[string]bool, len(node.Settings))
		for key, value := range node.Settings {
			if !privateRPTKey.MatchString(key) || len(value) > 1024 || strings.ContainsAny(value, "\r\n\x00") {
				return "", fmt.Errorf("opción inválida %q para el nodo %s", key, node.ID)
			}
			normalizedKey := strings.ToLower(key)
			if normalizedKey == "rxchannel" {
				return "", fmt.Errorf("rxchannel debe configurarse en su campo para el nodo %s", node.ID)
			}
			if settingKeys[normalizedKey] {
				return "", fmt.Errorf("la opción %s está repetida para el nodo %s", key, node.ID)
			}
			settingKeys[normalizedKey] = true
		}
		desired[node.ID] = node
	}
	for _, node := range desired {
		for otherID, channel := range existingRxchannels {
			if privateRPTNodeID.MatchString(otherID) {
				continue
			}
			if strings.EqualFold(channel, node.Rxchannel) {
				return "", fmt.Errorf("los nodos %s y %s no pueden tener el mismo rxchannel", otherID, node.ID)
			}
			nodePorts := privateUSRPPorts.FindStringSubmatch(node.Rxchannel)
			existingPorts := privateUSRPPorts.FindStringSubmatch(channel)
			if nodePorts != nil && existingPorts != nil && nodePorts[1]+":"+nodePorts[2] == existingPorts[1]+":"+existingPorts[2] {
				return "", fmt.Errorf("los nodos USRP %s y %s deben usar puertos distintos", otherID, node.ID)
			}
		}
	}

	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	old, err := parsePrivateRPTNodes(content)
	if err != nil {
		return "", err
	}
	oldIDs := make(map[string]bool, len(old))
	for _, node := range old {
		oldIDs[node.ID] = true
	}
	for id := range desired {
		if !oldIDs[id] {
			for i, line := range lines {
				if header := reAllmonHeader.FindStringSubmatch(line); header != nil &&
					strings.TrimSpace(header[1]) == "nodes" {
					for j := i + 1; j < len(lines); j++ {
						if reAllmonHeader.FindStringSubmatch(lines[j]) != nil {
							break
						}
						match := nodeAssignment.FindStringSubmatch(lines[j])
						if match != nil && strings.TrimSpace(match[2]) == id {
							return "", fmt.Errorf("el nodo %s ya está definido en [nodes]", id)
						}
					}
				}
			}
		}
	}

	out := make([]string, 0, len(lines)+len(desired)*12)
	section := ""
	seenSections := make(map[string]bool)
	seenMappings := make(map[string]bool)
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if header := reAllmonHeader.FindStringSubmatch(line); header != nil {
			section = strings.TrimSpace(header[1])
			if privateRPTNodeID.MatchString(section) {
				node, keep := desired[section]
				if !keep {
					for i+1 < len(lines) && reAllmonHeader.FindStringSubmatch(lines[i+1]) == nil {
						i++
					}
					continue
				}
				if seenSections[section] {
					return "", fmt.Errorf("sección duplicada [%s]", section)
				}
				seenSections[section] = true
				out = append(out, line)
				body := make([]string, 0)
				lastSetting := make(map[string]int)
				for j := i + 1; j < len(lines) && reAllmonHeader.FindStringSubmatch(lines[j]) == nil; j++ {
					if privateRPTServerFlag.MatchString(lines[j]) {
						continue
					}
					body = append(body, lines[j])
					if match := nodeAssignment.FindStringSubmatch(lines[j]); match != nil {
						lastSetting[strings.ToLower(match[2])] = len(body) - 1
					}
				}
				for _, key := range sortedPrivateRPTSettingKeys(node.Settings) {
					value := node.Settings[key]
					if index, exists := lastSetting[strings.ToLower(key)]; exists {
						body[index] = setNodeAssignmentValue(body[index], value)
					} else {
						body = append(body, key+" = "+value)
					}
				}
				if index, exists := lastSetting["rxchannel"]; exists {
					body[index] = setNodeAssignmentValue(body[index], node.Rxchannel)
				} else {
					body = append(body, "rxchannel = "+node.Rxchannel)
				}
				if node.ShowAsServer {
					body = append(body, "; amerimon_show_as_server = yes")
				}
				out = append(out, body...)
				for i+1 < len(lines) && reAllmonHeader.FindStringSubmatch(lines[i+1]) == nil {
					i++
				}
				continue
			}
			out = append(out, line)
			continue
		}
		if section == "nodes" {
			match := nodeAssignment.FindStringSubmatch(line)
			if match != nil && privateRPTNodeID.MatchString(match[2]) {
				id := match[2]
				if _, keep := desired[id]; !keep {
					continue
				}
				if seenMappings[id] {
					return "", fmt.Errorf("el nodo %s aparece más de una vez en [nodes]", id)
				}
				seenMappings[id] = true
				out = append(out, line)
				continue
			}
		}
		out = append(out, line)
	}
	for _, id := range sortedPrivateRPTIDs(desired) {
		if !seenSections[id] {
			out = append(out, "")
			out = append(out, "["+id+"](node-main)")
			out = appendPrivateRPTSettings(out, desired[id])
			out = append(out, "")
		}
	}
	if len(desired) > 0 {
		nodesHeader, nodesEnd := -1, len(out)
		for i, line := range out {
			if header := reAllmonHeader.FindStringSubmatch(line); header != nil {
				if nodesHeader >= 0 {
					nodesEnd = i
					break
				}
				if strings.TrimSpace(header[1]) == "nodes" {
					nodesHeader = i
				}
			}
		}
		if nodesHeader < 0 {
			out = append(out, "", "[nodes]")
			nodesHeader = len(out) - 1
			nodesEnd = len(out)
		}
		for i := nodesHeader + 1; i < nodesEnd; i++ {
			match := nodeAssignment.FindStringSubmatch(out[i])
			if match != nil && privateRPTNodeID.MatchString(match[2]) {
				seenMappings[match[2]] = true
			}
		}
		mappings := make([]string, 0)
		for _, id := range sortedPrivateRPTIDs(desired) {
			if seenMappings[id] {
				continue
			}
			mappings = append(mappings, id+" = radio@127.0.0.1/"+id+",NONE")
		}
		out = append(out[:nodesEnd], append(mappings, out[nodesEnd:]...)...)
	}
	return strings.ReplaceAll(strings.Join(out, "\n"), "\n", eol), nil
}

func rptNodeRxchannels(content string) map[string]string {
	channels := make(map[string]string)
	section := ""
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if header := reAllmonHeader.FindStringSubmatch(line); header != nil {
			section = strings.TrimSpace(header[1])
			continue
		}
		if !rptNodeSectionID.MatchString(section) {
			continue
		}
		match := nodeAssignment.FindStringSubmatch(line)
		if match != nil && strings.EqualFold(match[2], "rxchannel") {
			channels[section] = strings.TrimSpace(strings.SplitN(match[4], ";", 2)[0])
		}
	}
	return channels
}

func setNodeAssignmentValue(line, value string) string {
	match := nodeAssignment.FindStringSubmatch(line)
	if match == nil {
		return line
	}
	return match[1] + match[2] + match[3] + value + match[5]
}

func sortedPrivateRPTIDs(nodes map[string]privateRPTNode) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func sortedPrivateRPTSettingKeys(settings map[string]string) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendPrivateRPTSettings(lines []string, node privateRPTNode) []string {
	for _, key := range sortedPrivateRPTSettingKeys(node.Settings) {
		lines = append(lines, key+" = "+node.Settings[key])
	}
	lines = append(lines, "rxchannel = "+node.Rxchannel)
	if node.ShowAsServer {
		lines = append(lines, "; amerimon_show_as_server = yes")
	}
	return lines
}

func readPrivateRPTServerNodes(path string) (map[string]bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	nodes, err := parsePrivateRPTNodes(string(content))
	if err != nil {
		return nil, err
	}
	visible := make(map[string]bool)
	for _, node := range nodes {
		if node.ShowAsServer {
			visible[node.ID] = true
		}
	}
	return visible, nil
}
