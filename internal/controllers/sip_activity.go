package controllers

// Este archivo implementa la detección de actividad de canales SIP.

import (
	"amerimon/internal/config"
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// sipActivityServer representa un servidor SIP para la detección de actividad.
type sipActivityServer struct {
	host, port, user, secret string
}

// sipTalkingChannel representa un canal SIP que está actualmente activo en una conversación.
type sipTalkingChannel struct {
	id, node, endpoint string
}

// sipReceiveActivity mantiene el estado de los canales SIP activos por servidor.
var sipReceiveActivity = struct {
	sync.RWMutex
	byServer map[sipActivityServer]map[string]sipTalkingChannel
}{byServer: make(map[sipActivityServer]map[string]sipTalkingChannel)}

// updateSIPTalkingEvent actualiza el estado de un canal SIP en función de un evento recibido.
func updateSIPTalkingEvent(server sipActivityServer, event map[string]string) {
	switch event["Event"] {
	case "ChannelTalkingStart", "ChannelTalkingStop", "Hangup":
	default:
		return
	}
	channel, pjsip := strings.CutPrefix(event["Channel"], "PJSIP/")
	separator := strings.LastIndex(channel, "-")
	if !pjsip || separator <= 0 || separator == len(channel)-1 || event["Uniqueid"] == "" {
		return
	}
	endpoint := channel[:separator]
	if !sipUserName.MatchString(endpoint) {
		return
	}
	sipReceiveActivity.Lock()
	defer sipReceiveActivity.Unlock()
	channels := sipReceiveActivity.byServer[server]
	if channels == nil {
		channels = make(map[string]sipTalkingChannel)
		sipReceiveActivity.byServer[server] = channels
	}
	if event["Event"] != "ChannelTalkingStart" {
		if previous, exists := channels[event["Channel"]]; exists && previous.id == event["Uniqueid"] {
			delete(channels, event["Channel"])
		}
		return
	}
	if !isNumericNode(event["Exten"]) {
		log.Printf("allstarlink activity: evento de voz SIP sin nodo destino válido")
		return
	}
	channels[event["Channel"]] = sipTalkingChannel{
		id: event["Uniqueid"], node: event["Exten"], endpoint: endpoint,
	}
}

// currentSIPReceiveActivity devuelve la actividad actual de los canales SIP en función del estado configurado y el tiempo actual.
func currentSIPReceiveActivity(now time.Time, configured map[string]allStarLinkReceiveState) []AllStarLinkNodeActivity {
	activities := make([]AllStarLinkNodeActivity, 0)
	seen := make(map[string]bool)
	sipReceiveActivity.RLock()
	defer sipReceiveActivity.RUnlock()
	for server, channels := range sipReceiveActivity.byServer {
		for _, channel := range channels {
			state, exists := configured[channel.node]
			if !exists || state.Server != server || now.Sub(state.ObservedAt) > 3*time.Second {
				continue
			}
			connected := false
			for _, endpoint := range state.SIPConnections {
				if endpoint == channel.endpoint {
					connected = true
					break
				}
			}
			key := channel.node + "\x00" + channel.endpoint
			if connected && !seen[key] {
				seen[key] = true
				activities = append(activities, AllStarLinkNodeActivity{
					LocalNode: channel.node, RemoteNode: "SIP/" + channel.endpoint, UltimaActividad: now,
				})
			}
		}
	}
	return activities
}

// watchSIPReceiveActivity supervisa los eventos de actividad SIP de un servidor y actualiza el estado correspondiente.
func watchSIPReceiveActivity(ctx context.Context, server sipActivityServer) {
	defer func() {
		sipReceiveActivity.Lock()
		delete(sipReceiveActivity.byServer, server)
		sipReceiveActivity.Unlock()
	}()
	for ctx.Err() == nil {
		err := config.StreamAMIEvents(ctx, server.host, server.port, server.user, server.secret,
			func(event map[string]string) { updateSIPTalkingEvent(server, event) })
		sipReceiveActivity.Lock()
		delete(sipReceiveActivity.byServer, server)
		sipReceiveActivity.Unlock()
		if ctx.Err() != nil {
			return
		}
		log.Printf("allstarlink activity: eventos de voz SIP en %s:%s interrumpidos: %v", server.host, server.port, err)
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
