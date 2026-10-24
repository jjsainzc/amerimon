package controllers

import (
	"amerimon/internal/config"
	"amerimon/internal/db"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestConnectionAudioSafeTarget(t *testing.T) {
	for _, scenario := range []string{"mute", "unmute", "unsupported", "ambiguous", "stale", "remote-server", "rejected", "disconnected", "wrong-local"} {
		t.Run(scenario, func(t *testing.T) {
			var mu sync.Mutex
			var actions []map[string]string
			host, port := mockConnectionAMI(t, func(fields map[string]string) string {
				mu.Lock()
				actions = append(actions, fields)
				mu.Unlock()
				switch fields["Action"] {
				case "ListCommands":
					if scenario == "unsupported" {
						return "Response: Success\r\nPing: Ping\r\n\r\n"
					}
					return "Response: Success\r\nMuteAudio: Mute audio (Priv: system,all)\r\n\r\n"
				case "RptStatus":
					if scenario == "disconnected" {
						return "Response: Success\r\nLinkedNodes:\r\n\r\n"
					}
					return "Response: Success\r\nConn: 61630 192.0.2.1 0 OUT 00:01 ESTABLISHED\r\n\r\n"
				case "MuteAudio":
					if scenario == "rejected" {
						return "Response: Error\r\nMessage: Permission denied\r\n\r\n"
					}
					return "Response: Success\r\nMessage: Success\r\n\r\n"
				}
				output := "IAX2/test-1!!61630!1!Up!Rpt!Remote Rx!!!!3!100!!connection-1\n"
				if fields["Command"] == "core show channels concise" {
					if scenario == "ambiguous" {
						output += "IAX2/test-2!!61630!1!Up!Rpt!Remote Rx!!!!3!100!!connection-2\n"
					}
				} else {
					name := strings.TrimPrefix(fields["Command"], "core show channel ")
					output = fmt.Sprintf("Name: %s\nUniqueID: connection-1\nExtension: 61630\nConnected Line ID: 616262\n -- Streams --\nName: audio-0\nType: audio\nState: sendrecv\n", name)
					if scenario == "wrong-local" {
						output = strings.ReplaceAll(output, "616262", "56502")
					}
				}
				return "Response: Follows\r\nOutput: " + strings.ReplaceAll(output, "\n", "\r\nOutput: ") + "\r\n--END COMMAND--\r\n\r\n"
			})
			dir := t.TempDir()
			database, err := db.Open(filepath.Join(dir, "audio.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			previous := config.App
			defer func() { config.App = previous }()
			config.App.DB = database
			config.App.AMI.Host, config.App.AMI.Port = host, port
			config.App.AMI.User, config.App.AMI.Secret = "test", "test"
			config.App.AMI.RPTConfigPath = filepath.Join(dir, "rpt.conf")
			config.App.Sources.AllmonINIPath = filepath.Join(dir, "allmon.ini")
			config.App.Sources.PrivateNodesPath = filepath.Join(dir, "private.txt")
			if err := os.WriteFile(config.App.AMI.RPTConfigPath, []byte("[616262]\n[nodes]\n616262 = radio@127.0.0.1/616262,NONE\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(config.App.Sources.AllmonINIPath, []byte(fmt.Sprintf("[616262]\nsystem=Nodes\nhost=%s\nport=%s\nuser=test\npasswd=test\n", host, port)), 0o600); err != nil {
				t.Fatal(err)
			}
			local, id := "616262", "connection-1"
			if scenario == "remote-server" {
				local = "56502"
			}
			if scenario == "stale" {
				id = "old-connection"
			}
			request := httptest.NewRequest(http.MethodPost, "/asterisk/connections/audio", strings.NewReader(fmt.Sprintf(
				`{"local_node":%q,"remote_node":"61630","connection_id":%q,"muted":%t}`, local, id, scenario != "unmute")))
			request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			recorder := httptest.NewRecorder()
			if err := SetConnectionAudio(echo.New().NewContext(request, recorder)); err != nil {
				t.Fatal(err)
			}
			success := scenario == "mute" || scenario == "unmute"
			if (recorder.Code == http.StatusOK) != success {
				t.Fatalf("unexpected response: %d %s", recorder.Code, recorder.Body.String())
			}
			if success {
				nodes := []AllStarLinkNode{{NodoLocal: "616262", Nodo: "61630", Protocolo: "iax"}}
				addConnectionAudioStates(nodes, allmonInstance{Node: "616262", Host: host, Port: port, User: "test", Secret: "test"})
				if nodes[0].Audio == nil || nodes[0].Audio.Muted == nil ||
					*nodes[0].Audio.Muted != (scenario == "mute") {
					t.Fatalf("refresh lost confirmed state: %+v", nodes[0].Audio)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			muteCount := 0
			for _, action := range actions {
				if action["Action"] != "MuteAudio" {
					continue
				}
				muteCount++
				state := "on"
				if scenario == "unmute" {
					state = "off"
				}
				if action["Direction"] != "in" || action["State"] != state || action["Channel"] != "IAX2/test-1" {
					t.Fatalf("unsafe mute target: %v", action)
				}
			}
			expected := 0
			if success || scenario == "rejected" {
				expected = 1
			}
			if muteCount != expected {
				t.Fatalf("MuteAudio count=%d expected=%d", muteCount, expected)
			}
		})
	}
}
