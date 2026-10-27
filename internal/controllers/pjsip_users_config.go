package controllers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

type PJSIPExtension struct {
	ID              string   `json:"id"`
	Context         string   `json:"context"`
	Disallow        []string `json:"disallow"`
	Allow           []string `json:"allow"`
	CallerID        string   `json:"caller_id"`
	Password        string   `json:"password"`
	MaxContacts     string   `json:"max_contacts"`
	DirectMedia     string   `json:"direct_media"`
	MediaEncryption string   `json:"media_encryption"`
}

func GetPJSIPExtensions(c echo.Context) error         { return newSIPUsersControl().getPJSIP(c) }
func SaveOrUpdatePJSIPExtension(c echo.Context) error { return newSIPUsersControl().savePJSIP(c) }
func DeletePJSIPExtension(c echo.Context) error       { return newSIPUsersControl().deletePJSIP(c) }

func (p sipUsersControl) getPJSIP(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	c.Response().Header().Set("Cache-Control", "no-store")
	data, err := os.ReadFile(p.pjsipPath)
	if err != nil {
		return primaryNodeError("No se pudo leer pjsip.conf; comprueba ami.pjsip_config_path y los permisos del servicio Amerimon", err)
	}
	sections := parseManagerSections(string(data))
	list := make([]PJSIPExtension, 0)
	for _, s := range sections {
		if sipOption(s, "type", "") != "endpoint" || strings.Contains(s.lines[s.start], "(!)") {
			continue
		}
		ext := PJSIPExtension{
			ID: s.name, Context: sipOption(s, "context", "sip-phones"),
			Disallow: s.values("disallow"), Allow: s.values("allow"),
			CallerID:        sipOption(s, "callerid", ""),
			DirectMedia:     sipOption(s, "direct_media", "no"),
			MediaEncryption: sipOption(s, "media_encryption", "no"),
		}
		for _, related := range sections {
			if related.name == sipOption(s, "auth", s.name) && sipOption(related, "type", "") == "auth" {
				ext.Password = sipOption(related, "password", "")
			}
			if related.name == sipOption(s, "aors", s.name) && sipOption(related, "type", "") == "aor" {
				ext.MaxContacts = sipOption(related, "max_contacts", "4")
			}
		}
		list = append(list, ext)
	}
	return c.JSON(http.StatusOK, list)
}

// Solo se modifican objetos inequívocos y exclusivos del endpoint.
func pjsipUserSections(content, id string) (map[string]int, error) {
	sections := parseManagerSections(content)
	found := map[string]int{}
	for i, s := range sections {
		if s.name == id && sipOption(s, "type", "") == "endpoint" {
			if _, exists := found["endpoint"]; exists {
				return nil, fmt.Errorf("El endpoint PJSIP está duplicado")
			}
			found["endpoint"] = i
		}
	}
	endpointIndex, exists := found["endpoint"]
	if !exists {
		for _, s := range sections {
			if s.name == id {
				return nil, fmt.Errorf("El ID ya pertenece a otro objeto PJSIP o plantilla")
			}
		}
		return found, nil
	}
	endpoint := sections[endpointIndex]
	if !plainSIPSection(endpoint) {
		return nil, fmt.Errorf("El endpoint usa una plantilla o contiene includes; edítalo manualmente")
	}
	for _, kind := range []string{"auth", "aor"} {
		key := kind
		if kind == "aor" {
			key = "aors"
		}
		name := sipOption(endpoint, key, "")
		if name == "" || strings.Contains(name, ",") {
			return nil, fmt.Errorf("El endpoint debe tener un único auth y AOR local")
		}
		for i, s := range sections {
			if s.name == name && sipOption(s, "type", "") == kind {
				if _, exists := found[kind]; exists || !plainSIPSection(s) {
					return nil, fmt.Errorf("El objeto PJSIP %s está duplicado, usa una plantilla o contiene includes", kind)
				}
				found[kind] = i
			}
			if i != endpointIndex && sipOption(s, "type", "") == "endpoint" {
				for _, reference := range strings.Split(sipOption(s, key, ""), ",") {
					if strings.TrimSpace(reference) == name {
						return nil, fmt.Errorf("El objeto PJSIP %s se comparte con otro endpoint", kind)
					}
				}
			}
		}
		if _, ok := found[kind]; !ok {
			return nil, fmt.Errorf("No se encontró el objeto PJSIP %s en el archivo principal", kind)
		}
	}
	if sipOption(sections[found["auth"]], "auth_type", "") != "userpass" {
		return nil, fmt.Errorf("Solo se admite autenticación PJSIP userpass")
	}
	return found, nil
}

func (p sipUsersControl) savePJSIP(c echo.Context) error {
	var ext PJSIPExtension
	if err := c.Bind(&ext); err != nil {
		return apiError(http.StatusBadRequest, "Datos PJSIP inválidos")
	}
	contacts, err := strconv.Atoi(nz(ext.MaxContacts, "4"))
	if !sipUserName.MatchString(ext.ID) || err != nil || contacts < 1 ||
		!validSIPValues(ext.Context, ext.CallerID, ext.Password, ext.DirectMedia, ext.MediaEncryption) ||
		!validSIPValues(ext.Disallow...) || !validSIPValues(ext.Allow...) ||
		strings.TrimSpace(ext.Context) == "" || strings.TrimSpace(ext.Password) == "" {
		return apiError(http.StatusBadRequest, "ID, contexto, contraseña o max_contacts PJSIP inválido; no se permiten saltos de línea ni comentarios de configuración")
	}
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	data, err := os.ReadFile(p.pjsipPath)
	if err != nil {
		return primaryNodeError("No se pudo leer pjsip.conf", err)
	}
	found, err := pjsipUserSections(string(data), ext.ID)
	if err != nil {
		return apiError(http.StatusConflict, err.Error())
	}
	options := map[string][]sipUserOption{
		"endpoint": {
			{"context", []string{ext.Context}}, {"disallow", ext.Disallow}, {"allow", ext.Allow},
			{"callerid", []string{ext.CallerID}},
			{"direct_media", []string{nz(ext.DirectMedia, "no")}},
			{"media_encryption", []string{nz(ext.MediaEncryption, "no")}},
		},
		"auth": {{"password", []string{ext.Password}}},
		"aor":  {{"max_contacts", []string{strconv.Itoa(contacts)}}},
	}
	output := string(data)
	if len(found) > 0 {
		updates := make(map[int][]sipUserOption)
		for kind, index := range found {
			updates[index] = options[kind]
		}
		output = rewriteSIPSections(output, updates, nil)
	} else {
		if len(ext.Disallow) == 0 {
			options["endpoint"][1].values = []string{"all"}
		}
		if len(ext.Allow) == 0 {
			options["endpoint"][2].values = []string{"ulaw", "alaw", "slin", "gsm"}
		}
		options["endpoint"] = append(options["endpoint"],
			sipUserOption{"auth", []string{ext.ID}}, sipUserOption{"aors", []string{ext.ID}},
			sipUserOption{"rtp_symmetric", []string{"yes"}}, sipUserOption{"force_rport", []string{"yes"}},
			sipUserOption{"rewrite_contact", []string{"yes"}},
		)
		options["auth"] = append(options["auth"],
			sipUserOption{"auth_type", []string{"userpass"}}, sipUserOption{"username", []string{ext.ID}},
		)
		for _, kind := range []string{"endpoint", "auth", "aor"} {
			output = strings.TrimRight(output, "\r\n") + "\n\n[" + ext.ID + "]\ntype=" + kind + "\n" + renderSIPOptions(options[kind])
		}
	}
	return p.persist(c, p.pjsipPath, string(data), output, "pjsip reload")
}

func (p sipUsersControl) deletePJSIP(c echo.Context) error {
	id := c.Param("id")
	if !sipUserName.MatchString(id) {
		return apiError(http.StatusBadRequest, "ID de extensión PJSIP inválido")
	}
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	data, err := os.ReadFile(p.pjsipPath)
	if err != nil {
		return primaryNodeError("No se pudo leer pjsip.conf", err)
	}
	found, err := pjsipUserSections(string(data), id)
	if err != nil {
		return apiError(http.StatusConflict, err.Error())
	}
	if len(found) == 0 {
		return apiError(http.StatusNotFound, "La extensión PJSIP no existe en el archivo principal")
	}
	deleted := make(map[int]bool)
	for _, i := range found {
		deleted[i] = true
	}
	return p.persist(c, p.pjsipPath, string(data), rewriteSIPSections(string(data), nil, deleted), "pjsip reload")
}
