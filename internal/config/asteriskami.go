package config

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strings"
	"time"
)

// dialAMI resuelve nombres antes de conectar y comparte el timeout entre DNS y TCP.
func dialAMI(host, port string, timeout time.Duration) (net.Conn, error) {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if host == "" {
		return nil, fmt.Errorf("host AMI vacío")
	}
	ctx := context.Background()
	if timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	dialer := net.Dialer{}
	if ip, err := netip.ParseAddr(host); err == nil {
		return dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), port))
	}

	addresses, err := resolveAMIHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolver host AMI %q: %w", host, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("resolver host AMI %q: no se encontraron direcciones IP", host)
	}
	var failures []error
	for i, address := range addresses {
		// Reparte el tiempo restante para que una IP inaccesible no bloquee las demás.
		if deadline, ok := ctx.Deadline(); ok {
			dialer.Timeout = time.Until(deadline) / time.Duration(len(addresses)-i)
		}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address.String(), port))
		if err == nil {
			return conn, nil
		}
		failures = append(failures, err)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, fmt.Errorf("conectar host AMI %q: %w", host, errors.Join(failures...))
}

func resolveAMIHost(ctx context.Context, host string) ([]net.IPAddr, error) {
	if !strings.HasSuffix(strings.ToLower(strings.TrimSuffix(host, ".")), ".local") {
		return net.DefaultResolver.LookupIPAddr(ctx, host)
	}
	// Los binarios sin cgo no consultan NSS/Avahi; getent sí usa el resolvedor del sistema.
	output, err := exec.CommandContext(ctx, "getent", "ahosts", host).Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("getent ahosts: %w; comprueba avahi-daemon y libnss-mdns", err)
	}
	var addresses []net.IPAddr
	seen := make(map[netip.Addr]bool)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		ip, err := netip.ParseAddr(fields[0])
		if err != nil {
			return nil, fmt.Errorf("getent devolvió una dirección inválida: %q", fields[0])
		}
		if !seen[ip] {
			seen[ip] = true
			addresses = append(addresses, net.IPAddr{IP: net.IP(ip.AsSlice()), Zone: ip.Zone()})
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("getent no devolvió direcciones IP para %q", host)
	}
	return addresses, nil
}

// ExecuteAMICommand se conecta a Asterisk AMI, ejecuta un comando CLI y devuelve su salida.
func ExecuteAMICommand(host, port, username, secret, command string, timeout time.Duration) (string, error) {
	conn, err := dialAMI(host, port, timeout)
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
	conn, err := dialAMI(host, port, timeout)
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

type AMIOriginateVariable struct {
	Name  string
	Value string
}

// ExecuteAMIOriginate inicia una llamada AMI y propaga las variables indicadas al canal.
func ExecuteAMIOriginate(host, port, username, secret, channel, context, exten string, timeout time.Duration, variables ...AMIOriginateVariable) error {
	var variableHeaders strings.Builder
	for _, variable := range variables {
		if variable.Name == "" || strings.ContainsAny(variable.Name, "\r\n=: ") || strings.ContainsAny(variable.Value, "\r\n") {
			return fmt.Errorf("variable Originate inválida")
		}
		fmt.Fprintf(&variableHeaders, "Variable: %s=%s\r\n", variable.Name, variable.Value)
	}
	conn, err := dialAMI(host, port, timeout)
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
	loginResponse, err := readAMIMessage(reader)
	if err != nil {
		return fmt.Errorf("error en respuesta de login: %w", err)
	}
	if !strings.Contains(loginResponse, "Response: Success") {
		return fmt.Errorf("login AMI rechazado: %s", strings.Join(ParseAMIOutputToLines(loginResponse), " "))
	}

	actionID := fmt.Sprintf("listen-live-%d", time.Now().UnixNano())
	orig := fmt.Sprintf("Action: Originate\r\nActionID: %s\r\nChannel: %s\r\nContext: %s\r\nExten: %s\r\nPriority: 1\r\nAsync: true\r\n%s\r\n", actionID, channel, context, exten, variableHeaders.String())
	if _, err := conn.Write([]byte(orig)); err != nil {
		return fmt.Errorf("error enviando originate: %w", err)
	}

	raw, err := readAMIActionResponse(reader, actionID)
	if err != nil {
		return fmt.Errorf("error leyendo respuesta de Originate: %w", err)
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
