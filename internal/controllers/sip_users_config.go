package controllers

// Este archivo implementa la lectura y administración de usuarios IAX y extensiones PJSIP.

import (
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

var sipUserName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`)

// sipUsersControl gestiona la lectura y actualización de usuarios IAX y extensiones PJSIP.
type sipUsersControl struct {
	iaxPath, pjsipPath string
	reload             func(string) (string, error)
}

// newSIPUsersControl crea una nueva instancia de sipUsersControl con las rutas de configuración y la función de recarga de Asterisk.
func newSIPUsersControl() sipUsersControl {
	ami := config.CurrentAMI()
	rptPath := nz(ami.RPTConfigPath, "/etc/asterisk/rpt.conf")
	return sipUsersControl{
		iaxPath:   filepath.Join(filepath.Dir(rptPath), "iax.conf"),
		pjsipPath: nz(ami.PJSIPConfigPath, filepath.Join(filepath.Dir(rptPath), "pjsip.conf")),
		reload: func(command string) (string, error) {
			return config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, command, 5*time.Second)
		},
	}
}

// sipUserOption representa una opción de usuario SIP con su clave y valores asociados.
type sipUserOption struct {
	key    string
	values []string
}

// sipOption obtiene el valor de una opción SIP de una sección, devolviendo un valor por defecto si no existe.
func sipOption(s managerSection, key, fallback string) string {
	if value, ok := s.last(key); ok {
		return value
	}
	return fallback
}

// plainSIPSection determina si una sección SIP es "plana", es decir, no tiene comentarios en sus líneas.
func plainSIPSection(s managerSection) bool {
	for _, line := range s.lines[s.start+1 : s.end] {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return false
		}
	}
	header := strings.SplitN(s.lines[s.start], ";", 2)[0]
	return strings.TrimSpace(header) == "["+s.name+"]"
}

// validSIPValues verifica que los valores SIP no contengan caracteres inválidos.
func validSIPValues(values ...string) bool {
	for _, value := range values {
		if strings.ContainsAny(value, "\r\n\x00;#") {
			return false
		}
	}
	return true
}

// renderSIPOptions genera una representación en cadena de las opciones SIP proporcionadas.
func renderSIPOptions(options []sipUserOption) string {
	var b strings.Builder
	for _, option := range options {
		for _, value := range option.values {
			b.WriteString(option.key + "=" + value + "\n")
		}
	}
	return b.String()
}

// Conserva las opciones no expuestas por los formularios, las plantillas y los includes.
// rewriteSIPSections reescribe las secciones SIP en el contenido dado, aplicando las actualizaciones y eliminaciones especificadas.
func rewriteSIPSections(content string, updates map[int][]sipUserOption, deleted map[int]bool) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	sections := parseManagerSections(content)
	for i := len(sections) - 1; i >= 0; i-- {
		s := sections[i]
		if deleted[i] {
			lines = append(lines[:s.start], lines[s.end:]...)
			continue
		}
		options, ok := updates[i]
		if !ok {
			continue
		}
		keys := make(map[string]bool)
		for _, option := range options {
			keys[option.key] = true
		}
		block := append([]string{lines[s.start]}, strings.Split(strings.TrimSuffix(renderSIPOptions(options), "\n"), "\n")...)
		for _, line := range lines[s.start+1 : s.end] {
			match := managerOptionPattern.FindStringSubmatch(line)
			if match != nil && keys[strings.ToLower(match[1])] {
				continue
			}
			block = append(block, line)
		}
		tail := append([]string(nil), lines[s.end:]...)
		lines = append(append(lines[:s.start], block...), tail...)
	}
	output := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	if strings.Contains(content, "\r\n") {
		output = strings.ReplaceAll(output, "\n", "\r\n")
	}
	return output
}

// persist guarda los cambios en la configuración SIP y recarga Asterisk, manejando errores y respaldos.
func (p sipUsersControl) persist(c echo.Context, path, original, updated, command string) error {
	if err := writePrimaryBackup(path, original); err != nil {
		return primaryNodeError("No se pudo crear el respaldo; no se ha modificado la configuración", err)
	}
	if err := writePrimaryFile(path, updated); err != nil {
		return primaryNodeError("No se pudo guardar la configuración; comprueba los permisos del servicio Amerimon", err)
	}
	output, err := p.reload(command)
	if err != nil {
		return primaryNodeError("El archivo se guardó, pero Asterisk no confirmó la recarga. Revisa AMI y recarga antes de reintentar", err)
	}
	lower := strings.ToLower(output)
	if strings.Contains(lower, "no such command") || strings.Contains(lower, "no such module") ||
		strings.Contains(lower, "unable to") || strings.Contains(lower, "failed") {
		return apiError(http.StatusBadGateway, "El archivo se guardó, pero Asterisk rechazó la recarga. Revisa sus registros")
	}
	return c.JSON(http.StatusOK, echo.Map{"message": "Configuración guardada y recarga enviada a Asterisk"})
}
