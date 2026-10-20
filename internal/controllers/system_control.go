package controllers

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

var systemControlLock sync.Mutex

type systemControlResult struct {
	Action    string   `json:"action"`
	Recovered bool     `json:"recovered"`
	Scheduled bool     `json:"scheduled"`
	Details   []string `json:"details"`
	Warnings  string   `json:"warnings"`
}

type systemControl struct {
	run        func(context.Context, time.Duration, string, ...string) (string, error)
	ami        func() error
	pause      func(time.Duration)
	fileErrors func(time.Time) (string, error)
}

// runSystemCommand ejecuta únicamente comandos y argumentos definidos por el servidor, con plazo limitado.
func runSystemCommand(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, text)
	}
	return text, nil
}

// checkControlAMI exige una respuesta AMI de uptime, no solo que el puerto acepte conexiones.
func checkControlAMI() error {
	ami := config.App.AMI
	response, err := config.ExecuteAMICommand(ami.Host, ami.Port, ami.User, ami.Secret, "core show uptime", 2*time.Second)
	if err != nil {
		return err
	}
	if strings.Contains(response, "Response: Error") ||
		(!strings.Contains(response, "Response: Success") && !strings.Contains(response, "Response: Follows")) {
		return fmt.Errorf("AMI no confirmó uptime: %s", strings.Join(config.ParseAMIOutputToLines(response), " "))
	}
	return nil
}

// ControlSystem restringe mantenimiento al administrador y no acepta comandos ni unidades arbitrarios.
func ControlSystem(c echo.Context) error {
	return (systemControl{
		run: runSystemCommand, ami: checkControlAMI, pause: time.Sleep,
		fileErrors: func(since time.Time) (string, error) {
			return asteriskFileErrors("/var/log/asterisk/messages.log", since)
		},
	}).handle(c)
}

// asteriskFileErrors limita la lectura al final del log estándar y filtra errores posteriores a la solicitud.
func asteriskFileErrors(path string, since time.Time) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	const maxBytes = 256 * 1024
	start := max(int64(0), info.Size()-maxBytes)
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	reader := bufio.NewReader(io.LimitReader(file, maxBytes))
	if start > 0 {
		if _, err := reader.ReadString('\n'); err != nil {
			return "", err
		}
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxBytes)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, " ERROR[") && !strings.Contains(line, " CRITICAL[") {
			continue
		}
		end := strings.IndexByte(line, ']')
		if !strings.HasPrefix(line, "[") || end < 0 {
			return "", fmt.Errorf("formato de fecha no reconocido en %s", path)
		}
		stamp, err := time.ParseInLocation("2006-01-02 15:04:05.999999999", line[1:end], time.Local)
		if err != nil {
			return "", fmt.Errorf("fecha de log inválida en %s: %w", path, err)
		}
		if !stamp.Before(since.Truncate(time.Second)) {
			lines = append(lines, line)
			if len(lines) > 40 {
				lines = lines[1:]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}

// handle confirma, serializa y registra cada operación; un apagado solo se informa como programado.
func (s systemControl) handle(c echo.Context) error {
	claims, err := GetClaims(c)
	if err != nil {
		return err
	}
	if claims.Role != "A" {
		return echo.NewHTTPError(http.StatusForbidden, "Se requieren privilegios de administrador")
	}
	var request struct {
		Action       string `json:"action"`
		Confirmation string `json:"confirmation"`
	}
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "message": "Solicitud inválida"})
	}
	confirmations := map[string]string{
		"asterisk_restart": "REINICIAR ASTERISK",
		"linux_reboot":     "REINICIAR LINUX",
		"linux_poweroff":   "APAGAR LINUX",
	}
	expected, ok := confirmations[request.Action]
	if !ok || request.Confirmation != expected {
		return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "message": "Acción o confirmación inválida"})
	}
	if !systemControlLock.TryLock() {
		return c.JSON(http.StatusConflict, echo.Map{"status": "error", "message": "Ya hay una operación de mantenimiento en curso"})
	}
	defer systemControlLock.Unlock()
	started := time.Now()
	// Una desconexión del navegador no debe dejar una recuperación a medias.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request().Context()), 55*time.Second)
	defer cancel()
	result := systemControlResult{Action: request.Action, Details: []string{}}
	log.Printf("system control: solicitado acción=%s administrador=%d ip=%s", request.Action, claims.UserID, c.RealIP())
	recordAsteriskOperation(ctx, request.Action, "", "", "", false, request.Action, "requested", "Mantenimiento solicitado por administrador", started)
	status := http.StatusOK
	message := ""
	if request.Action == "asterisk_restart" {
		err = s.restartAsterisk(ctx, &result)
		message = "Asterisk reiniciado; servicio activo y AMI operativo."
		warnings, journalErr := s.run(ctx, 3*time.Second, "journalctl", "-u", "asterisk.service", "--since", started.Format(time.RFC3339), "-p", "err", "-n", "40", "--no-pager", "-o", "short-iso")
		if journalErr != nil {
			result.Warnings = "No se pudieron consultar los errores del arranque: " + journalErr.Error()
		} else if warnings != "" && warnings != "-- No entries --" {
			result.Warnings = warnings
		}
		if s.fileErrors != nil {
			fileWarnings, fileErr := s.fileErrors(started)
			if fileErr != nil {
				fileWarnings = "No se pudo consultar /var/log/asterisk/messages.log: " + fileErr.Error()
			}
			if fileWarnings != "" {
				result.Warnings = strings.TrimSpace(result.Warnings + "\n" + fileWarnings)
			}
		}
	} else {
		flag := "-r"
		message = "Reinicio de Linux programado dentro de un minuto. Se perderá la conexión."
		if request.Action == "linux_poweroff" {
			flag = "-P"
			message = "Apagado de Linux programado dentro de un minuto. Para volver a encenderlo puede requerirse acceso físico."
		}
		_, err = s.run(ctx, 5*time.Second, "shutdown", flag, "+1", "Mantenimiento solicitado desde Amerimon")
		result.Scheduled = err == nil
		status = http.StatusAccepted
	}
	outcome := "success"
	if result.Scheduled {
		outcome = "scheduled"
	}
	if err != nil {
		status, outcome, message = http.StatusBadGateway, "system_error", err.Error()
	}
	log.Printf("system control: acción=%s resultado=%s mensaje=%s detalles=%v advertencias=%s", request.Action, outcome, message, result.Details, result.Warnings)
	recordAsteriskOperation(ctx, request.Action, "", "", "", false, request.Action, outcome,
		message+"\n"+strings.Join(result.Details, "\n")+"\n"+result.Warnings, started)
	responseStatus := "success"
	if err != nil {
		responseStatus = "error"
	}
	return c.JSON(status, echo.Map{"status": responseStatus, "message": message, "data": result})
}

// stopped espera a que systemd haya terminado el trabajo de parada antes de intentar start.
func (s systemControl) stopped(ctx context.Context, attempts int) (bool, error) {
	for i := 0; i < attempts; i++ {
		state, err := s.run(ctx, 2*time.Second, "systemctl", "show", "asterisk.service", "--property=ActiveState", "--value")
		if err != nil {
			return false, fmt.Errorf("no se pudo comprobar la parada de Asterisk: %w", err)
		}
		if state == "inactive" || state == "failed" {
			return true, nil
		}
		s.pause(time.Second)
	}
	return false, nil
}

// signalAndWait tolera la salida simultánea del servicio, pero no escala si no puede consultar su estado.
func (s systemControl) signalAndWait(ctx context.Context, signal string, attempts int) (bool, error) {
	_, signalErr := s.run(ctx, 3*time.Second, "systemctl", "kill", "--kill-whom=all", "--signal="+signal, "asterisk.service")
	stopped, err := s.stopped(ctx, attempts)
	if err != nil {
		return false, err
	}
	if !stopped && signalErr != nil {
		return false, fmt.Errorf("falló %s de asterisk.service: %w", signal, signalErr)
	}
	return stopped, nil
}

// restartAsterisk recupera solo asterisk.service; nunca busca ni mata procesos por nombre.
func (s systemControl) restartAsterisk(ctx context.Context, result *systemControlResult) error {
	_, restartErr := s.run(ctx, 20*time.Second, "systemctl", "restart", "asterisk.service")
	if restartErr == nil {
		state, err := s.run(ctx, 2*time.Second, "systemctl", "is-active", "asterisk.service")
		if err != nil || state != "active" {
			restartErr = fmt.Errorf("restart terminó pero el servicio no está activo: %s (%v)", state, err)
		}
	}
	if restartErr != nil {
		result.Details = append(result.Details, "Restart falló: "+restartErr.Error())
		if _, err := s.run(ctx, 3*time.Second, "systemctl", "stop", "--no-block", "asterisk.service"); err != nil {
			return fmt.Errorf("no se pudo solicitar la parada para recuperar Asterisk: %w", err)
		}
		stopped, err := s.stopped(ctx, 1)
		if err != nil {
			return err
		}
		if !stopped {
			stopped, err = s.signalAndWait(ctx, "SIGTERM", 3)
			if err != nil {
				return err
			}
			if !stopped {
				stopped, err = s.signalAndWait(ctx, "SIGKILL", 3)
				if err != nil {
					return err
				}
				result.Details = append(result.Details, "Se requirió SIGKILL para los procesos de asterisk.service.")
				if !stopped {
					return fmt.Errorf("Asterisk sigue en parada; no se inició una segunda instancia")
				}
			}
		}
		if _, err := s.run(ctx, 3*time.Second, "systemctl", "reset-failed", "asterisk.service"); err != nil {
			return err
		}
		if _, err := s.run(ctx, 15*time.Second, "systemctl", "start", "asterisk.service"); err != nil {
			return fmt.Errorf("falló el inicio tras la recuperación: %w", err)
		}
		result.Recovered = true
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if ctx.Err() != nil {
			return fmt.Errorf("se agotó el plazo de verificación: %w", ctx.Err())
		}
		state, err := s.run(ctx, 2*time.Second, "systemctl", "is-active", "asterisk.service")
		if err == nil && state == "active" {
			if lastErr = s.ami(); lastErr == nil {
				result.Details = append(result.Details, "Servicio activo y consulta AMI de uptime correcta.")
				return nil
			}
		} else {
			lastErr = fmt.Errorf("servicio Asterisk no activo: %s (%v)", state, err)
		}
		s.pause(time.Second)
	}
	return fmt.Errorf("no se confirmó Asterisk operativo: %w", lastErr)
}
