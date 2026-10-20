package config

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

// ExecuteAMICommand se conecta a Asterisk AMI, ejecuta un comando CLI y devuelve su salida.
func ExecuteAMICommand(host, port, username, secret, command string, timeout time.Duration) (string, error) {
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return "", fmt.Errorf("error al conectar: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	reader := bufio.NewReader(conn)

	// Leer banner inicial
	if _, err := reader.ReadString('\n'); err != nil {
		return "", fmt.Errorf("error leyendo banner: %w", err)
	}

	// Login
	loginMsg := fmt.Sprintf("Action: Login\r\nUsername: %s\r\nSecret: %s\r\nEvents: off\r\n\r\n", username, secret)
	if _, err := conn.Write([]byte(loginMsg)); err != nil {
		return "", fmt.Errorf("error enviando login: %w", err)
	}

	loginResponse, err := readAMIMessage(reader)
	if err != nil {
		return "", fmt.Errorf("error en respuesta de login: %w", err)
	}
	if !strings.Contains(loginResponse, "Response: Success") {
		return "", fmt.Errorf("login AMI rechazado: %s", strings.Join(ParseAMIOutputToLines(loginResponse), " "))
	}

	// Enviar comando CLI
	cmdMsg := fmt.Sprintf("Action: Command\r\nCommand: %s\r\n\r\n", command)
	if _, err := conn.Write([]byte(cmdMsg)); err != nil {
		return "", fmt.Errorf("error enviando comando: %w", err)
	}

	// Leer toda la respuesta de la AMI
	rawOutput, err := readFullResponse(reader)
	if err != nil {
		return "", fmt.Errorf("error leyendo respuesta: %w", err)
	}
	if err := amiResponseError(rawOutput); err != nil {
		return "", fmt.Errorf("comando AMI %q rechazado: %w", command, err)
	}

	_, _ = conn.Write([]byte("Action: Logoff\r\n\r\n"))

	return rawOutput, nil
}

// amiResponseError devuelve el mensaje de Asterisk cuando la respuesta AMI indica un error.
func amiResponseError(raw string) error {
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Response: Error") {
			return fmt.Errorf("%s", strings.Join(ParseAMIOutputToLines(raw), " "))
		}
	}
	return nil
}

// ExecuteAMIRptStatus solicita XStat o SawStat para un nodo local de AllStar mediante AMI.
func ExecuteAMIRptStatus(host, port, username, secret, node, command string, timeout time.Duration) (string, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return "", fmt.Errorf("error al conectar: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	reader := bufio.NewReader(conn)
	if _, err := reader.ReadString('\n'); err != nil {
		return "", fmt.Errorf("error leyendo banner: %w", err)
	}

	loginMsg := fmt.Sprintf("Action: Login\r\nUsername: %s\r\nSecret: %s\r\nEvents: off\r\n\r\n", username, secret)
	if _, err := conn.Write([]byte(loginMsg)); err != nil {
		return "", fmt.Errorf("error enviando login: %w", err)
	}
	loginResponse, err := readAMIMessage(reader)
	if err != nil {
		return "", fmt.Errorf("error en respuesta de login: %w", err)
	}
	if !strings.Contains(loginResponse, "Authentication accepted") {
		return "", fmt.Errorf("login AMI rechazado: %s", strings.Join(ParseAMIOutputToLines(loginResponse), " "))
	}

	actionID := fmt.Sprintf("rpt-status-%d", time.Now().UnixNano())
	statusMsg := fmt.Sprintf("Action: RptStatus\r\nCommand: %s\r\nNode: %s\r\nActionID: %s\r\n\r\n", command, node, actionID)
	if _, err := conn.Write([]byte(statusMsg)); err != nil {
		return "", fmt.Errorf("error enviando RptStatus: %w", err)
	}

	rawOutput, err := readAMIActionResponse(reader, actionID)
	if err != nil {
		return "", fmt.Errorf("error leyendo respuesta de RptStatus: %w", err)
	}
	_, _ = conn.Write([]byte("Action: Logoff\r\n\r\n"))

	if strings.Contains(rawOutput, "Response: Error") {
		return "", fmt.Errorf("RptStatus rechazado: %s", strings.Join(ParseAMIOutputToLines(rawOutput), " "))
	}
	return rawOutput, nil
}

// ExecuteAMIOriginate inicia una llamada mediante Action: Originate de AMI.
func ExecuteAMIOriginate(host, port, username, secret, channel, context, exten string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
	if err != nil {
		return fmt.Errorf("error al conectar: %w", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	reader := bufio.NewReader(conn)

	if _, err := reader.ReadString('\n'); err != nil {
		return fmt.Errorf("error leyendo banner: %w", err)
	}

	login := fmt.Sprintf("Action: Login\r\nUsername: %s\r\nSecret: %s\r\nEvents: off\r\n\r\n", username, secret)
	if _, err := conn.Write([]byte(login)); err != nil {
		return fmt.Errorf("error enviando login: %w", err)
	}
	if err := discardUntilBlank(reader); err != nil {
		return fmt.Errorf("error en respuesta de login: %w", err)
	}

	orig := fmt.Sprintf("Action: Originate\r\nChannel: %s\r\nContext: %s\r\nExten: %s\r\nPriority: 1\r\nAsync: true\r\n\r\n", channel, context, exten)
	if _, err := conn.Write([]byte(orig)); err != nil {
		return fmt.Errorf("error enviando originate: %w", err)
	}

	raw, err := readFullResponse(reader)
	if err != nil {
		return fmt.Errorf("error leyendo respuesta: %w", err)
	}
	_, _ = conn.Write([]byte("Action: Logoff\r\n\r\n"))

	if !strings.Contains(raw, "Response: Success") {
		return fmt.Errorf("originate rechazado: %s", strings.Join(ParseAMIOutputToLines(raw), " "))
	}
	return nil
}

// discardUntilBlank consume líneas hasta el separador en blanco de un mensaje AMI.
func discardUntilBlank(reader *bufio.Reader) error {
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}
	return nil
}

// readAMIMessage lee y devuelve un mensaje AMI delimitado por una línea en blanco.
func readAMIMessage(reader *bufio.Reader) (string, error) {
	var message strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		if line == "\r\n" || line == "\n" {
			return message.String(), nil
		}
		message.WriteString(line)
	}
}

// readAMIActionResponse ignora mensajes AMI hasta encontrar el ActionID solicitado.
func readAMIActionResponse(reader *bufio.Reader, actionID string) (string, error) {
	for {
		message, err := readAMIMessage(reader)
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(message, "\n") {
			if strings.TrimSpace(line) == "ActionID: "+actionID {
				return message, nil
			}
		}
	}
}

// readFullResponse lee la respuesta completa de un comando AMI hasta detectar su final.
func readFullResponse(reader *bufio.Reader) (string, error) {
	var sb strings.Builder
	hasStartedOutput := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		// Delimitador para versiones antiguas de Asterisk
		if strings.Contains(line, "--END COMMAND--") {
			break
		}

		// En Asterisk >= 16, detectar que ya comenzaron a llegar líneas de Output
		if strings.HasPrefix(line, "Output:") {
			hasStartedOutput = true
		}

		sb.WriteString(line)

		// Una línea en blanco sólo marca el fin si YA leímos datos del comando (Output)
		// O si es la línea en blanco que cierra la respuesta sin output
		if line == "\r\n" || line == "\n" {
			if hasStartedOutput {
				break
			}
			// Si no hay "Output:" pero recibimos un Response: Follows (común en ciertos comandos)
			if strings.Contains(sb.String(), "Response: Follows") {
				continue
			}
			// Si la respuesta es directa (ej: Response: Error o Success sin cuerpo)
			if strings.Contains(sb.String(), "Response:") && !strings.Contains(sb.String(), "Response: Follows") {
				break
			}
		}
	}

	return sb.String(), nil
}

// ParseAMIOutputToLines limpia una respuesta AMI y devuelve solo sus líneas de salida.
func ParseAMIOutputToLines(raw string) []string {
	clean := strings.ReplaceAll(raw, "\r\n", "\n")
	clean = strings.ReplaceAll(clean, "\r", "\n")

	rawLines := strings.Split(clean, "\n")
	var lines []string

	for _, line := range rawLines {
		line = strings.TrimSpace(line)

		if line == "" ||
			strings.HasPrefix(line, "Response:") ||
			strings.HasPrefix(line, "Privilege:") ||
			strings.HasPrefix(line, "ActionID:") ||
			strings.Contains(line, "--END COMMAND--") {
			continue
		}

		if trimmed, ok := strings.CutPrefix(line, "Output: "); ok {
			line = trimmed
		}

		lines = append(lines, line)
	}

	return lines
}
