package controllers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labstack/echo/v4"
)

var iaxClientTemplateHeader = regexp.MustCompile(`(?i)^\s*\[iaxclient\]\s*\(\s*!\s*\)\s*(?:;.*)?$`)
var iaxClientUserHeader = regexp.MustCompile(`(?i)^\s*\[[^\]]+\]\s*\(iaxclient\)`)
var iaxClientUsersInclude = regexp.MustCompile(`^\s*#tryinclude\s+(?:"([^"]+)"|'([^']+)'|([^;\s"']+))\s*(?:;.*)?$`)

type IAXUser struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Context  string   `json:"context"`
	Auth     string   `json:"auth"`
	Secret   string   `json:"secret"`
	Host     string   `json:"host"`
	Disallow []string `json:"disallow"`
	Allow    []string `json:"allow"`
	Transfer string   `json:"transfer"`
}

func GetIAXUsers(c echo.Context) error         { return newSIPUsersControl().getIAX(c) }
func SaveOrUpdateIAXUser(c echo.Context) error { return newSIPUsersControl().saveIAX(c) }
func DeleteIAXUser(c echo.Context) error       { return newSIPUsersControl().deleteIAX(c) }

func (p sipUsersControl) getIAX(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	c.Response().Header().Set("Cache-Control", "no-store")
	data, err := os.ReadFile(p.iaxPath)
	if err != nil {
		return primaryNodeError("No se pudo leer iax.conf; comprueba la ruta y los permisos del servicio Amerimon", err)
	}
	users := make(map[string]IAXUser)
	for _, s := range parseManagerSections(string(data)) {
		if strings.EqualFold(s.name, "general") || isHiddenIAXSection(s.name) || strings.Contains(s.lines[s.start], "(!)") {
			continue
		}
		if err := addIAXUser(users, s); err != nil {
			return apiError(http.StatusConflict, err.Error())
		}
	}
	includePath, templated, err := iaxClientUsersPath(string(data), p.iaxPath)
	if err != nil {
		return apiError(http.StatusConflict, err.Error())
	}
	if templated {
		included, err := os.ReadFile(includePath)
		if err != nil && !os.IsNotExist(err) {
			return primaryNodeError("No se pudo leer el archivo IAX incluido; comprueba la ruta y los permisos del servicio Amerimon", err)
		}
		for _, s := range parseManagerSections(string(included)) {
			if !isIAXClientUserSection(s) || isHiddenIAXSection(s.name) {
				continue
			}
			if err := addIAXUser(users, s); err != nil {
				return apiError(http.StatusConflict, err.Error())
			}
		}
	}
	return c.JSON(http.StatusOK, users)
}

func (p sipUsersControl) saveIAX(c echo.Context) error {
	var u IAXUser
	if err := c.Bind(&u); err != nil {
		return apiError(http.StatusBadRequest, "Datos IAX inválidos")
	}
	if isHiddenIAXSection(u.Name) {
		return apiError(http.StatusForbidden, "No se pueden modificar las secciones IAX reservadas")
	}
	if !sipUserName.MatchString(u.Name) || strings.EqualFold(u.Name, "general") ||
		!validSIPValues(u.Type, u.Context, u.Auth, u.Secret, u.Host, u.Transfer) ||
		!validSIPValues(u.Disallow...) || !validSIPValues(u.Allow...) ||
		strings.TrimSpace(u.Context) == "" || strings.TrimSpace(u.Secret) == "" {
		return apiError(http.StatusBadRequest, "Nombre, contexto o secreto IAX inválido; no se permiten saltos de línea ni comentarios de configuración")
	}
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	data, err := os.ReadFile(p.iaxPath)
	if err != nil {
		return primaryNodeError("No se pudo leer iax.conf", err)
	}
	sections := parseManagerSections(string(data))
	includePath, templated, err := iaxClientUsersPath(string(data), p.iaxPath)
	if err != nil {
		return apiError(http.StatusConflict, err.Error())
	}
	if templated {
		return p.saveIAXWithTemplate(c, u, string(data), sections, includePath)
	}
	index := -1
	for i, s := range sections {
		if s.name == u.Name {
			if index >= 0 || !plainSIPSection(s) {
				return apiError(http.StatusConflict, "La sección IAX está duplicada, usa una plantilla o contiene includes; edítala manualmente")
			}
			index = i
		}
	}
	options := []sipUserOption{
		{"type", []string{nz(u.Type, "friend")}}, {"context", []string{u.Context}},
		{"auth", []string{u.Auth}}, {"secret", []string{u.Secret}},
		{"host", []string{nz(u.Host, "dynamic")}},
		{"disallow", u.Disallow}, {"allow", u.Allow},
		{"transfer", []string{nz(u.Transfer, "no")}},
	}
	output := string(data)
	if index >= 0 {
		output = rewriteSIPSections(output, map[int][]sipUserOption{index: options}, nil)
	} else {
		output = strings.TrimRight(output, "\r\n") + "\n\n[" + u.Name + "]\n" + renderSIPOptions(options)
	}
	return p.persist(c, p.iaxPath, string(data), output, "iax2 reload")
}

func (p sipUsersControl) deleteIAX(c echo.Context) error {
	name := c.Param("name")
	if !sipUserName.MatchString(name) || strings.EqualFold(name, "general") {
		return apiError(http.StatusBadRequest, "Nombre de usuario IAX inválido")
	}
	if isHiddenIAXSection(name) {
		return apiError(http.StatusForbidden, "No se pueden eliminar las secciones IAX reservadas")
	}
	if strings.EqualFold(name, "iaxrpt") {
		return apiError(http.StatusForbidden, "No se puede eliminar la sección IAX iaxrpt")
	}
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	data, err := os.ReadFile(p.iaxPath)
	if err != nil {
		return primaryNodeError("No se pudo leer iax.conf", err)
	}
	includePath, templated, err := iaxClientUsersPath(string(data), p.iaxPath)
	if err != nil {
		return apiError(http.StatusConflict, err.Error())
	}
	if templated {
		return p.deleteIAXWithTemplate(c, name, string(data), includePath)
	}
	deleted := make(map[int]bool)
	for i, s := range parseManagerSections(string(data)) {
		if s.name == name {
			if len(deleted) > 0 || !plainSIPSection(s) {
				return apiError(http.StatusConflict, "La sección IAX está duplicada, usa una plantilla o contiene includes; edítala manualmente")
			}
			deleted[i] = true
		}
	}
	if len(deleted) == 0 {
		return apiError(http.StatusNotFound, "El usuario IAX no existe en el archivo principal")
	}
	return p.persist(c, p.iaxPath, string(data), rewriteSIPSections(string(data), nil, deleted), "iax2 reload")
}

func isHiddenIAXSection(name string) bool {
	switch strings.ToLower(name) {
	case "radio", "allstar-sys", "allstar-public":
		return true
	default:
		return false
	}
}

func isIAXClientUserSection(s managerSection) bool {
	return iaxClientUserHeader.MatchString(s.lines[s.start])
}

func addIAXUser(users map[string]IAXUser, s managerSection) error {
	if _, exists := users[s.name]; exists {
		return errors.New("Hay más de una sección IAX con el mismo nombre")
	}
	users[s.name] = IAXUser{
		Name: s.name, Type: sipOption(s, "type", "friend"),
		Context: sipOption(s, "context", "iax-client"), Auth: sipOption(s, "auth", "md5"),
		Secret: sipOption(s, "secret", ""), Host: sipOption(s, "host", "dynamic"),
		Transfer: sipOption(s, "transfer", "no"),
		Disallow: s.values("disallow"), Allow: s.values("allow"),
	}
	return nil
}

func iaxClientUsersPath(content, iaxPath string) (string, bool, error) {
	var template *managerSection
	sections := parseManagerSections(content)
	for i := range sections {
		s := &sections[i]
		if iaxClientTemplateHeader.MatchString(s.lines[s.start]) {
			if template != nil {
				return "", true, errors.New("La plantilla [iaxclient](!) está duplicada; edítala manualmente")
			}
			template = s
		}
	}
	if template == nil {
		return "", false, nil
	}
	includePath := ""
	for _, line := range template.lines[template.start+1 : template.end] {
		match := iaxClientUsersInclude.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		if includePath != "" {
			return "", true, errors.New("La plantilla [iaxclient](!) contiene varios #tryinclude; el destino de usuarios es ambiguo")
		}
		for _, path := range match[1:] {
			if path != "" {
				includePath = path
				break
			}
		}
		if strings.ContainsAny(includePath, "*?[]\x00") {
			return "", true, errors.New("El #tryinclude de [iaxclient](!) debe indicar un único archivo, sin comodines")
		}
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(filepath.Dir(iaxPath), includePath)
		}
		includePath = filepath.Clean(includePath)
		if includePath == filepath.Clean(iaxPath) {
			return "", true, errors.New("El #tryinclude de [iaxclient](!) no puede apuntar al mismo iax.conf")
		}
	}
	if includePath == "" {
		return "", true, errors.New("La plantilla [iaxclient](!) no tiene un #tryinclude válido dentro de su propia stanza")
	}
	return includePath, true, nil
}

func (p sipUsersControl) saveIAXWithTemplate(c echo.Context, u IAXUser, mainContent string, mainSections []managerSection, includePath string) error {
	includeContent, err := os.ReadFile(includePath)
	includeExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return primaryNodeError("No se pudo leer el archivo IAX incluido", err)
	}
	includeText := string(includeContent)
	includeSections := parseManagerSections(includeText)
	mainIndex, includeIndex := -1, -1
	mainMatches, includeMatches := 0, 0
	for i, s := range mainSections {
		if s.name == u.Name {
			mainIndex, mainMatches = i, mainMatches+1
		}
	}
	for i, s := range includeSections {
		if s.name == u.Name {
			includeIndex, includeMatches = i, includeMatches+1
		}
	}
	if mainMatches+includeMatches > 1 {
		return apiError(http.StatusConflict, "La sección IAX está duplicada; edítala manualmente")
	}
	if mainMatches == 1 {
		s := mainSections[mainIndex]
		if !plainSIPSection(s) {
			return apiError(http.StatusConflict, "La sección IAX usa una plantilla o contiene includes; edítala manualmente")
		}
		options := []sipUserOption{
			{"type", []string{nz(u.Type, "friend")}}, {"context", []string{u.Context}},
			{"auth", []string{u.Auth}}, {"secret", []string{u.Secret}},
			{"host", []string{nz(u.Host, "dynamic")}},
			{"disallow", u.Disallow}, {"allow", u.Allow},
			{"transfer", []string{nz(u.Transfer, "no")}},
		}
		return p.persist(c, p.iaxPath, mainContent, rewriteSIPSections(mainContent, map[int][]sipUserOption{mainIndex: options}, nil), "iax2 reload")
	}
	if includeMatches == 1 {
		s := includeSections[includeIndex]
		if !isIAXClientUserSection(s) || len(s.values("secret")) > 1 {
			return apiError(http.StatusConflict, "La sección IAX del archivo incluido no usa la plantilla esperada; edítala manualmente")
		}
		return p.persist(c, includePath, includeText, rewriteIAXClientSecret(includeText, includeIndex, u.Secret), "iax2 reload")
	}
	updated := appendIAXClientUser(includeText, u.Name, u.Secret)
	if includeExists {
		return p.persist(c, includePath, includeText, updated, "iax2 reload")
	}
	return p.persistNewIAXFile(c, includePath, updated)
}

func (p sipUsersControl) deleteIAXWithTemplate(c echo.Context, name, mainContent, includePath string) error {
	includeContent, err := os.ReadFile(includePath)
	if err != nil && !os.IsNotExist(err) {
		return primaryNodeError("No se pudo leer el archivo IAX incluido", err)
	}
	includeText := string(includeContent)
	mainDeleted, includeDeleted := make(map[int]bool), make(map[int]bool)
	for i, s := range parseManagerSections(mainContent) {
		if s.name == name {
			if !plainSIPSection(s) {
				return apiError(http.StatusConflict, "La sección IAX usa una plantilla o contiene includes; edítala manualmente")
			}
			mainDeleted[i] = true
		}
	}
	for i, s := range parseManagerSections(includeText) {
		if s.name == name {
			if !isIAXClientUserSection(s) {
				return apiError(http.StatusConflict, "La sección IAX del archivo incluido no usa la plantilla esperada; edítala manualmente")
			}
			includeDeleted[i] = true
		}
	}
	if len(mainDeleted)+len(includeDeleted) > 1 {
		return apiError(http.StatusConflict, "La sección IAX está duplicada; edítala manualmente")
	}
	if len(mainDeleted) == 1 {
		return p.persist(c, p.iaxPath, mainContent, rewriteSIPSections(mainContent, nil, mainDeleted), "iax2 reload")
	}
	if len(includeDeleted) == 1 {
		return p.persist(c, includePath, includeText, rewriteSIPSections(includeText, nil, includeDeleted), "iax2 reload")
	}
	return apiError(http.StatusNotFound, "El usuario IAX no existe en iax.conf ni en el archivo incluido")
}

func appendIAXClientUser(content, name, secret string) string {
	var b strings.Builder
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	if content != "" {
		b.WriteString(content)
		if !strings.HasSuffix(content, eol) {
			b.WriteString(eol)
		}
		if !strings.HasSuffix(content, eol+eol) {
			b.WriteString(eol)
		}
	}
	b.WriteString("[" + name + "](iaxclient)                ; pull in the config from the [iaxclient] section in" + eol)
	b.WriteString("    user = " + name + "                      ; iax.conf and then add this user's specific information" + eol)
	b.WriteString("    secret = " + secret + " ; (user, secret, and callerid). n0call is the USERNAME in" + eol)
	b.WriteString("    callerid = \"" + strings.ToUpper(name) + "\"                ; their client account configuration." + eol)
	return b.String()
}

func rewriteIAXClientSecret(content string, index int, secret string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	s := parseManagerSections(content)[index]
	secretLine := regexp.MustCompile(`(?i)^(\s*secret\s*=\s*)([^;#]*?)(\s*(?:[;#].*)?)$`)
	for i := s.start + 1; i < s.end; i++ {
		match := secretLine.FindStringSubmatch(lines[i])
		if match != nil {
			lines[i] = match[1] + secret + match[3]
			output := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
			if strings.Contains(content, "\r\n") {
				output = strings.ReplaceAll(output, "\n", "\r\n")
			}
			return output
		}
	}
	output := strings.Join(append(append(lines[:s.end], "    secret = "+secret), lines[s.end:]...), "\n")
	if strings.Contains(content, "\r\n") {
		output = strings.ReplaceAll(output, "\n", "\r\n")
	}
	return output
}

func (p sipUsersControl) persistNewIAXFile(c echo.Context, path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return primaryNodeError("No se pudo crear el directorio de iaxrpt-users.conf", err)
	}
	temp, err := os.CreateTemp(dir, ".amerimon-iax-*.tmp")
	if err != nil {
		return primaryNodeError("No se pudo preparar iaxrpt-users.conf", err)
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0o640); err != nil {
		temp.Close()
		return primaryNodeError("No se pudieron establecer los permisos de iaxrpt-users.conf", err)
	}
	if _, err := temp.WriteString(content); err != nil {
		temp.Close()
		return primaryNodeError("No se pudo escribir iaxrpt-users.conf", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return primaryNodeError("No se pudo sincronizar iaxrpt-users.conf", err)
	}
	if err := temp.Close(); err != nil {
		return primaryNodeError("No se pudo cerrar iaxrpt-users.conf", err)
	}
	if err := os.Link(temp.Name(), path); err != nil {
		return primaryNodeError("No se pudo crear iaxrpt-users.conf; comprueba la ruta y los permisos del servicio Amerimon", err)
	}
	if err := syncDirectory(dir); err != nil {
		return primaryNodeError("No se pudo sincronizar el directorio de iaxrpt-users.conf", err)
	}
	output, err := p.reload("iax2 reload")
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

func nz(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
