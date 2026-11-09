package controllers

// Este archivo implementa la administración del acceso público al nodo principal.

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

type publicCallsignEntry struct {
	Type     string `json:"type"`
	Callsign string `json:"callsign"`
	Reason   string `json:"reason"`
}

func GetPrimaryPublicAccess(c echo.Context) error {
	return newPrimaryNodeControl().getPublicAccess(c)
}

func SavePrimaryPublicAccess(c echo.Context) error {
	return newPrimaryNodeControl().changePublicAccess(c, false)
}

func DeletePrimaryPublicAccess(c echo.Context) error {
	return newPrimaryNodeControl().changePublicAccess(c, true)
}

func (p primaryNodeControl) publicAccess() ([]publicCallsignEntry, error) {
	result := make([]publicCallsignEntry, 0)
	for _, family := range []string{"whitelist", "blacklist"} {
		raw, err := p.ami("database show " + family)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(raw, "results found") {
			return nil, errors.New("respuesta AstDB inesperada")
		}
		prefix := "/" + family + "/"
		for _, line := range config.ParseAMIOutputToLines(raw) {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			callsign, reason, ok := strings.Cut(strings.TrimPrefix(line, prefix), " :")
			if !ok {
				return nil, errors.New("entrada AstDB no reconocida")
			}
			entry := publicCallsignEntry{
				Type: family, Callsign: strings.TrimSpace(callsign), Reason: strings.TrimSpace(reason),
			}
			if err := validatePublicCallsignKey(entry); err != nil {
				return nil, err
			}
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Type+result[i].Callsign < result[j].Type+result[j].Callsign
	})
	return result, nil
}

func validatePublicCallsign(entry publicCallsignEntry) error {
	if err := validatePublicCallsignKey(entry); err != nil {
		return err
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

func validatePublicCallsignKey(entry publicCallsignEntry) error {
	if entry.Type != "whitelist" && entry.Type != "blacklist" {
		return errors.New("Lista de indicativos inválida")
	}
	if !primaryCallsign.MatchString(strings.ToUpper(entry.Callsign)) {
		return errors.New("Introduce un indicativo válido")
	}
	return nil
}

func (p primaryNodeControl) getPublicAccess(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	entries, err := p.publicAccess()
	if err != nil {
		return primaryNodeError("No se pudieron leer las listas de indicativos de [allstar-public]", err)
	}
	return c.JSON(http.StatusOK, echo.Map{"entries": entries})
}

func (p primaryNodeControl) changePublicAccess(c echo.Context, remove bool) error {
	var req publicCallsignEntry
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	req.Callsign = strings.ToUpper(strings.TrimSpace(req.Callsign))
	req.Reason = strings.TrimSpace(req.Reason)
	if !systemControlLock.TryLock() {
		return apiError(http.StatusConflict, "Hay otra operación de configuración o mantenimiento en curso")
	}
	defer systemControlLock.Unlock()
	if err := validatePublicCallsign(req); err != nil {
		return apiError(http.StatusBadRequest, err.Error())
	}
	command := ""
	if remove {
		command = "database del " + req.Type + " " + req.Callsign
	} else {
		command = fmt.Sprintf("database put %s %s \"%s\"", req.Type, req.Callsign, req.Reason)
	}
	raw, err := p.ami(command)
	if err != nil {
		return primaryNodeError("No se pudo actualizar la lista de indicativos", err)
	}
	if (!remove && !strings.Contains(raw, "Updated database successfully")) ||
		(remove && !strings.Contains(raw, "Database entry removed") && !strings.Contains(raw, "Database entry does not exist")) {
		return primaryNodeError("AstDB no confirmó la actualización de la lista de indicativos", errors.New("respuesta AMI inesperada"))
	}
	log.Printf("primary node: lista de indicativos actualizada lista=%s indicativo=%s eliminar=%t", req.Type, req.Callsign, remove)
	return c.JSON(http.StatusOK, echo.Map{"status": "ok"})
}
