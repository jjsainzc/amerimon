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
	"syscall"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

var (
	primaryNodeNumber = regexp.MustCompile(`^[2-9][0-9]{3,5}$`)
	primaryCallsign   = regexp.MustCompile(`^[A-Z0-9/-]{3,64}$`)
	primaryPassword   = regexp.MustCompile(`^[A-Za-z0-9_-]{6,128}$`)
	nodeRegistration  = regexp.MustCompile(`^(\s*;*\s*register\s*=>?\s*)([0-9]+):([^@\s]*)@(.+)$`)
	nodeAssignment    = regexp.MustCompile(`^(\s*)([A-Za-z0-9_]+)(\s*=\s*)([^;]*)(.*)$`)
)

type primaryNodeRequest struct {
	CurrentNode string `json:"current_node"`
	Node        string `json:"node"`
	Callsign    string `json:"callsign"`
	Password    string `json:"password"`
}

type nodeAccessEntry struct {
	Type   string `json:"type"`
	Node   string `json:"node"`
	Reason string `json:"reason"`
}

type primaryNodeControl struct {
	rptPath     string
	allmonPaths []string
	ami         func(string) (string, error)
}

func newPrimaryNodeControl() primaryNodeControl {
	path := config.App.AMI.RPTConfigPath
	if path == "" {
		path = "/etc/asterisk/rpt.conf"
	}
	return primaryNodeControl{
		rptPath:     path,
		allmonPaths: []string{allmonINIPath(), "/etc/allmon3/allmon3.ini"},
		ami: func(command string) (string, error) {
			ami := config.App.AMI
			return config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, command, 5*time.Second)
		},
	}
}

func GetPrimaryNodeConfig(c echo.Context) error  { return newPrimaryNodeControl().get(c) }
func SavePrimaryNodeConfig(c echo.Context) error { return newPrimaryNodeControl().save(c) }
func GetPrimaryNodeAccess(c echo.Context) error  { return newPrimaryNodeControl().getAccess(c) }
func SavePrimaryNodeAccess(c echo.Context) error {
	return newPrimaryNodeControl().changeAccess(c, false)
}
func DeletePrimaryNodeAccess(c echo.Context) error {
	return newPrimaryNodeControl().changeAccess(c, true)
}

func ClearPrimaryNodeAccess(c echo.Context) error {
	return newPrimaryNodeControl().clearAccess(c)
}

func (p primaryNodeControl) clearAccess(c echo.Context) error {
	var req struct {
		Primary      string `json:"primary_node"`
		Type         string `json:"type"`
		Confirmation string `json:"confirmation"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	if (req.Type != "allowlist" && req.Type != "denylist") || req.Confirmation != "VACIAR "+req.Type {
		return apiError(http.StatusBadRequest, "Lista o confirmación inválida")
	}
	if !systemControlLock.TryLock() {
		return apiError(http.StatusConflict, "Hay otra operación de configuración o mantenimiento en curso")
	}
	defer systemControlLock.Unlock()
	node, err := p.primary()
	if err != nil {
		return primaryNodeError("No se pudo identificar el nodo principal", err)
	}
	if req.Primary != node {
		return apiError(http.StatusConflict, "El nodo principal cambió. Recarga la configuración")
	}
	entries, err := p.access(node)
	if err != nil {
		return primaryNodeError("No se pudo leer la lista antes de vaciarla", err)
	}
	// Borrar claves exactas evita que un prefijo de AstDB afecte a otros nodos.
	for _, entry := range entries {
		if entry.Type != req.Type {
			continue
		}
		if err := p.removeAccess(node, entry); err != nil {
			return primaryNodeError("No se pudo vaciar toda la lista; algunas entradas pueden haberse eliminado. Recarga la lista antes de reintentar", err)
		}
	}
	remaining, err := p.access(node)
	if err != nil {
		return primaryNodeError("No se pudo verificar el vaciado. Recarga la lista antes de reintentar", err)
	}
	for _, entry := range remaining {
		if entry.Type == req.Type {
			return primaryNodeError("La lista aún contiene entradas. Recárgala antes de reintentar", errors.New("lista no vacía"))
		}
	}
	log.Printf("primary node: lista vaciada nodo=%s lista=%s", node, req.Type)
	return c.JSON(http.StatusOK, echo.Map{"status": "ok"})
}

func (p primaryNodeControl) primary() (string, error) {
	node, err := readPrimaryRPTNode(p.rptPath)
	if err == nil && node == "" {
		err = errors.New("no se encontró un nodo principal inequívoco en rpt.conf")
	}
	return node, err
}

func (p primaryNodeControl) get(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	node, err := p.primary()
	if err != nil {
		return primaryNodeError("No se pudo identificar el nodo principal", err)
	}
	data, err := os.ReadFile(p.rptPath)
	if err != nil {
		return primaryNodeError("No se pudo leer rpt.conf", err)
	}
	section := ""
	callsign := ""
	for _, line := range strings.Split(string(data), "\n") {
		if header := reAllmonHeader.FindStringSubmatch(line); header != nil {
			section = strings.TrimSpace(header[1])
		}
		if m := nodeAssignment.FindStringSubmatch(line); m != nil && section == node && strings.EqualFold(m[2], "idrecording") {
			value := strings.TrimSpace(m[4])
			if strings.HasPrefix(value, "|i") {
				callsign = strings.TrimPrefix(value, "|i")
			}
		}
	}
	password := ""
	for _, name := range []string{"rpt_http_registrations.conf", "iax.conf"} {
		content, err := os.ReadFile(filepath.Join(filepath.Dir(p.rptPath), name))
		if err != nil {
			return primaryNodeError("No se pudo leer "+name, err)
		}
		registrationSection := ""
		for _, line := range strings.Split(string(content), "\n") {
			if header := reAllmonHeader.FindStringSubmatch(line); header != nil {
				registrationSection = strings.TrimSpace(header[1])
			}
			if m := nodeRegistration.FindStringSubmatch(line); m != nil && m[2] == node && m[3] != "" &&
				password == "" && ((name == "rpt_http_registrations.conf" && registrationSection == "registrations" &&
				!strings.Contains(m[1], ";")) || (name == "iax.conf" && registrationSection == "general")) {
				password = m[3]
			}
		}
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, echo.Map{
		"node": node, "callsign": callsign, "password_set": password != "", "password": password,
		"path": p.rptPath,
	})
}

func primaryNodeError(message string, err error) error {
	// No registrar contenido ni respuestas AMI: pueden incluir credenciales.
	log.Printf("primary node: %s (%T)", message, err)
	return apiError(http.StatusInternalServerError, message+". Revisa los permisos y la configuración del servidor.")
}

type primaryFileChange struct {
	path     string
	original string
	updated  string
}

func validatePrimaryNodeRequest(req primaryNodeRequest) error {
	if !primaryNodeNumber.MatchString(req.Node) {
		return errors.New("El nodo principal del monitor debe ser público: 4 a 6 dígitos sin comenzar por 0 o 1")
	}
	if !primaryCallsign.MatchString(req.Callsign) {
		return errors.New("El indicativo debe tener de 3 a 64 letras, números, / o -")
	}
	if req.Password != "" && !primaryPassword.MatchString(req.Password) {
		return errors.New("La contraseña debe tener de 6 a 128 letras, números, _ o -")
	}
	return nil
}

func (p primaryNodeControl) save(c echo.Context) error {
	var req primaryNodeRequest
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	req.Node = strings.TrimSpace(req.Node)
	req.Callsign = strings.ToUpper(strings.TrimSpace(req.Callsign))
	if err := validatePrimaryNodeRequest(req); err != nil {
		return apiError(http.StatusBadRequest, err.Error())
	}
	if !systemControlLock.TryLock() {
		return apiError(http.StatusConflict, "Hay otra operación de configuración o mantenimiento en curso")
	}
	defer systemControlLock.Unlock()
	old, err := p.primary()
	if err != nil {
		return primaryNodeError("No se pudo identificar el nodo principal", err)
	}
	if old != req.CurrentNode {
		return apiError(http.StatusConflict, "El nodo principal cambió. Recarga la configuración antes de guardar")
	}
	changes, err := p.prepare(old, req)
	if err != nil {
		return primaryNodeError("No se pudieron preparar los cambios: "+err.Error(), err)
	}
	// Preparar todos los respaldos antes de modificar ningún archivo.
	for _, change := range changes {
		if err := writePrimaryBackup(change.path, change.original); err != nil {
			return primaryNodeError("No se pudo crear el respaldo de "+filepath.Base(change.path), err)
		}
	}
	var access []nodeAccessEntry
	if old != req.Node {
		access, err = p.access(old)
		if err != nil {
			return primaryNodeError("No se pudieron leer las listas de acceso para migrarlas", err)
		}
		targetAccess, err := p.access(req.Node)
		if err != nil {
			return primaryNodeError("No se pudo comprobar la lista del nuevo número", err)
		}
		if len(targetAccess) > 0 {
			return apiError(http.StatusConflict, "El nuevo número ya tiene listas de acceso en AstDB")
		}
		for _, entry := range access {
			if err := validateNodeAccess(entry, req.Node); err != nil {
				return apiError(http.StatusConflict, "Una entrada de acceso existente no puede trasladarse al nuevo número. Corrígela antes de renombrar el nodo")
			}
		}
	}
	written := 0
	var migrated []nodeAccessEntry
	rollback := func(cause error) error {
		var failures []string
		for _, entry := range migrated {
			if err := p.putAccess(old, entry); err != nil {
				failures = append(failures, "restaurar lista original")
			}
			if err := p.removeAccess(req.Node, entry); err != nil {
				failures = append(failures, "retirar lista nueva")
			}
		}
		for i := written - 1; i >= 0; i-- {
			if err := writePrimaryFile(changes[i].path, changes[i].original); err != nil {
				failures = append(failures, filepath.Base(changes[i].path))
			}
		}
		if len(failures) > 0 {
			return primaryNodeError("Falló el guardado y la restauración de "+strings.Join(failures, ", ")+". Restaura los respaldos .amerimon.bak antes de reiniciar Asterisk", cause)
		}
		return primaryNodeError("No se pudo guardar; se restauró la configuración anterior", cause)
	}
	for _, change := range changes {
		current, err := os.ReadFile(change.path)
		if err != nil || string(current) != change.original {
			return rollback(errors.New("un archivo cambió durante el guardado"))
		}
		// Incluir el archivo en la restauración incluso si falla su sincronización.
		written++
		if err := writePrimaryFile(change.path, change.updated); err != nil {
			return rollback(err)
		}
	}
	for _, entry := range access {
		migrated = append(migrated, entry)
		if err := p.putAccess(req.Node, entry); err != nil {
			return rollback(err)
		}
		if err := p.removeAccess(old, entry); err != nil {
			return rollback(err)
		}
	}
	log.Printf("primary node: configuración guardada nodo=%s anterior=%s", req.Node, old)
	return c.JSON(http.StatusOK, echo.Map{
		"status": "ok", "node": req.Node, "restart_required": true,
		"message": "Configuración guardada con respaldo. Reinicia Asterisk manualmente desde Sistema para aplicar los cambios.",
	})
}

func (p primaryNodeControl) prepare(old string, req primaryNodeRequest) ([]primaryFileChange, error) {
	dir := filepath.Dir(p.rptPath)
	names := []string{"rpt.conf", "rpt_http_registrations.conf", "iax.conf"}
	if old != req.Node || req.Password != "" {
		names = append(names, "savenode.conf")
	}
	if old != req.Node {
		names = append(names, "extensions.conf", "simpleusb.conf", "usbradio.conf", "voter.conf")
	}
	var changes []primaryFileChange
	for _, name := range names {
		path := filepath.Join(dir, name)
		if name == "rpt.conf" {
			path = p.rptPath
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) && name != "rpt.conf" && name != "iax.conf" && name != "rpt_http_registrations.conf" {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("no se pudo leer %s", name)
		}
		updated, err := rewritePrimaryFile(name, string(data), old, req)
		if err != nil {
			return nil, err
		}
		if updated != string(data) {
			changes = append(changes, primaryFileChange{path, string(data), updated})
		}
	}
	if old != req.Node {
		seen := map[string]bool{}
		for _, path := range p.allmonPaths {
			if seen[path] {
				continue
			}
			seen[path] = true
			data, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, errors.New("no se pudo leer la configuración Allmon")
			}
			updated, err := rewritePrimaryFile("allmon.ini", string(data), old, req)
			if err != nil {
				return nil, err
			}
			if updated != string(data) {
				changes = append(changes, primaryFileChange{path, string(data), updated})
			}
		}
	}
	return changes, nil
}

// rewritePrimaryFile solo modifica secciones y referencias propias del nodo, no números en comentarios o destinos remotos.
func rewritePrimaryFile(name, content, old string, req primaryNodeRequest) (string, error) {
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	section, foundNode, foundRegistration, foundID := "", false, false, false
	for i, line := range lines {
		if h := reAllmonHeader.FindStringSubmatch(line); h != nil {
			section = strings.TrimSpace(h[1])
			if old != req.Node && section == req.Node {
				return "", fmt.Errorf("el nuevo número ya existe en %s", name)
			}
			if section == old && name != "iax.conf" && name != "rpt_http_registrations.conf" {
				start, end := strings.IndexByte(line, '['), strings.IndexByte(line, ']')
				lines[i] = line[:start+1] + req.Node + line[end:]
				foundNode = true
			}
		}
		if m := nodeRegistration.FindStringSubmatch(line); m != nil && m[2] == old &&
			((name == "iax.conf" && section == "general") || (name == "rpt_http_registrations.conf" && section == "registrations")) {
			foundRegistration = true
			if m := nodeRegistration.FindStringSubmatch(line); m != nil && old != req.Node && m[2] == req.Node {
				return "", fmt.Errorf("el nuevo número ya tiene un registro en %s", name)
			}
			password := m[3]
			if req.Password != "" {
				password = req.Password
			}
			lines[i] = m[1] + req.Node + ":" + password + "@" + m[4]
			continue
		}
		m := nodeAssignment.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, value := strings.ToLower(m[2]), strings.TrimSpace(m[4])
		if name == "rpt.conf" && section == "nodes" && old != req.Node && m[2] == req.Node {
			return "", errors.New("el nuevo número ya existe en la tabla [nodes] de rpt.conf")
		}
		switch {
		case name == "rpt.conf" && section == old && key == "idrecording":
			lines[i] = m[1] + m[2] + m[3] + "|i" + req.Callsign + m[5]
			foundID = true
		case name == "rpt.conf" && section == "nodes" && m[2] == old:
			value = regexp.MustCompile(`/`+regexp.QuoteMeta(old)+`([,/\s]|$)`).ReplaceAllString(value, "/"+req.Node+"$1")
			lines[i] = m[1] + req.Node + m[3] + value + m[5]
		case name == "rpt.conf" && section == old && key == "rxchannel":
			for _, prefix := range []string{"SimpleUSB/", "Radio/", "Voter/"} {
				if value == prefix+old {
					lines[i] = m[1] + m[2] + m[3] + prefix + req.Node + m[5]
				}
			}
		case (name == "extensions.conf" || name == "savenode.conf") && key == "node" && value == old:
			lines[i] = m[1] + m[2] + m[3] + req.Node + m[5]
		}
	}
	if name == "rpt.conf" {
		if !foundNode {
			return "", errors.New("la sección principal debe estar definida directamente en rpt.conf")
		}
		if !foundID {
			for i, line := range lines {
				if h := reAllmonHeader.FindStringSubmatch(line); h != nil && strings.TrimSpace(h[1]) == req.Node {
					lines[i] += "\nidrecording = |i" + req.Callsign
					break
				}
			}
		}
	}
	if (name == "iax.conf" || name == "rpt_http_registrations.conf") && !foundRegistration && req.Password != "" {
		target, prefix := "registrations", ""
		if name == "iax.conf" {
			target, prefix = "general", ";"
		}
		added := false
		for i, line := range lines {
			if h := reAllmonHeader.FindStringSubmatch(line); h != nil && strings.TrimSpace(h[1]) == target {
				lines[i] += "\n" + prefix + "register => " + req.Node + ":" + req.Password + "@register.allstarlink.org"
				added = true
				break
			}
		}
		if !added {
			return "", fmt.Errorf("falta la sección %s en %s", target, name)
		}
	}
	if name == "savenode.conf" && req.Password != "" {
		target := false
		for _, line := range lines {
			if m := nodeAssignment.FindStringSubmatch(line); m != nil && strings.EqualFold(m[2], "NODE") {
				target = strings.TrimSpace(m[4]) == req.Node || strings.TrimSpace(m[4]) == "0"
			}
		}
		if target {
			for i, line := range lines {
				if m := nodeAssignment.FindStringSubmatch(line); m != nil {
					if strings.EqualFold(m[2], "NODE") {
						lines[i] = m[1] + m[2] + m[3] + req.Node + m[5]
					}
					if strings.EqualFold(m[2], "PASSWORD") {
						lines[i] = m[1] + m[2] + m[3] + req.Password + m[5]
					}
					if strings.EqualFold(m[2], "ENABLE") {
						lines[i] = m[1] + m[2] + m[3] + "1" + m[5]
					}
				}
			}
		}
	}
	return strings.ReplaceAll(strings.Join(lines, "\n"), "\n", eol), nil
}

func writePrimaryBackup(path, content string) error {
	f, err := os.OpenFile(path+".amerimon.bak", os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.WriteString(content); err != nil {
		return err
	}
	return f.Sync()
}

func writePrimaryFile(path, content string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(resolved), ".amerimon-node-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if owner, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := temp.Chown(int(owner.Uid), int(owner.Gid)); err != nil {
			return err
		}
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := temp.WriteString(content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), resolved); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(resolved))
}

func (p primaryNodeControl) access(node string) ([]nodeAccessEntry, error) {
	result := make([]nodeAccessEntry, 0)
	for _, family := range []string{"allowlist", "denylist"} {
		raw, err := p.ami("database show " + family + "/" + node)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(raw, "results found") {
			return nil, errors.New("respuesta AstDB inesperada")
		}
		prefix := "/" + family + "/" + node + "/"
		for _, line := range config.ParseAMIOutputToLines(raw) {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			key, reason, ok := strings.Cut(strings.TrimPrefix(line, prefix), " :")
			if !ok {
				return nil, errors.New("entrada AstDB no reconocida")
			}
			result = append(result, nodeAccessEntry{family, strings.TrimSpace(key), strings.TrimSpace(reason)})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Type+result[i].Node < result[j].Type+result[j].Node })
	return result, nil
}

func validateNodeAccess(entry nodeAccessEntry, primary string) error {
	if entry.Type != "allowlist" && entry.Type != "denylist" {
		return errors.New("Lista inválida")
	}
	if !primaryCallsign.MatchString(entry.Node) && !(isNumericNode(entry.Node) && len(entry.Node) <= 7) {
		return errors.New("Introduce un número de nodo o indicativo válido")
	}
	if entry.Node == primary {
		return errors.New("El nodo principal no puede incluirse a sí mismo")
	}
	if len(entry.Reason) > 200 || strings.ContainsAny(entry.Reason, "\"\\\r\n\x00") {
		return errors.New("El motivo no puede superar 200 caracteres ni contener comillas, barras inversas o saltos de línea")
	}
	for _, c := range entry.Reason {
		if c < 32 || c == 127 {
			return errors.New("El motivo contiene caracteres de control")
		}
	}
	return nil
}

func (p primaryNodeControl) putAccess(node string, entry nodeAccessEntry) error {
	if err := validateNodeAccess(entry, node); err != nil {
		return err
	}
	raw, err := p.ami(fmt.Sprintf("database put %s/%s %s \"%s\"", entry.Type, node, entry.Node, entry.Reason))
	if err != nil {
		return err
	}
	if !strings.Contains(raw, "Updated database successfully") {
		return errors.New("AstDB no confirmó el guardado")
	}
	return nil
}

func (p primaryNodeControl) removeAccess(node string, entry nodeAccessEntry) error {
	if err := validateNodeAccess(entry, node); err != nil {
		return err
	}
	raw, err := p.ami("database del " + entry.Type + "/" + node + " " + entry.Node)
	if err != nil {
		return err
	}
	if !strings.Contains(raw, "Database entry removed") && !strings.Contains(raw, "Database entry does not exist") {
		return errors.New("AstDB no confirmó la eliminación")
	}
	return nil
}

func (p primaryNodeControl) getAccess(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	node, err := p.primary()
	if err != nil {
		return primaryNodeError("No se pudo identificar el nodo principal", err)
	}
	entries, err := p.access(node)
	if err != nil {
		return primaryNodeError("No se pudieron leer las listas de acceso", err)
	}
	return c.JSON(http.StatusOK, echo.Map{"node": node, "entries": entries})
}

func (p primaryNodeControl) changeAccess(c echo.Context, remove bool) error {
	var req struct {
		Primary string `json:"primary_node"`
		nodeAccessEntry
	}
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	req.Node = strings.ToUpper(strings.TrimSpace(req.Node))
	req.Reason = strings.TrimSpace(req.Reason)
	if !systemControlLock.TryLock() {
		return apiError(http.StatusConflict, "Hay otra operación de configuración o mantenimiento en curso")
	}
	defer systemControlLock.Unlock()
	node, err := p.primary()
	if err != nil {
		return primaryNodeError("No se pudo identificar el nodo principal", err)
	}
	if req.Primary != node {
		return apiError(http.StatusConflict, "El nodo principal cambió. Recarga la configuración")
	}
	if err := validateNodeAccess(req.nodeAccessEntry, node); err != nil {
		return apiError(http.StatusBadRequest, err.Error())
	}
	if remove {
		err = p.removeAccess(node, req.nodeAccessEntry)
	} else {
		err = p.putAccess(node, req.nodeAccessEntry)
	}
	if err != nil {
		return primaryNodeError("No se pudo actualizar la lista de acceso", err)
	}
	log.Printf("primary node: lista actualizada nodo=%s lista=%s remoto=%s eliminar=%t", node, req.Type, req.Node, remove)
	return c.JSON(http.StatusOK, echo.Map{"status": "ok"})
}
