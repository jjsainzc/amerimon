package controllers

import (
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
)

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
		if strings.EqualFold(s.name, "general") || strings.Contains(s.lines[s.start], "(!)") {
			continue
		}
		users[s.name] = IAXUser{
			Name: s.name, Type: sipOption(s, "type", "friend"),
			Context: sipOption(s, "context", ""), Auth: sipOption(s, "auth", ""),
			Secret: sipOption(s, "secret", ""), Host: sipOption(s, "host", "dynamic"),
			Transfer: sipOption(s, "transfer", "no"),
			Disallow: s.values("disallow"), Allow: s.values("allow"),
		}
	}
	return c.JSON(http.StatusOK, users)
}

func (p sipUsersControl) saveIAX(c echo.Context) error {
	var u IAXUser
	if err := c.Bind(&u); err != nil {
		return apiError(http.StatusBadRequest, "Datos IAX inválidos")
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
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	data, err := os.ReadFile(p.iaxPath)
	if err != nil {
		return primaryNodeError("No se pudo leer iax.conf", err)
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

func nz(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
