package controllers

// Este archivo contiene pruebas para las operaciones de conexión y desconexión de nodos mediante Asterisk.

import (
	"amerimon/internal/config"
	"amerimon/internal/db"
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func mockConnectionAMI(t *testing.T, respond func(map[string]string) string) (string, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			func() {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				fmt.Fprint(conn, "Asterisk Call Manager/1.0\r\n")
				reader := bufio.NewReader(conn)
				for {
					fields := make(map[string]string)
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							return
						}
						line = strings.TrimSpace(line)
						if line == "" {
							break
						}
						key, value, found := strings.Cut(line, ":")
						if found {
							fields[key] = strings.TrimSpace(value)
						}
					}
					if fields["Action"] == "Login" {
						fmt.Fprint(conn, "Response: Success\r\nMessage: Authentication accepted\r\n\r\n")
						continue
					}
					if fields["ActionID"] != "" {
						fmt.Fprintf(conn, "ActionID: %s\r\n", fields["ActionID"])
					}
					fmt.Fprint(conn, respond(fields))
					return
				}
			}()
		}
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func TestConnectAfterDisconnectingSelectedServer(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		enabled bool
		fail    bool
	}{
		{"disabled", false, false},
		{"disconnect-first", true, false},
		{"disconnect-fails", true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var operations []string
			var operationsMu sync.Mutex
			statusQueries := 0
			host, port := mockConnectionAMI(t, func(fields map[string]string) string {
				operationsMu.Lock()
				defer operationsMu.Unlock()
				if fields["Action"] == "RptStatus" {
					operations = append(operations, "XStat "+fields["Node"])
					statusQueries++
					if statusQueries == 1 {
						return "Response: Success\r\nConn: 10001 127.0.0.1 OUT 00:01\r\nConn: 10002 127.0.0.1 OUT 00:01\r\n\r\n"
					}
					return "Response: Success\r\nLinkedNodes:\r\n\r\n"
				}
				operations = append(operations, fields["Command"])
				if scenario.fail {
					return "Response: Error\r\nMessage: Simulated disconnect failure\r\n\r\n"
				}
				return "Response: Follows\r\nOutput: OK\r\n--END COMMAND--\r\n\r\n"
			})
			dir := t.TempDir()
			database, err := db.Open(filepath.Join(dir, "test.db"))
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
			config.App.Sources.PrivateNodesPath = filepath.Join(dir, "privatenodes.txt")
			if err := os.WriteFile(config.App.AMI.RPTConfigPath, []byte("[56502]\n[nodes]\n56502 = radio@127.0.0.1/56502,NONE\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			config.App.Sources.AllmonINIPath = filepath.Join(dir, "allmon.ini")
			if err := os.WriteFile(config.App.Sources.AllmonINIPath, []byte(fmt.Sprintf(
				"[56502]\nsystem=Nodes\nhost=%s\nport=%s\nuser=test\npasswd=test\n[616262]\nsystem=Nodes\nhost=192.0.2.1\nport=5038\nuser=other\npasswd=other\namerimon_tracking=disabled\n", host, port)), 0o600); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/asterisk/commands", strings.NewReader(fmt.Sprintf(
				`{"local_node":"56502","remote_node":"46818","action":"connect","permanent":true,"disconnect_before_connect":%t}`, scenario.enabled)))
			request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			recorder := httptest.NewRecorder()
			if err := ExecuteSupermonCommand(echo.New().NewContext(request, recorder)); err != nil {
				t.Fatal(err)
			}
			want := []string{"XStat 56502", "rpt cmd 56502 ilink 13 46818"}
			status := http.StatusOK
			if scenario.enabled {
				want = []string{"XStat 56502", "rpt cmd 56502 ilink 11 10001"}
				if scenario.fail {
					status = http.StatusBadGateway
				} else {
					want = append(want, "rpt cmd 56502 ilink 11 10002", "XStat 56502", "XStat 56502", "rpt cmd 56502 ilink 13 46818")
				}
			}
			operationsMu.Lock()
			defer operationsMu.Unlock()
			if recorder.Code != status || !reflect.DeepEqual(operations, want) {
				t.Fatalf("status=%d body=%s operations=%v want=%v", recorder.Code, recorder.Body.String(), operations, want)
			}
		})
	}
}

func TestDisconnectBeforeConnectRejectsOtherActions(t *testing.T) {
	for _, action := range []string{"disconnect", "restart"} {
		request := SupermonCommandRequest{LocalNode: "56502", RemoteNode: "10001", Action: action, DisconnectBeforeConnect: true}
		_, err := buildSupermonCommand(request, newSupermonCommandDefinition(action, action, "rpt restart %s"))
		if err == nil || !strings.Contains(err.Error(), "solo se admite") {
			t.Fatalf("action %s: expected validation error, got %v", action, err)
		}
	}
}

func TestDisconnectBeforeConnectRequiresConfirmedEmptyServer(t *testing.T) {
	for _, scenario := range []string{"empty", "query-error", "still-connected", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			host, port := mockConnectionAMI(t, func(fields map[string]string) string {
				if fields["Action"] == "RptStatus" {
					switch scenario {
					case "query-error":
						return "Response: Error\r\nMessage: Simulated query failure\r\n\r\n"
					case "still-connected":
						return "Response: Success\r\nConn: 10001 127.0.0.1 OUT 00:01\r\n\r\n"
					}
					return "Response: Success\r\nLinkedNodes:\r\n\r\n"
				}
				return "Response: Follows\r\nOutput: OK\r\n--END COMMAND--\r\n\r\n"
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancelled" {
				cancel()
			}
			err := disconnectRPTConnections(ctx, allmonInstance{Node: "56502", Host: host, Port: port, User: "test", Secret: "test"}, 100*time.Millisecond)
			if (err == nil) != (scenario == "empty") {
				t.Fatalf("scenario %s: unexpected result %v", scenario, err)
			}
		})
	}
}
