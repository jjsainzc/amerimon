package controllers

import (
	"bufio"
	"net/http"
	"os"
	"amerimon/internal/config"
	"strings"

	"github.com/labstack/echo/v4"
)

// AstDBEntry representa una linea del archivo astdb.txt.
// Formato: nodo|callsign|descripcion|ubicacion
type AstDBEntry struct {
	Nodo        string `json:"nodo"`
	Callsign    string `json:"callsign"`
	Descripcion string `json:"descripcion"`
	Ubicacion   string `json:"ubicacion"`
}

// GetAstDB lee /var/lib/asterisk/astdb.txt y devuelve las entradas que
// coinciden con el filtro recibido. Acepta busqueda por "nodo" o por
// "callsign" mediante query params.
//
// Ejemplos:
//
//	GET /mon/astdb?nodo=61627
//	GET /mon/astdb?callsign=HC1LNM
//
// La busqueda por nodo es exacta; la busqueda por callsign es parcial e
// insensible a mayusculas/minusculas.
// GetAstDB godoc
// @Summary Consultar la base de datos AST
// @Tags Radio
// @Produce json
// @Param nodo query string false "Filtrar por número de nodo"
// @Param callsign query string false "Filtrar por indicativo"
// @Success 200 {array} AstDBEntry
// @Router /astdb [get]
func GetAstDB(c echo.Context) error {
	response := make(map[string]any)
	response["status"] = "success"
	response["data"] = ""
	response["message"] = "Consulta realizada correctamente"

	nodo := strings.TrimSpace(c.QueryParam("nodo"))
	callsign := strings.TrimSpace(c.QueryParam("callsign"))

	if nodo == "" && callsign == "" {
		response["status"] = "error"
		response["message"] = "Debe indicar el parametro 'nodo' o 'callsign'"
		return c.JSON(http.StatusBadRequest, response)
	}

	astdbPath := config.App.Sources.AstDBPath
	if astdbPath == "" {
		response["status"] = "error"
		response["message"] = "La ruta de astdb.txt no esta configurada"
		return c.JSON(http.StatusInternalServerError, response)
	}

	archivo, err := os.Open(astdbPath)
	if err != nil {
		response["status"] = "error"
		response["message"] = "No se pudo abrir el archivo astdb.txt"
		return c.JSON(http.StatusInternalServerError, response)
	}
	defer archivo.Close()

	callsignLower := strings.ToLower(callsign)
	resultados := make([]AstDBEntry, 0)

	scanner := bufio.NewScanner(archivo)
	// Ampliar el buffer por si alguna linea es larga.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		linea := scanner.Text()
		if strings.TrimSpace(linea) == "" {
			continue
		}

		campos := strings.Split(linea, "|")
		if len(campos) < 2 {
			continue // Linea con formato inesperado
		}

		entrada := AstDBEntry{Nodo: strings.TrimSpace(campos[0])}
		if len(campos) > 1 {
			entrada.Callsign = strings.TrimSpace(campos[1])
		}
		if len(campos) > 2 {
			entrada.Descripcion = strings.TrimSpace(campos[2])
		}
		if len(campos) > 3 {
			entrada.Ubicacion = strings.TrimSpace(campos[3])
		}

		// Filtrado.
		if nodo != "" {
			// Busqueda exacta por numero de nodo.
			if entrada.Nodo == nodo {
				resultados = append(resultados, entrada)
			}
			continue
		}

		// Busqueda parcial por callsign (insensible a mayusculas).
		if callsign != "" && strings.Contains(strings.ToLower(entrada.Callsign), callsignLower) {
			resultados = append(resultados, entrada)
		}
	}

	if err := scanner.Err(); err != nil {
		response["status"] = "error"
		response["message"] = "Error al leer el archivo astdb.txt"
		return c.JSON(http.StatusInternalServerError, response)
	}

	if len(resultados) == 0 {
		criterio := "callsign '" + callsign + "'"
		if nodo != "" {
			criterio = "nodo '" + nodo + "'"
		}
		response["status"] = "error"
		response["message"] = "No se encontraron entradas para el " + criterio
		return c.JSON(http.StatusNotFound, response)
	}

	response["data"] = map[string]any{
		"count":      len(resultados),
		"resultados": resultados,
	}

	return c.JSON(http.StatusOK, response)
}
