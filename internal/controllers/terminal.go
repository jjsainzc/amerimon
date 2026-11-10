package controllers

// Este archivo implementa las conexiones de terminal y la administración de hosts conocidos.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const terminalMessageLimit = 64 * 1024

var terminalKnownHostsMu sync.Mutex

// terminalUpgrader configura la actualización de conexiones HTTP a WebSocket para la terminal.
var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		parsed, err := url.Parse(origin)
		return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") &&
			strings.EqualFold(parsed.Host, r.Host)
	},
}

// terminalConnectionRequest representa la solicitud de conexión SSH desde el cliente WebSocket, incluyendo host, puerto, usuario y método de autenticación.
type terminalConnectionRequest struct {
	Type          string `json:"type"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	AuthMethod    string `json:"authMethod"`
	Password      string `json:"password"`
	PrivateKey    string `json:"privateKey"`
	KeyPassphrase string `json:"keyPassphrase"`
	Cols          int    `json:"cols"`
	Rows          int    `json:"rows"`
}

// terminalMessage representa un mensaje enviado o recibido a través del WebSocket de la terminal, incluyendo tipo, datos y dimensiones del terminal.
type terminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// terminalHostKeyConfirmation representa la confirmación del usuario sobre si confiar en la clave del host SSH desconocido.
type terminalHostKeyConfirmation struct {
	Type  string `json:"type"`
	Trust bool   `json:"trust"`
}

// unknownTerminalHostKey representa una clave de host SSH desconocida.
type unknownTerminalHostKey struct {
	host string
	key  ssh.PublicKey
}

// terminalHostAddress representa la dirección de un host SSH.
type terminalHostAddress string

// Network y String implementan la interfaz net.Addr para terminalHostAddress.
func (address terminalHostAddress) Network() string { return "tcp" }

// String devuelve la representación en cadena de la dirección del host SSH.
func (address terminalHostAddress) String() string { return string(address) }

// Error implementa la interfaz error para unknownTerminalHostKey, indicando que la clave del host SSH no es confiable.
func (e *unknownTerminalHostKey) Error() string {
	return "SSH server host key is not trusted"
}

// HandleTerminalConnection crea un PTY SSH autenticado a través del servidor WebSocket existente.
func HandleTerminalConnection(c echo.Context) error {
	ws, err := terminalUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	ws.SetReadLimit(terminalMessageLimit)
	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Minute))

	messageType, payload, err := ws.ReadMessage()
	if err != nil || messageType != websocket.TextMessage {
		return writeTerminalError(ws, "No se recibieron los datos de conexión SSH.")
	}
	var request terminalConnectionRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return writeTerminalError(ws, "La configuración SSH no es válida.")
	}
	request.Host = strings.TrimSpace(request.Host)
	request.Username = strings.TrimSpace(request.Username)
	if err := validateTerminalConnectionRequest(request); err != nil {
		return writeTerminalError(ws, err.Error())
	}

	hostKeyCallback, knownHostsPath, err := terminalHostKeyCallback()
	if err != nil {
		log.Printf("[Terminal] No se pudo preparar known_hosts: %v", err)
		return writeTerminalError(ws, fmt.Sprintf(
			"No se pudo preparar ~/.ssh/known_hosts para el usuario del servicio: %v", err,
		))
	}
	authMethod, err := terminalSSHAuth(request)
	if err != nil {
		log.Printf("[Terminal] No se pudo preparar la autenticación SSH: %v", err)
		return writeTerminalError(ws, terminalAuthPreparationError(err, request.AuthMethod))
	}

	address := net.JoinHostPort(request.Host, strconv.Itoa(request.Port))
	client, err := dialTerminalSSH(address, request.Username, authMethod, hostKeyCallback)
	if err != nil {
		var untrusted *unknownTerminalHostKey
		if !errors.As(err, &untrusted) {
			log.Printf("[Terminal] Falló el handshake SSH con %s: %v", address, err)
			return writeTerminalError(ws, terminalConnectionError(err, request))
		}
		if err := ws.WriteJSON(map[string]string{
			"type":        "host-key-required",
			"host":        untrusted.host,
			"algorithm":   untrusted.key.Type(),
			"fingerprint": ssh.FingerprintSHA256(untrusted.key),
		}); err != nil {
			return nil
		}
		_ = ws.SetReadDeadline(time.Now().Add(2 * time.Minute))
		messageType, payload, readErr := ws.ReadMessage()
		if readErr != nil || messageType != websocket.TextMessage {
			return writeTerminalError(ws, "No se confirmó la clave del servidor SSH.")
		}
		var confirmation terminalHostKeyConfirmation
		if json.Unmarshal(payload, &confirmation) != nil ||
			confirmation.Type != "trust-host-key" || !confirmation.Trust {
			return writeTerminalError(ws, "No se confirmó la clave del servidor SSH.")
		}
		if err := saveTerminalHostKey(knownHostsPath, untrusted.host, untrusted.key); err != nil {
			log.Printf("[Terminal] No se pudo guardar la clave SSH en %s: %v", knownHostsPath, err)
			return writeTerminalError(ws, "No se pudo guardar la clave en ~/.ssh/known_hosts.")
		}
		hostKeyCallback, _, err = terminalHostKeyCallback()
		if err != nil {
			log.Printf("[Terminal] No se pudo recargar known_hosts: %v", err)
			return writeTerminalError(ws, "No se pudo leer la clave recién guardada.")
		}
		client, err = dialTerminalSSH(address, request.Username, authMethod, hostKeyCallback)
		if err != nil {
			log.Printf("[Terminal] Falló la reconexión SSH con %s: %v", address, err)
			return writeTerminalError(ws, terminalConnectionError(err, request))
		}
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		log.Printf("[Terminal] No se pudo crear una sesión SSH en %s: %v", address, err)
		return writeTerminalError(ws, "No se pudo abrir una sesión SSH.")
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return writeTerminalError(ws, "No se pudo abrir la entrada de la terminal SSH.")
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return writeTerminalError(ws, "No se pudo abrir la salida de la terminal SSH.")
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return writeTerminalError(ws, "No se pudo abrir la salida de errores de la terminal SSH.")
	}

	cols, rows := validTerminalSize(request.Cols, request.Rows)
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		log.Printf("[Terminal] No se pudo solicitar PTY en %s: %v", address, err)
		return writeTerminalError(ws, "El servidor SSH no permitió abrir una terminal.")
	}
	if err := session.Shell(); err != nil {
		log.Printf("[Terminal] No se pudo iniciar el shell en %s: %v", address, err)
		return writeTerminalError(ws, "No se pudo iniciar el shell remoto.")
	}
	go func() {
		if err := session.Wait(); err != nil {
			log.Printf("[Terminal] Finalizó el shell SSH en %s: %v", address, err)
		}
	}()
	_ = ws.SetReadDeadline(time.Time{})
	if err := ws.WriteJSON(map[string]string{"type": "status", "status": "connected"}); err != nil {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go readTerminalInput(ws, stdin, session, cancel)

	output := make(chan []byte, 32)
	var readers sync.WaitGroup
	readers.Add(2)
	go forwardTerminalOutput(ctx, stdout, output, &readers)
	go forwardTerminalOutput(ctx, stderr, output, &readers)
	go func() {
		readers.Wait()
		close(output)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case chunk, ok := <-output:
			if !ok {
				_ = ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := ws.WriteJSON(map[string]string{"type": "status", "status": "disconnected"}); err != nil {
					return nil
				}
				if err := ws.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					time.Now().Add(5*time.Second),
				); err != nil {
					log.Printf("[Terminal] No se pudo enviar cierre normal del WebSocket: %v", err)
				}
				return nil
			}
			_ = ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := ws.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
				return nil
			}
		}
	}
}

// dialTerminalSSH establece una conexión SSH con el host especificado utilizando el nombre de usuario y el método de autenticación proporcionados.
func dialTerminalSSH(address, username string, auth ssh.AuthMethod, hostKeyCallback ssh.HostKeyCallback) (*ssh.Client, error) {
	tcpConn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir una conexión SSH a %s: %w", address, err)
	}
	_ = tcpConn.SetDeadline(time.Now().Add(15 * time.Second))

	clientConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}
	sshConn, channels, requests, err := ssh.NewClientConn(tcpConn, address, clientConfig)
	if err != nil {
		_ = tcpConn.Close()
		return nil, err
	}
	_ = tcpConn.SetDeadline(time.Time{})
	return ssh.NewClient(sshConn, channels, requests), nil
}

// terminalConnectionError devuelve un mensaje de error legible para el usuario basado en el error de conexión SSH y la solicitud de conexión.
func terminalConnectionError(err error, request terminalConnectionRequest) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "El backend no pudo resolver el host SSH. Comprueba el nombre del host y el DNS disponible desde el servidor Amerimon."
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "El host rechazó la conexión SSH. Comprueba que sshd esté activo y escuchando en ese puerto desde la red interna."
	}
	var timeoutErr net.Error
	if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
		return "La conexión SSH desde el backend agotó el tiempo de espera. Comprueba la ruta de red interna y el firewall entre servidores."
	}
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return "El servidor Amerimon no puede alcanzar ese host y puerto SSH. No es necesario exponer SSH a Internet; debe ser accesible desde la red del backend."
	}
	var keyErr *knownhosts.KeyError
	if errors.As(err, &keyErr) && len(keyErr.Want) > 0 {
		return "La clave SSH del servidor cambió y no coincide con known_hosts; se rechazó la conexión."
	}
	var revokedErr *knownhosts.RevokedError
	if errors.As(err, &revokedErr) {
		return "La clave SSH de este servidor está revocada y se rechazó la conexión."
	}
	if strings.Contains(err.Error(), "unable to authenticate") {
		if request.AuthMethod == "publickey" {
			return fmt.Sprintf(
				"El servidor %s rechazó la clave pública para el usuario %q. Comprueba que la clave privada seleccionada corresponda a una clave pública autorizada para ese usuario en el servidor destino.",
				request.Host, request.Username,
			)
		}
		return fmt.Sprintf(
			"El servidor %s rechazó la contraseña para el usuario %q. Comprueba el usuario remoto y que sshd permita autenticación por contraseña.",
			request.Host, request.Username,
		)
	}
	return "Falló la autenticación SSH o la verificación de la clave del servidor."
}

// terminalAuthPreparationError devuelve un mensaje de error legible para el usuario basado en el error de preparación de autenticación SSH y el método de autenticación.
func terminalAuthPreparationError(err error, authMethod string) string {
	var passphraseErr *ssh.PassphraseMissingError
	if errors.As(err, &passphraseErr) {
		return "La clave privada está protegida con frase de contraseña. Escríbela en el campo «Frase de clave»."
	}
	if authMethod == "publickey" {
		return "No se pudo leer la clave privada seleccionada. Comprueba que sea una clave SSH válida y, si está cifrada, introduce su frase de clave."
	}
	return "No se pudo preparar la autenticación SSH."
}

// validateTerminalConnectionRequest valida los campos de la solicitud de conexión SSH y devuelve un error si alguno no es válido.
func validateTerminalConnectionRequest(request terminalConnectionRequest) error {
	if request.Type != "connect" {
		return errors.New("El primer mensaje debe iniciar una conexión SSH.")
	}
	host := strings.TrimSpace(request.Host)
	if host == "" || strings.ContainsAny(host, " \t\r\n/") {
		return errors.New("El host SSH no es válido.")
	}
	if request.Port < 1 || request.Port > 65535 {
		return errors.New("El puerto SSH debe estar entre 1 y 65535.")
	}
	if username := strings.TrimSpace(request.Username); username == "" ||
		len(username) > 64 || strings.ContainsAny(username, " \t\r\n") {
		return errors.New("El usuario SSH no es válido.")
	}
	switch request.AuthMethod {
	case "password":
		if request.Password == "" || len(request.Password) > 4096 {
			return errors.New("Introduce una contraseña SSH válida.")
		}
	case "publickey":
		if strings.TrimSpace(request.PrivateKey) == "" || len(request.PrivateKey) > terminalCredentialMaxKey ||
			len(request.KeyPassphrase) > 4096 {
			return errors.New("Selecciona una clave privada SSH válida.")
		}
	default:
		return errors.New("El método de autenticación SSH no es válido.")
	}
	return nil
}

// validTerminalSize ajusta las dimensiones del terminal a valores válidos si están fuera de los límites permitidos.
func validTerminalSize(cols, rows int) (int, int) {
	if cols < 2 || cols > 500 {
		cols = 80
	}
	if rows < 2 || rows > 300 {
		rows = 24
	}
	return cols, rows
}

// terminalSSHAuth devuelve el método de autenticación SSH correspondiente a la solicitud de conexión.
func terminalSSHAuth(request terminalConnectionRequest) (ssh.AuthMethod, error) {
	if request.AuthMethod == "password" {
		return ssh.Password(request.Password), nil
	}
	var signer ssh.Signer
	var err error
	if request.KeyPassphrase == "" {
		signer, err = ssh.ParsePrivateKey([]byte(request.PrivateKey))
	} else {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(request.PrivateKey), []byte(request.KeyPassphrase))
	}
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

// terminalHostKeyCallback devuelve un callback de verificación de clave de host SSH y la ruta al archivo known_hosts correspondiente.
func terminalHostKeyCallback() (ssh.HostKeyCallback, string, error) {
	home, err := terminalUserHomeDir()
	if err != nil {
		return nil, "", fmt.Errorf("no se pudo determinar el directorio HOME del usuario del servicio: %w", err)
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return nil, "", fmt.Errorf("no se pudo crear %s: %w", sshDir, err)
	}
	if err := os.Chmod(sshDir, 0700); err != nil {
		return nil, "", fmt.Errorf("no se pudieron proteger los permisos de %s: %w", sshDir, err)
	}
	knownHostsPath := filepath.Join(sshDir, "known_hosts")
	file, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, "", fmt.Errorf("no se pudo abrir %s: %w", knownHostsPath, err)
	}
	if err := file.Close(); err != nil {
		return nil, "", fmt.Errorf("no se pudo cerrar %s: %w", knownHostsPath, err)
	}
	if err := os.Chmod(knownHostsPath, 0600); err != nil {
		return nil, "", fmt.Errorf("no se pudieron proteger los permisos de %s: %w", knownHostsPath, err)
	}

	paths := []string{knownHostsPath}
	if _, err := os.Stat("/etc/ssh/ssh_known_hosts"); err == nil {
		paths = append(paths, "/etc/ssh/ssh_known_hosts")
	} else if !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("no se pudo acceder a /etc/ssh/ssh_known_hosts: %w", err)
	}
	callback, err := knownhosts.New(paths...)
	if err != nil {
		return nil, "", fmt.Errorf("no se pudo cargar known_hosts desde %s: %w", strings.Join(paths, ", "), err)
	}
	hostKeyCallback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := callback(hostname, remote, key)
		if err == nil {
			return nil
		}
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) && len(keyErr.Want) == 0 {
			return &unknownTerminalHostKey{host: hostname, key: key}
		}
		return err
	}
	return hostKeyCallback, knownHostsPath, nil
}

// terminalUserHomeDir devuelve el directorio HOME del usuario del servicio, o un error si no se puede determinar.
func terminalUserHomeDir() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	currentUser, err := user.Current()
	if err != nil {
		return "", err
	}
	if currentUser.HomeDir == "" {
		return "", errors.New("el usuario del servicio no tiene un directorio HOME")
	}
	return currentUser.HomeDir, nil
}

// readTerminalInput lee los mensajes del cliente WebSocket y los procesa como entrada de terminal o cambios de tamaño, manejando la cancelación del contexto y errores de lectura.
func saveTerminalHostKey(path, hostname string, key ssh.PublicKey) error {
	terminalKnownHostsMu.Lock()
	defer terminalKnownHostsMu.Unlock()

	callback, err := knownhosts.New(path)
	if err != nil {
		return err
	}
	normalizedHost := knownhosts.Normalize(hostname)
	err = callback(hostname, terminalHostAddress(hostname), key)
	if err == nil {
		return nil
	}
	var keyErr *knownhosts.KeyError
	if !errors.As(err, &keyErr) || len(keyErr.Want) != 0 {
		return err
	}
	line := knownhosts.Line([]string{normalizedHost}, key)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(file, line); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// saveTerminalHostKey guarda la clave de host SSH en el archivo known_hosts especificado, evitando duplicados y manejando errores de clave desconocida.
func readTerminalInput(ws *websocket.Conn, stdin io.Writer, session *ssh.Session, cancel context.CancelFunc) {
	defer cancel()
	for {
		messageType, payload, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			continue
		}
		var message terminalMessage
		if err := json.Unmarshal(payload, &message); err != nil {
			log.Printf("[Terminal] Mensaje de terminal inválido: %v", err)
			return
		}
		switch message.Type {
		case "input":
			if len(message.Data) > terminalMessageLimit {
				return
			}
			if _, err := io.WriteString(stdin, message.Data); err != nil {
				return
			}
		case "resize":
			cols, rows := validTerminalSize(message.Cols, message.Rows)
			if err := session.WindowChange(rows, cols); err != nil {
				log.Printf("[Terminal] No se pudo cambiar el tamaño de la terminal: %v", err)
				return
			}
		default:
			log.Printf("[Terminal] Tipo de mensaje de terminal desconocido: %q", message.Type)
			return
		}
	}
}

// forwardTerminalOutput lee la salida del terminal SSH y la envía al canal de salida, manejando la cancelación del contexto y errores de lectura.
func forwardTerminalOutput(ctx context.Context, reader io.Reader, output chan<- []byte, readers *sync.WaitGroup) {
	defer readers.Done()
	buffer := make([]byte, 4096)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			chunk := append([]byte(nil), buffer[:count]...)
			select {
			case output <- chunk:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("[Terminal] Error leyendo la salida SSH: %v", err)
			}
			return
		}
	}
}

// writeTerminalError envía un mensaje de error al cliente WebSocket.
func writeTerminalError(ws *websocket.Conn, message string) error {
	if err := ws.WriteJSON(map[string]string{"type": "error", "message": message}); err != nil {
		return err
	}
	return nil
}
