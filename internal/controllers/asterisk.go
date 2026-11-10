package controllers

// Este archivo implementa las estructuras y utilidades de comunicación AMI con Asterisk.

import (
	"amerimon/internal/config"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// AMIRequest representa la estructura de solicitud para comandos AMI.
type AMIRequest struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Secret   string `json:"secret"`
	Cmd      string `json:"cmd"`
}

// AMIResponse representa la estructura de respuesta de un comando AMI.
type AMIResponse struct {
	Status  string   `json:"status"`
	Command string   `json:"command"`
	Lines   []string `json:"lines"`
	Message string   `json:"message,omitempty"`
}

// SendAsteriskCmd valida una petición HTTP, ejecuta su comando AMI y devuelve el resultado.
// SendAsteriskCmd godoc
// @Summary Enviar un comando a Asterisk
// @Tags Asterisk
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body AMIRequest true "Conexión y comando AMI"
// @Success 200 {object} AMIResponse
// @Failure 400 {object} AMIResponse
// @Failure 401
// @Router /asterisk [post]
func SendAsteriskCmd(c echo.Context) (err error) {
	var req AMIRequest

	// 1. Vincular (bind) el cuerpo del JSON recibido al struct
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, AMIResponse{
			Status:  "error",
			Message: "Formato JSON inválido: " + err.Error(),
		})
	}

	// 2. Validaciones básicas de campos requeridos
	if req.Host == "" || req.Port == "" || req.Username == "" || req.Secret == "" || req.Cmd == "" {
		return c.JSON(http.StatusBadRequest, AMIResponse{
			Status:  "error",
			Message: "Todos los campos (host, port, username, secret, cmd) son obligatorios",
		})
	}

	// 3. Ejecutar el comando AMI pasándole un timeout prudente
	rawOutput, err := config.ExecuteAMICommand(req.Host, req.Port, req.Username, req.Secret, req.Cmd, 5*time.Second)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, AMIResponse{
			Status:  "error",
			Command: req.Cmd,
			Message: err.Error(),
		})
	}

	// 4. Convertir la salida cruda al slice de líneas []string
	lines := config.ParseAMIOutputToLines(rawOutput)

	if len(lines) == 0 {
		lines = append(lines, "No data returned")
	}

	// 5. Responder con JSON de éxito
	return c.JSON(http.StatusOK, AMIResponse{
		Status:  "success",
		Command: req.Cmd,
		Lines:   lines,
		Message: "Comando ejecutado correctamente",
	})

}
