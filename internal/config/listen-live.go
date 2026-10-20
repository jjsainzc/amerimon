package config

/**
* Listen Live Configuration en Asterisk
[listen-live]
	; ${EXTEN_TO_SPY} es el canal objetivo y ${UUID} es la clave para mapear en Go
	exten => _X.,1,NoOp(Escuchando canal ${EXTEN})
	same => n,Set(UUID=${ARG1})
	same => n,ChanSpy(PJSIP/${EXTEN},qB) ; q = silencioso, B = barge-in/whisper si fuera necesario
	same => n,AudioSocket(${UUID},127.0.0.1:8086)
	same => n,Hangup()

* via eagi
[listen-live-eagi]
	exten => _.,1,NoOp(EAGI listen-live UUID ${EXTEN})
	same => n,Answer()
	same => n,EAGI(/usr/local/bin/audio_eagi.sh,${EXTEN})
	same => n,Hangup()

* via MixMonitor
[listen-live]
exten => _.,1,NoOp(Escuchando canal via MixMonitor Pipe)
	same => n,MixMonitor(/tmp/asterisk_audio.raw,r(raw)t(raw)) ; Graba en PCM raw
	same => n,ChanSpy(PJSIP/${EXTEN},q)
	same => n,StopMixMonitor()
	same => n,Hangup()

*/

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// ---------------------------------------------------------------------
// 1. ESTRUCTURA Y MÉTODOS DEL GESTOR DE STREAMS (AudioStreamManager)
// ---------------------------------------------------------------------

type AudioStreamManager struct {
	listeners  map[string][]*websocket.Conn
	audioConns map[string]net.Conn
	mu         sync.RWMutex
}

// NewAudioStreamManager crea el registro sincronizado de clientes que escuchan llamadas.
func NewAudioStreamManager() *AudioStreamManager {
	return &AudioStreamManager{
		listeners:  make(map[string][]*websocket.Conn),
		audioConns: make(map[string]net.Conn),
	}
}

// Variantes de captura de audio soportadas.
const (
	ListenModeAudioSocket = "audiosocket"
	ListenModeEAGI        = "eagi"
	ListenModeMixMonitor  = "mixmonitor" // se lee por FIFO con HandleMixMonitorPipe, no usa TCP
)

const (
	websocketPongWait  = 60 * time.Second
	websocketPingEvery = 25 * time.Second
	websocketWriteWait = 10 * time.Second
)

// ListenMode es la variante activa.
var ListenMode = ListenModeEAGI

// StartAudioListener levanta el servidor TCP (bloqueante) que recibe el audio
// de Asterisk según ListenMode. Con MixMonitor no abre TCP.
func StartAudioListener(addr string, mgr *AudioStreamManager) {
	if ListenMode == ListenModeMixMonitor {
		log.Printf("[AudioStream] Modo MixMonitor: sin listener TCP, use HandleMixMonitorPipe por FIFO")
		return
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[AudioStream] Error iniciando listener TCP en %s: %v", addr, err)
	}
	defer listener.Close()

	log.Printf("[AudioStream] Listener TCP (%s) activo en %s", ListenMode, addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[AudioStream] Error aceptando conexión: %v", err)
			continue
		}

		switch ListenMode {
		case ListenModeAudioSocket:
			go HandleAudioSocketConnection(conn, mgr)
		default:
			go HandleEAGIConnection(conn, mgr)
		}
	}
}

// RegisterLiveListenRoutes registra el endpoint del WebSocket en el grupo de API de Echo
func RegisterLiveListenRoutes(g *echo.Group, mgr *AudioStreamManager) {
	// Endpoint: GET /mon/v1/listen-live/ws?uuid=XXXXX
	g.GET("/listen-live/ws", HandleLiveListenWS(mgr))
}

// AddClient registra una conexión WebSocket para recibir el audio de una llamada.
func (m *AudioStreamManager) AddClient(uuid string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners[uuid] = append(m.listeners[uuid], conn)
}

// RemoveClient elimina el WebSocket de la llamada y borra su registro si queda vacío.
func (m *AudioStreamManager) RemoveClient(uuid string, conn *websocket.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	clients := m.listeners[uuid]
	for i, c := range clients {
		if c == conn {
			m.listeners[uuid] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	if len(m.listeners[uuid]) == 0 {
		delete(m.listeners, uuid)
	}
}

// CloseClients cierra los WebSockets asociados cuando termina el stream de audio.
func (m *AudioStreamManager) CloseClients(uuid string) {
	m.mu.Lock()
	clients := m.listeners[uuid]
	delete(m.listeners, uuid)
	m.mu.Unlock()

	closeMessage := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "audio stream ended")
	for _, client := range clients {
		_ = client.WriteControl(websocket.CloseMessage, closeMessage, time.Now().Add(websocketWriteWait))
		_ = client.Close()
	}
}

// AddAudioConnection registra la conexión TCP de audio asociada al UUID.
func (m *AudioStreamManager) AddAudioConnection(uuid string, conn net.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.audioConns == nil {
		m.audioConns = make(map[string]net.Conn)
	}
	m.audioConns[uuid] = conn
}

// RemoveAudioConnection elimina la conexión registrada solo si coincide con conn.
func (m *AudioStreamManager) RemoveAudioConnection(uuid string, conn net.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.audioConns[uuid] == conn {
		delete(m.audioConns, uuid)
	}
}

// CloseAudioConnection quita y cierra la conexión TCP del UUID; devuelve false si no había ninguna.
func (m *AudioStreamManager) CloseAudioConnection(uuid string) bool {
	m.mu.Lock()
	conn, exists := m.audioConns[uuid]
	if exists {
		delete(m.audioConns, uuid)
	}
	m.mu.Unlock()
	if !exists {
		return false
	}
	if err := conn.Close(); err != nil {
		log.Printf("[AudioStream] error cerrando TCP EAGI uuid=%s: %v", uuid, err)
	}
	return true
}

// Broadcast envía el audio a los WebSockets del uuid y devuelve cuántos había.
func (m *AudioStreamManager) Broadcast(uuid string, payload []byte) int {
	m.mu.RLock()
	clients := append([]*websocket.Conn(nil), m.listeners[uuid]...)
	m.mu.RUnlock()

	active := 0
	for _, client := range clients {
		if err := client.SetWriteDeadline(time.Now().Add(websocketWriteWait)); err != nil {
			m.RemoveClient(uuid, client)
			_ = client.Close()
			continue
		}
		if err := client.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			m.RemoveClient(uuid, client)
			_ = client.Close()
			continue
		}
		active++
	}
	return active
}

// Upgrader para el WebSocket
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Configurar orígenes permitidos
	},
}

// ---------------------------------------------------------------------
// 2. HANDLER DE ECHO (Para la conexión WebSocket desde Angular)
// ---------------------------------------------------------------------

// HandleLiveListenWS crea el handler WebSocket que registra al oyente y mantiene su conexión.
// HandleLiveListenWS godoc
// @Summary Abrir el WebSocket de audio en vivo
// @Tags Listen Live
// @Security BearerAuth
// @Param token query string false "JWT alternativo para la conexión WebSocket"
// @Success 101
// @Failure 401
// @Router /listen-live/ws [get]
func HandleLiveListenWS(mgr *AudioStreamManager) echo.HandlerFunc {
	return func(c echo.Context) error {
		callUUID := c.QueryParam("uuid")
		if callUUID == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "uuid es requerido"})
		}

		ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			return err
		}
		defer ws.Close()

		mgr.AddClient(callUUID, ws)
		log.Printf("[ListenLive] WebSocket conectado uuid=%s", callUUID)
		defer log.Printf("[ListenLive] WebSocket cerrado uuid=%s", callUUID)
		defer mgr.RemoveClient(callUUID, ws)
		if err := ws.SetReadDeadline(time.Now().Add(websocketPongWait)); err != nil {
			return err
		}
		ws.SetPongHandler(func(string) error {
			return ws.SetReadDeadline(time.Now().Add(websocketPongWait))
		})

		stopPing := make(chan struct{})
		defer close(stopPing)
		go func() {
			ticker := time.NewTicker(websocketPingEvery)
			defer ticker.Stop()
			for {
				select {
				case <-stopPing:
					return
				case <-ticker.C:
					if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(websocketWriteWait)); err != nil {
						_ = ws.Close()
						return
					}
				}
			}
		}()

		// Mantener la conexión abierta hasta que el cliente se desconecte
		for {
			_, _, err := ws.ReadMessage()
			if err != nil {
				log.Printf("[ListenLive] WebSocket lectura terminada uuid=%s error=%v", callUUID, err)
				break
			}
		}

		return nil
	}
}

// ---------------------------------------------------------------------
// 3. VARIANTE 1: AUDIOSOCKET (Protocolo Nativo de Asterisk)
// ---------------------------------------------------------------------

// HandleAudioSocketConnection procesa tramas AudioSocket de Asterisk y distribuye el audio.
func HandleAudioSocketConnection(conn net.Conn, mgr *AudioStreamManager) {
	defer conn.Close()

	var callUUID string
	defer func() {
		if callUUID != "" {
			mgr.CloseClients(callUUID)
		}
	}()

	for {
		var msgType uint8
		var length uint16

		// Header: 1 byte Type + 2 bytes Length (BigEndian)
		if err := binary.Read(conn, binary.BigEndian, &msgType); err != nil {
			break
		}
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			break
		}

		payload := make([]byte, length)
		if length > 0 {
			if _, err := io.ReadFull(conn, payload); err != nil {
				break
			}
		}

		switch msgType {
		case 0x01: // Handshake UUID
			callUUID = string(payload)
		case 0x10: // Audio PCM SLIN16
			if callUUID != "" && len(payload) > 0 {
				mgr.Broadcast(callUUID, payload)
			}
		case 0x00, 0xFF: // Hangup / Error
			return
		}
	}
}

// ---------------------------------------------------------------------
// 4. VARIANTE 2: EAGI (Vía Socket TCP con Script Scripted netcat/socat)
// ---------------------------------------------------------------------

// HandleEAGIConnection lee el UUID y los frames PCM enviados por el script EAGI.
func HandleEAGIConnection(conn net.Conn, mgr *AudioStreamManager) {
	defer conn.Close()

	// Lee los primeros 36 bytes enviados por el script con el UUID
	uuidBuf := make([]byte, 36)
	if _, err := io.ReadFull(conn, uuidBuf); err != nil {
		log.Printf("[EAGI] Error leyendo UUID inicial: %v", err)
		return
	}
	callUUID := string(uuidBuf)
	log.Printf("[EAGI] TCP conectado uuid=%s origen=%s", callUUID, conn.RemoteAddr())
	mgr.AddAudioConnection(callUUID, conn)
	defer mgr.RemoveAudioConnection(callUUID, conn)
	defer mgr.CloseClients(callUUID)
	defer log.Printf("[EAGI] TCP audio stream ended uuid=%s", callUUID)

	// Buffer de 320 bytes (20ms PCM SLIN16 @ 8kHz)
	buffer := make([]byte, 320)
	var frames, bytesReceived int64
	defer func() {
		log.Printf("[EAGI] resumen uuid=%s frames=%d bytes=%d", callUUID, frames, bytesReceived)
	}()
	start, seen := time.Now(), false
	for {
		n, err := io.ReadFull(conn, buffer)
		if err != nil {
			log.Printf("[EAGI] TCP audio read stopped uuid=%s error=%v", callUUID, err)
			break
		}
		frames++
		bytesReceived += int64(n)
		if frames == 1 {
			log.Printf("[EAGI] primer audio recibido uuid=%s bytes=%d", callUUID, n)
		}
		if mgr.Broadcast(callUUID, buffer[:n]) > 0 {
			seen = true
		} else if seen || time.Since(start) > 15*time.Second {
			log.Printf("[EAGI] cerrando sin oyentes uuid=%s frames=%d", callUUID, frames)
			// Sin oyentes: al cerrar conn el script/EAGI termina y Asterisk cuelga el espionaje.
			return
		}
	}
}

// ---------------------------------------------------------------------
// 5. VARIANTE 3: MIXMONITOR (Lectura de Named Pipe / FIFO Local)
// ---------------------------------------------------------------------

// HandleMixMonitorPipe lee frames PCM de una FIFO y los distribuye hasta recibir la señal de parada.
func HandleMixMonitorPipe(fifoPath string, callUUID string, mgr *AudioStreamManager, stopChan <-chan struct{}) {
	fifo, err := os.OpenFile(fifoPath, os.O_RDONLY, 0600)
	if err != nil {
		log.Printf("[MixMonitor] Error abriendo pipe FIFO %s: %v", fifoPath, err)
		return
	}
	defer fifo.Close()

	buffer := make([]byte, 320)
	for {
		select {
		case <-stopChan:
			return
		default:
			n, err := fifo.Read(buffer)
			if err != nil {
				if err == io.EOF {
					continue // Esperar a que Asterisk escriba más frames
				}
				break
			}
			if n > 0 {
				mgr.Broadcast(callUUID, buffer[:n])
			}
		}
	}
}
