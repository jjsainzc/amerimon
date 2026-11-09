package controllers

// Este archivo implementa la exportación e importación de permisos del nodo principal.

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

const maxAccessTransferEntries = 1000
const maxAccessTransferBytes = 1024 * 1024

type nodeAccessDocument struct {
	Format  string            `json:"format"`
	Version int               `json:"version"`
	Node    string            `json:"node"`
	Entries []nodeAccessEntry `json:"entries"`
}

func ExportPrimaryNodeAccess(c echo.Context) error {
	return newPrimaryNodeControl().exportAccess(c)
}

func ImportPrimaryNodeAccess(c echo.Context) error {
	return newPrimaryNodeControl().importAccess(c)
}

func (p primaryNodeControl) exportAccess(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	node, err := p.primary()
	if err != nil {
		return primaryNodeError("No se pudo identificar el nodo principal", err)
	}
	entries, err := p.access(node)
	if err != nil {
		return primaryNodeError("No se pudieron exportar las listas", err)
	}
	if len(entries) > maxAccessTransferEntries {
		return apiError(http.StatusConflict, "Las listas superan el límite de 1000 entradas por archivo")
	}
	for _, entry := range entries {
		if err := validateNodeAccess(entry, node); err != nil {
			return apiError(http.StatusConflict, "Las listas contienen entradas incompatibles con el formato de importación. Corrígelas antes de exportar")
		}
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="listas-acceso-`+node+`.json"`)
	return c.JSON(http.StatusOK, nodeAccessDocument{"amerimon-node-access", 1, node, entries})
}

func (p primaryNodeControl) importAccess(c echo.Context) error {
	var req struct {
		Primary  string             `json:"primary_node"`
		Document nodeAccessDocument `json:"document"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, maxAccessTransferBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return apiError(http.StatusBadRequest, "Archivo JSON inválido o mayor de 1 MiB")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return apiError(http.StatusBadRequest, "El archivo debe contener un único documento JSON")
	}
	doc := req.Document
	if doc.Format != "amerimon-node-access" || doc.Version != 1 || !primaryNodeNumber.MatchString(doc.Node) ||
		doc.Entries == nil || len(doc.Entries) > maxAccessTransferEntries {
		return apiError(http.StatusBadRequest, "Formato de listas inválido: se requiere versión 1 y hasta 1000 entradas")
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
	seen := map[string]bool{}
	for i := range doc.Entries {
		entry := &doc.Entries[i]
		entry.Node = strings.ToUpper(strings.TrimSpace(entry.Node))
		entry.Reason = strings.TrimSpace(entry.Reason)
		if err := validateNodeAccess(*entry, node); err != nil {
			return apiError(http.StatusBadRequest, "Entrada de importación inválida: "+err.Error())
		}
		key := entry.Type + ":" + entry.Node
		if seen[key] {
			return apiError(http.StatusBadRequest, "El archivo contiene entradas duplicadas en una misma lista")
		}
		seen[key] = true
	}
	current, err := p.access(node)
	if err != nil {
		return primaryNodeError("No se pudieron leer las listas antes de importar", err)
	}
	original := map[string]nodeAccessEntry{}
	for _, entry := range current {
		original[entry.Type+":"+entry.Node] = entry
	}
	applied := make([]nodeAccessEntry, 0, len(doc.Entries))
	rollback := func(cause error) error {
		failed := false
		for i := len(applied) - 1; i >= 0; i-- {
			entry := applied[i]
			var err error
			if old, exists := original[entry.Type+":"+entry.Node]; exists {
				err = p.putAccess(node, old)
			} else {
				err = p.removeAccess(node, entry)
			}
			if err != nil {
				failed = true
			}
		}
		if failed {
			return primaryNodeError("Falló la importación y no se pudieron restaurar todas las entradas. Recarga y revisa ambas listas", cause)
		}
		return primaryNodeError("No se pudo importar; se restauraron las entradas anteriores", cause)
	}
	for _, entry := range doc.Entries {
		if old, exists := original[entry.Type+":"+entry.Node]; exists && old.Reason == entry.Reason {
			continue
		}
		applied = append(applied, entry)
		if err := p.putAccess(node, entry); err != nil {
			return rollback(err)
		}
	}
	result, err := p.access(node)
	if err != nil {
		return rollback(err)
	}
	actual := map[string]string{}
	for _, entry := range result {
		actual[entry.Type+":"+entry.Node] = entry.Reason
	}
	for _, entry := range doc.Entries {
		reason, exists := actual[entry.Type+":"+entry.Node]
		if !exists || reason != entry.Reason {
			return rollback(errors.New("AstDB no confirmó las entradas importadas"))
		}
	}
	log.Printf("primary node: listas importadas nodo=%s entradas=%d", node, len(doc.Entries))
	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "imported": len(doc.Entries)})
}
