package controllers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

const maxAllmonNodes = 50

var (
	reAllmonNode   = regexp.MustCompile(`^\d{1,7}$`)
	reAllmonHost   = regexp.MustCompile(`^[A-Za-z0-9._:\[\]-]{1,253}$`)
	reAllmonUser   = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,64}$`)
	reAllmonHeader = regexp.MustCompile(`^\s*\[([^\]]*)\]`)
	reAllmonKey    = regexp.MustCompile(`^\s*([A-Za-z0-9_]+)\s*=`)
)

type allmonNodeIn struct {
	Node   string `json:"node"`
	Host   string `json:"host"`
	Port   string `json:"port"`
	User   string `json:"user"`
	Passwd string `json:"passwd"`
}

type allmonNodeOut struct {
	Node      string `json:"node"`
	Host      string `json:"host"`
	Port      string `json:"port"`
	User      string `json:"user"`
	Passwd    string `json:"passwd"`
	PasswdSet bool   `json:"passwd_set"`
}

func allmonWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		f.Close()
		return true
	}
	return os.IsNotExist(err)
}

// GetAllmonConfig devuelve los nodos del allmon.ini en uso, con sus contraseñas (solo admin).
func GetAllmonConfig(c echo.Context) error {
	path := allmonINIPath()
	instances, err := readAllmonInstances(path)
	if err != nil && !os.IsNotExist(err) {
		return apiError(http.StatusInternalServerError, "No se pudo leer "+path)
	}
	nodes := make([]allmonNodeOut, 0, len(instances))
	for _, i := range instances {
		nodes = append(nodes, allmonNodeOut{Node: i.Node, Host: i.Host, Port: i.Port, User: i.User, Passwd: i.Secret, PasswdSet: i.Secret != ""})
	}
	return c.JSON(http.StatusOK, echo.Map{
		"status":   "ok",
		"path":     path,
		"writable": allmonWritable(path),
		"nodes":    nodes,
	})
}

// SaveAllmonConfig actualiza solo las stanzas de nodos del allmon.ini en uso; el resto del archivo no se toca.
func SaveAllmonConfig(c echo.Context) error {
	var req struct {
		Nodes []allmonNodeIn `json:"nodes"`
	}
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	if len(req.Nodes) > maxAllmonNodes {
		return apiError(http.StatusBadRequest, "Demasiados nodos")
	}

	path := allmonINIPath()
	current, err := readAllmonInstances(path)
	if err != nil && !os.IsNotExist(err) {
		return apiError(http.StatusInternalServerError, "No se pudo leer "+path)
	}
	existing := map[string]bool{}
	for _, i := range current {
		existing[i.Node] = true
	}

	seen := map[string]bool{}
	for i := range req.Nodes {
		n := &req.Nodes[i]
		n.Node, n.Host, n.Port = strings.TrimSpace(n.Node), strings.TrimSpace(n.Host), strings.TrimSpace(n.Port)
		n.User, n.Passwd = strings.TrimSpace(n.User), strings.TrimSpace(n.Passwd)
		if err := validateAllmonNode(*n, seen); err != nil {
			return apiError(http.StatusBadRequest, err.Error())
		}
		if n.Passwd == "" && !existing[n.Node] {
			return apiError(http.StatusBadRequest, "Nodo "+n.Node+": la contraseña es obligatoria")
		}
		seen[n.Node] = true
	}

	var original []byte
	if data, err := os.ReadFile(path); err == nil {
		original = data
	}
	out := rewriteAllmonNodes(string(original), req.Nodes, existing)

	if len(original) > 0 {
		_ = os.WriteFile(path+".amerimon.bak", original, 0o600) // respaldo previo, si el directorio lo permite
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		return apiError(http.StatusInternalServerError, "No hay permiso para escribir "+path)
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "path": path})
}

// rewriteAllmonNodes reescribe solo los bloques de nodos (existing); el resto del texto queda igual.
func rewriteAllmonNodes(content string, nodes []allmonNodeIn, existing map[string]bool) string {
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	byNode := map[string]allmonNodeIn{}
	for _, n := range nodes {
		byNode[n.Node] = n
	}

	lines := strings.SplitAfter(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var b strings.Builder
	handled := map[string]bool{}
	for i := 0; i < len(lines); {
		m := reAllmonHeader.FindStringSubmatch(lines[i])
		name := ""
		if m != nil {
			name = strings.TrimSpace(m[1])
		}
		j := i + 1
		for j < len(lines) && reAllmonHeader.FindStringSubmatch(lines[j]) == nil {
			j++
		}
		block := lines[i:j]
		i = j

		if m == nil || !existing[name] || handled[name] {
			for _, l := range block {
				b.WriteString(l)
			}
			continue
		}
		handled[name] = true
		n, keep := byNode[name]
		if !keep {
			continue // nodo eliminado
		}
		b.WriteString(updateAllmonBlock(block, n, eol))
	}

	for _, n := range nodes {
		if existing[n.Node] {
			continue
		}
		text := b.String()
		if text != "" && !strings.HasSuffix(text, "\n") {
			b.WriteString(eol)
		}
		if text != "" {
			b.WriteString(eol)
		}
		b.WriteString("[" + n.Node + "]" + eol + "system=Nodes" + eol + "host=" + n.Host + eol)
		if n.Port != "" {
			b.WriteString("port=" + n.Port + eol)
		}
		b.WriteString("user=" + n.User + eol + "passwd=" + n.Passwd + eol + "hideNodeURL=no" + eol + "menu=yes" + eol)
	}
	return b.String()
}

// updateAllmonBlock cambia host/port/user/passwd dentro de un bloque conservando el resto de líneas.
func updateAllmonBlock(block []string, n allmonNodeIn, eol string) string {
	want := map[string]string{"host": n.Host, "user": n.User}
	if n.Port != "" {
		want["port"] = n.Port
	}
	if n.Passwd != "" {
		want["passwd"] = n.Passwd
	}
	done := map[string]bool{}

	var b strings.Builder
	b.WriteString(block[0])
	lastKey := b.Len() // posición tras la última línea con contenido del bloque
	for _, l := range block[1:] {
		trim := strings.TrimSpace(l)
		if k := reAllmonKey.FindStringSubmatch(l); k != nil && !strings.HasPrefix(trim, ";") && !strings.HasPrefix(trim, "#") {
			key := strings.ToLower(k[1])
			switch {
			case key == "port" && n.Port == "":
				continue // puerto vacío: se quita para usar el 5038 por defecto
			case want[key] != "":
				if done[key] {
					continue
				}
				done[key] = true
				b.WriteString(k[1] + "=" + want[key] + eol)
				lastKey = b.Len()
				continue
			}
		}
		b.WriteString(l)
		if trim != "" {
			lastKey = b.Len()
		}
	}

	text := b.String()
	var missing strings.Builder
	for _, key := range []string{"host", "port", "user", "passwd"} {
		if v, ok := want[key]; ok && !done[key] {
			missing.WriteString(key + "=" + v + eol)
		}
	}
	if missing.Len() == 0 {
		return text
	}
	prefix := text[:lastKey]
	if !strings.HasSuffix(prefix, "\n") {
		prefix += eol
	}
	return prefix + missing.String() + text[lastKey:]
}

func validateAllmonNode(n allmonNodeIn, seen map[string]bool) error {
	switch {
	case !reAllmonNode.MatchString(n.Node):
		return errors.New("El nodo debe ser numérico (hasta 7 dígitos)")
	case seen[n.Node]:
		return errors.New("Nodo repetido: " + n.Node)
	case !reAllmonHost.MatchString(n.Host):
		return errors.New("Nodo " + n.Node + ": host inválido")
	case !reAllmonUser.MatchString(n.User):
		return errors.New("Nodo " + n.Node + ": usuario inválido")
	case strings.ContainsAny(n.Passwd, "\r\n\x00") || len(n.Passwd) > 128:
		return errors.New("Nodo " + n.Node + ": contraseña inválida")
	}
	if n.Port != "" {
		if p, err := strconv.Atoi(n.Port); err != nil || p < 1 || p > 65535 {
			return errors.New("Nodo " + n.Node + ": puerto inválido")
		}
	}
	return nil
}
