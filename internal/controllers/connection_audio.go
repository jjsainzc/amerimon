package controllers

// Este archivo implementa la asociación de las conexiones con sus canales de audio.

import (
	"amerimon/internal/config"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

type ConnectionAudio struct {
	Available    bool   `json:"available"`
	Muted        *bool  `json:"muted"`
	ConnectionID string `json:"connection_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type audioChannel struct {
	name, id string
}

var audioState = struct {
	sync.Mutex
	confirmed map[string]bool
}{confirmed: make(map[string]bool)}

var safeAudioChannel = regexp.MustCompile(`^(IAX2|echolink|tlb)/[A-Za-z0-9_.:@/;-]+$`)

func audioCommand(instance allmonInstance, command string) (string, error) {
	return config.ExecuteAMICommand(instance.Host, instance.Port, instance.User, instance.Secret, command, 5*time.Second)
}

func supportsConnectionAudio(instance allmonInstance) error {
	response, err := config.ExecuteAMIAudioAction(instance.Host, instance.Port, instance.User, instance.Secret, "", nil)
	if err != nil {
		return fmt.Errorf("no se pudo consultar el soporte de audio: %w", err)
	}
	for _, line := range strings.Split(response, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "MuteAudio:") {
			return nil
		}
	}
	return fmt.Errorf("MuteAudio no está disponible; habilita res_mutestream y los permisos AMI necesarios en Asterisk")
}

func findConnectionAudioChannel(instance allmonInstance, remote string, channels string) (audioChannel, error) {
	var matches []audioChannel
	for _, line := range config.ParseAMIOutputToLines(channels) {
		fields := strings.Split(line, "!")
		if len(fields) < 8 || !safeAudioChannel.MatchString(fields[0]) || fields[5] != "Rpt" {
			continue
		}
		if fields[2] != remote && !(fields[2] == instance.Node && fields[7] == remote) {
			continue
		}
		detail, err := audioCommand(instance, "core show channel "+fields[0])
		if err != nil {
			return audioChannel{}, err
		}
		values := make(map[string]string)
		for _, line := range config.ParseAMIOutputToLines(detail) {
			if strings.TrimSpace(line) == "-- Streams --" {
				break
			}
			key, value, found := strings.Cut(strings.TrimSpace(line), ":")
			if found {
				key = strings.TrimSpace(key)
				if _, exists := values[key]; !exists {
					values[key] = strings.TrimSpace(value)
				}
			}
		}
		outgoing := values["Extension"] == remote && values["Connected Line ID"] == instance.Node
		incoming := values["Extension"] == instance.Node && values["Caller ID"] == remote
		if values["Name"] == fields[0] && values["UniqueID"] != "" && (outgoing || incoming) {
			matches = append(matches, audioChannel{name: fields[0], id: values["UniqueID"]})
		}
	}
	if len(matches) != 1 {
		return audioChannel{}, fmt.Errorf("no se identificó un canal único y seguro para el nodo %s del servidor %s", remote, instance.Node)
	}
	return matches[0], nil
}

func addConnectionAudioStates(nodes []AllStarLinkNode, instance allmonInstance) {
	err := supportsConnectionAudio(instance)
	channels := ""
	if err == nil {
		channels, err = audioCommand(instance, "core show channels concise")
	}
	for i := range nodes {
		node := &nodes[i]
		if node.NodoLocal != instance.Node || node.Protocolo == "sip" {
			continue
		}
		state := &ConnectionAudio{}
		node.Audio = state
		if err != nil {
			state.Reason = err.Error()
			continue
		}
		channel, channelErr := findConnectionAudioChannel(instance, node.Nodo, channels)
		if channelErr != nil {
			state.Reason = channelErr.Error()
			continue
		}
		state.Available = true
		state.ConnectionID = channel.id
		audioState.Lock()
		value, known := audioState.confirmed[instance.Host+":"+instance.Port+":"+channel.id]
		audioState.Unlock()
		if known {
			state.Muted = &value
		}
	}
	if err != nil {
		log.Printf("audio: soporte no disponible local=%s: %v", instance.Node, err)
	}
}

func SetConnectionAudio(c echo.Context) error {
	var request struct {
		LocalNode    string `json:"local_node"`
		RemoteNode   string `json:"remote_node"`
		ConnectionID string `json:"connection_id"`
		Muted        *bool  `json:"muted"`
	}
	if err := c.Bind(&request); err != nil || request.Muted == nil || !isNumericNode(request.LocalNode) ||
		!isValidRemoteNodeID(request.RemoteNode) || request.ConnectionID == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{"status": "error", "message": "Solicitud de audio inválida."})
	}
	primary, err := newPrimaryNodeControl().primary()
	if err != nil {
		return primaryNodeError("No se pudo identificar el servidor principal", err)
	}
	if request.LocalNode != primary {
		return c.JSON(http.StatusForbidden, echo.Map{"status": "error", "message": "El control de audio solo está habilitado en el servidor principal."})
	}
	instance, _, err := findAllmonInstance(primary)
	if err != nil || instance.Node == "" {
		return c.JSON(http.StatusServiceUnavailable, echo.Map{"status": "error", "message": "No se pudo resolver el servidor principal en Allmon."})
	}
	started := time.Now()
	fail := func(err error) error {
		log.Printf("audio: operación falló local=%s remoto=%s: %v", primary, request.RemoteNode, err)
		recordAsteriskOperation(c.Request().Context(), "mute_audio", primary, request.RemoteNode, "", false, "MuteAudio Direction=in", "error", err.Error(), started)
		return c.JSON(http.StatusBadGateway, echo.Map{"status": "error", "message": err.Error()})
	}
	if err := supportsConnectionAudio(instance); err != nil {
		return fail(err)
	}
	xstat, err := config.ExecuteAMIRptStatus(instance.Host, instance.Port, instance.User, instance.Secret, primary, "XStat", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	if !hasEstablishedRPTConnection(xstat, request.RemoteNode) {
		return c.JSON(http.StatusConflict, echo.Map{"status": "error", "message": "El nodo ya no tiene un enlace establecido. Actualiza el monitor."})
	}
	channels, err := audioCommand(instance, "core show channels concise")
	if err != nil {
		return fail(err)
	}
	channel, err := findConnectionAudioChannel(instance, request.RemoteNode, channels)
	if err != nil {
		return fail(err)
	}
	if channel.id != request.ConnectionID {
		return c.JSON(http.StatusConflict, echo.Map{"status": "error", "message": "La conexión cambió. Actualiza el monitor antes de modificar el audio."})
	}
	audioState.Lock()
	defer audioState.Unlock()
	key := instance.Host + ":" + instance.Port + ":" + channel.id
	if _, err := config.ExecuteAMIAudioAction(instance.Host, instance.Port, instance.User, instance.Secret, channel.name, request.Muted); err != nil {
		delete(audioState.confirmed, key)
		return fail(err)
	}
	audioState.confirmed[key] = *request.Muted
	recordAsteriskOperation(c.Request().Context(), "mute_audio", primary, request.RemoteNode, "", false, "MuteAudio Direction=in", "success", fmt.Sprintf("muted=%t", *request.Muted), started)
	return c.JSON(http.StatusOK, echo.Map{"status": "success", "data": ConnectionAudio{
		Available: true, Muted: request.Muted, ConnectionID: channel.id,
	}})
}
