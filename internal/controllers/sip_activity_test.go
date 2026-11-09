package controllers

// Este archivo contiene pruebas para la detección de actividad de canales SIP.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestSIPVoiceEventsClearOnSilenceHangupAndIgnoreOtherCalls(t *testing.T) {
	setupReceiveActivityTest(t)
	server := sipActivityServer{host: "test", port: "5038"}
	allStarLinkReceiveStates.byNode["616262"] = allStarLinkReceiveState{
		Server: server, ObservedAt: time.Now().UTC(), SIPConnections: []string{"phone-portal"},
	}
	event := map[string]string{
		"Event": "ChannelTalkingStart", "Channel": "PJSIP/phone-portal-00000001", "Uniqueid": "call1", "Exten": "616262",
	}
	check := func(want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/activity?recent=true", nil), recorder)
		if err := GetAllStarLinkNodeActivity(c); err != nil {
			t.Fatal(err)
		}
		var response struct {
			Data []AllStarLinkNodeActivity `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Data) != want || (want == 1 && response.Data[0].RemoteNode != "SIP/phone-portal") {
			t.Fatalf("unexpected voice snapshot: %s", recorder.Body.String())
		}
	}
	check(0)
	updateSIPTalkingEvent(server, event)
	check(1)
	event["Event"] = "ChannelTalkingStop"
	event["Uniqueid"] = "old-call"
	updateSIPTalkingEvent(server, event)
	check(1)
	event["Uniqueid"] = "call1"
	updateSIPTalkingEvent(server, event)
	check(0)
	event["Event"] = "ChannelTalkingStart"
	updateSIPTalkingEvent(server, event)
	event["Channel"], event["Uniqueid"] = "PJSIP/phone-portal-00000002", "call2"
	updateSIPTalkingEvent(server, event)
	check(1)
	event["Event"] = "Hangup"
	updateSIPTalkingEvent(server, event)
	check(1)
	event["Channel"], event["Uniqueid"] = "PJSIP/phone-portal-00000001", "call1"
	updateSIPTalkingEvent(server, event)
	check(0)
	event["Event"] = "ChannelTalkingStart"
	updateSIPTalkingEvent(sipActivityServer{host: "other"}, event)
	check(0)
	updateSIPTalkingEvent(server, event)
	allStarLinkReceiveStates.byNode["616262"] = allStarLinkReceiveState{
		Server: server, ObservedAt: time.Now().Add(-4 * time.Second), SIPConnections: []string{"phone-portal"},
	}
	check(0)
}

func TestSIPVoiceWatcherClearsStateOnCancellation(t *testing.T) {
	setupReceiveActivityTest(t)
	server := sipActivityServer{host: "test"}
	updateSIPTalkingEvent(server, map[string]string{
		"Event": "ChannelTalkingStart", "Channel": "PJSIP/phone-portal-00000001", "Uniqueid": "call1", "Exten": "616262",
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { watchSIPReceiveActivity(ctx, server); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop on cancellation")
	}
	sipReceiveActivity.RLock()
	defer sipReceiveActivity.RUnlock()
	if len(sipReceiveActivity.byServer[server]) != 0 {
		t.Fatal("voice state survived disconnected AMI session")
	}
}
