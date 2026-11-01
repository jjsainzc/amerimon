/**
 * @file node_activity_live_test.go
 * @brief Pruebas en vivo de la actividad de los nodos StarLink.
 */
package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

// setupReceiveActivityTest configura el entorno para probar la actividad de recepción.
// Inicializa una base de datos temporal, limpia los estados de recepción globales y asegura la limpieza después de la prueba.
func setupReceiveActivityTest(t *testing.T) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "activity.db"))
	if err != nil {
		t.Fatal(err)
	}
	previous := config.App
	config.App.DB = database
	allStarLinkReceiveStates.Lock()
	previousStates := allStarLinkReceiveStates.byNode
	allStarLinkReceiveStates.byNode = make(map[string]allStarLinkReceiveState)
	allStarLinkReceiveStates.Unlock()
	t.Cleanup(func() {
		config.App = previous
		database.Close()
		allStarLinkReceiveStates.Lock()
		allStarLinkReceiveStates.byNode = previousStates
		allStarLinkReceiveStates.Unlock()
	})
}

// TestRPTReceiveStateUsesCOSAndRXNotLastKeyTime verifica que el estado de recepción de RPT utilice COS y RX, y no el tiempo de la última clave.
func TestRPTReceiveStateUsesCOSAndRXNotLastKeyTime(t *testing.T) {
	for _, test := range []struct {
		name, raw string
		want      int
		wantError bool
	}{
		{"receiving before first unkey", "Conn: 700001 1 90 -1", 1, false},
		{"inactive despite recent key", "Conn: 700001 0 0 0", 0, false},
		{"never keyed", "Conn: 700001 0 -1 -1", 0, false},
		{"invalid rx", "Conn: 700001 2 0 0", 0, true},
		{"truncated", "Conn: 700001", 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			links, err := parseRPTReceivingLinks(test.raw, "616265", time.Now().UTC())
			if (err != nil) != test.wantError || len(links) != test.want {
				t.Fatalf("links=%v error=%v", links, err)
			}
		})
	}
	for _, value := range []string{"YES", "NO"} {
		cos, err := parseRPTCOS("Response: Success\r\nSignalOnInput: " + value + "\r\n")
		if err != nil || cos != (value == "YES") {
			t.Fatalf("COS %s: %v %v", value, cos, err)
		}
	}
	if _, err := parseRPTCOS("Response: Success"); err == nil {
		t.Fatal("missing COS must report an error")
	}
	_, connections, err := parseRPTReceiveState("Conn: IAXRPT 0 0 0", "616265", time.Now().UTC())
	if err != nil || len(connections) != 1 || connections[0] != "IAXRPT" {
		t.Fatalf("inactive IAXRpt connection missing from snapshot: %v, %v", connections, err)
	}
}

// TestGetAllStarLinkNodeActivityReportsCOSAndClearsOnUnkey verifica que la actividad de los nodos StarLink reporte COS y se limpie al desactivar la clave.
func TestGetAllStarLinkNodeActivityReportsCOSAndClearsOnUnkey(t *testing.T) {
	setupReceiveActivityTest(t)
	var keyed atomic.Bool
	keyed.Store(true)
	host, port := mockConnectionAMI(t, func(fields map[string]string) string {
		if fields["Command"] == "NodeStat" {
			if keyed.Load() {
				return "Response: Success\r\nSignalOnInput: YES\r\n\r\n"
			}
			return "Response: Success\r\nSignalOnInput: NO\r\n\r\n"
		}
		if fields["Command"] == "SawStat" {
			if keyed.Load() {
				return "Response: Success\r\nConn: 700001 1 90 -1\r\nConn: 56502 0 0 0\r\n\r\n"
			}
			return "Response: Success\r\nConn: 700001 0 0 0\r\n\r\n"
		}
		return "Response: Error\r\nMessage: unexpected action\r\n\r\n"
	})
	instance := allmonInstance{Node: "616265", Host: host, Port: port, User: "test", Secret: "test"}
	for _, active := range []bool{true, false} {
		keyed.Store(active)
		pollAllStarLinkReceiveState(instance)
		recorder := httptest.NewRecorder()
		context := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/activity?recent=true", nil), recorder)
		if err := GetAllStarLinkNodeActivity(context); err != nil {
			t.Fatal(err)
		}
		var response struct {
			Status      string                    `json:"status"`
			Data        []AllStarLinkNodeActivity `json:"data"`
			Servers     []string                  `json:"server_nodes"`
			Connections map[string][]string       `json:"connections"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != http.StatusOK || response.Status != "success" {
			t.Fatalf("unexpected response: %s", recorder.Body.String())
		}
		if active {
			if len(response.Data) != 1 || response.Data[0].RemoteNode != "700001" ||
				len(response.Servers) != 1 || response.Servers[0] != "616265" ||
				len(response.Connections["616265"]) != 2 {
				t.Fatalf("COS/RX not reported: %s", recorder.Body.String())
			}
		} else if len(response.Data) != 0 || len(response.Servers) != 0 ||
			len(response.Connections["616265"]) != 1 {
			t.Fatalf("COS/RX still active after unkey: %s", recorder.Body.String())
		}
	}
}

// TestStreamAllStarLinkNodeActivityPublishesAndClearsReceiveState verifica que la transmisión de la actividad de los nodos StarLink publique y limpie el estado de recepción.
func TestStreamAllStarLinkNodeActivityPublishesAndClearsReceiveState(t *testing.T) {
	setupReceiveActivityTest(t)
	now := time.Now().UTC()
	allStarLinkReceiveStates.Lock()
	allStarLinkReceiveStates.byNode["616265"] = allStarLinkReceiveState{
		ObservedAt: now, COS: true,
		Links:       []AllStarLinkNodeActivity{{LocalNode: "616265", RemoteNode: "700001", UltimaActividad: now}},
		Connections: []string{"700001"},
	}
	allStarLinkReceiveStates.byNode["56502"] = allStarLinkReceiveState{
		ObservedAt: now.Add(-30 * time.Second), COS: true,
		Links: []AllStarLinkNodeActivity{{LocalNode: "56502", RemoteNode: "700001", UltimaActividad: now}},
	}
	allStarLinkReceiveStates.Unlock()
	e := echo.New()
	e.GET("/activity/live", StreamAllStarLinkNodeActivity)
	server := httptest.NewServer(e)
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"/activity/live",
		http.Header{"Origin": []string{server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Data        []AllStarLinkNodeActivity `json:"data"`
		Servers     []string                  `json:"server_nodes"`
		Connections map[string][]string       `json:"connections"`
	}
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 1 || len(response.Servers) != 1 || response.Servers[0] != "616265" ||
		len(response.Connections["616265"]) != 1 || response.Connections["616265"][0] != "700001" {
		t.Fatalf("incorrect receive snapshot: %+v", response)
	}
	allStarLinkReceiveStates.Lock()
	allStarLinkReceiveStates.byNode["616265"] = allStarLinkReceiveState{ObservedAt: time.Now().UTC()}
	allStarLinkReceiveStates.Unlock()
	if err := conn.SetReadDeadline(time.Now().Add(750 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data) != 0 || len(response.Servers) != 0 {
		t.Fatalf("receive snapshot did not turn off: %+v", response)
	}
}

// TestReceiveActivityPollingAndPublishingIntervals verifica que los intervalos de sondeo y publicación de la actividad de recepción no excedan los 250 ms.
func TestReceiveActivityPollingAndPublishingIntervals(t *testing.T) {
	if liveActivityPollInterval > 250*time.Millisecond || liveActivityPublishInterval > 250*time.Millisecond {
		t.Fatal("COS/RX polling and publishing must not each add more than 250ms")
	}
}
