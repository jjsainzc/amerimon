package controllers

import (
	"amerimon/internal/config"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestConnectionModalityChanges(t *testing.T) {
	for oldAction, oldMode := range map[string]string{"connect": "T", "monitor": "R", "localmonitor": "L"} {
		for _, oldPermanent := range []bool{false, true} {
			for _, action := range []string{"connect", "monitor", "localmonitor"} {
				for _, permanent := range []bool{false, true} {
					name := fmt.Sprintf("%s/%t/to/%s/%t", oldAction, oldPermanent, action, permanent)
					t.Run(name, func(t *testing.T) {
						var mu sync.Mutex
						var commands []string
						disconnected, connected := false, false
						setupRPTSafetyTest(t, func(fields map[string]string) string {
							mu.Lock()
							defer mu.Unlock()
							if fields["Action"] == "RptStatus" {
								if fields["Node"] != "50001" {
									return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
								}
								status := "Conn: 90001 192.0.2.9 OUT 00:01\r\n"
								if !disconnected || connected {
									mode := oldMode
									if connected {
										mode = map[string]string{"connect": "T", "monitor": "R", "localmonitor": "L"}[action]
									}
									status += fmt.Sprintf("Conn: 50002 192.0.2.2 0 OUT 00:01 ESTABLISHED\r\nLinkedNodes: %s50002\r\n", mode)
								}
								return "Response: Success\r\n" + status + "\r\n"
							}
							command := fields["Command"]
							commands = append(commands, command)
							if command == "rpt cmd 50001 ilink 11 50002" {
								disconnected = true
							} else {
								connected = true
							}
							return "Response: Follows\r\nOutput: OK\r\n--END COMMAND--\r\n\r\n"
						})
						oldCommand := fmt.Sprintf("rpt cmd 50001 ilink %d 50002", supermonConnectionCode(oldAction, oldPermanent))
						recordAsteriskOperation(context.Background(), oldAction, "50001", "50002", "", oldPermanent, oldCommand, "success", "", time.Now())
						req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(
							`{"local_node":"50001","remote_node":"50002","action":%q,"permanent":%t}`, action, permanent)))
						req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
						rec := httptest.NewRecorder()
						if err := ExecuteSupermonCommand(echo.New().NewContext(req, rec)); err != nil {
							t.Fatal(err)
						}
						mu.Lock()
						defer mu.Unlock()
						if action == oldAction && permanent == oldPermanent {
							if rec.Code != http.StatusConflict || len(commands) != 0 || !strings.Contains(rec.Body.String(), "mismo modo") {
								t.Fatalf("duplicate status=%d body=%s commands=%v", rec.Code, rec.Body.String(), commands)
							}
							return
						}
						want := []string{"rpt cmd 50001 ilink 11 50002",
							fmt.Sprintf("rpt cmd 50001 ilink %d 50002", supermonConnectionCode(action, permanent))}
						if rec.Code != http.StatusOK || !reflect.DeepEqual(commands, want) {
							t.Fatalf("status=%d body=%s commands=%v want=%v", rec.Code, rec.Body.String(), commands, want)
						}
					})
				}
			}
		}
	}
}

func TestConnectionModalityUnknownAndFailures(t *testing.T) {
	for _, scenario := range []string{"unknown", "disconnect-pending", "disconnect-failure", "status-failure", "alternate-path", "legacy-api"} {
		t.Run(scenario, func(t *testing.T) {
			var mu sync.Mutex
			var commands []string
			disconnected := false
			postDisconnectQueries := 0
			setupRPTSafetyTest(t, func(fields map[string]string) string {
				mu.Lock()
				defer mu.Unlock()
				if fields["Action"] == "RptStatus" {
					if disconnected && fields["Node"] == "50001" {
						postDisconnectQueries++
						if scenario == "disconnect-pending" && postDisconnectQueries == 1 {
							return "Response: Success\r\nConn: 50002 192.0.2.2 OUT 00:01\r\n\r\n"
						}
					}
					if scenario == "status-failure" && disconnected && fields["Node"] == "50001" {
						return "Response: Success\r\n\r\n"
					}
					if fields["Node"] == "50001" && !disconnected {
						return "Response: Success\r\nConn: 50002 192.0.2.2 OUT 00:01\r\nLinkedNodes: T50002\r\n\r\n"
					}
					if scenario == "alternate-path" {
						if fields["Node"] == "50001" {
							return "Response: Success\r\nConn: 50003 192.0.2.3 OUT 00:01\r\n\r\n"
						}
						if fields["Node"] == "50003" {
							return "Response: Success\r\nConn: 50002 192.0.2.2 OUT 00:01\r\n\r\n"
						}
					}
					return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
				}
				command := fields["Command"]
				if scenario == "disconnect-pending" && command != "rpt cmd 50001 ilink 11 50002" && postDisconnectQueries < 2 {
					return "Response: Error\r\nMessage: old link still present\r\n\r\n"
				}
				commands = append(commands, command)
				if scenario == "disconnect-failure" {
					return "Response: Error\r\nMessage: disconnect rejected\r\n\r\n"
				}
				if command == "rpt cmd 50001 ilink 11 50002" {
					disconnected = true
				}
				return "Response: Follows\r\nOutput: OK\r\n--END COMMAND--\r\n\r\n"
			})
			body := `{"local_node":"50001","remote_node":"50002","action":"monitor","permanent":true}`
			if scenario == "legacy-api" {
				body = `{"local_node":"50001","remote_node":"50002","mode":"permanent"}`
			}
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)
			var err error
			if scenario == "legacy-api" {
				err = AddNodeConnection(c)
			} else {
				err = ExecuteSupermonCommand(c)
			}
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			wantStatus, wantCommands := http.StatusOK, 2
			switch scenario {
			case "disconnect-failure", "status-failure":
				wantStatus, wantCommands = http.StatusBadGateway, 1
			case "alternate-path":
				wantStatus, wantCommands = http.StatusConflict, 1
			}
			if rec.Code != wantStatus || len(commands) != wantCommands || commands[0] != "rpt cmd 50001 ilink 11 50002" {
				t.Fatalf("status=%d body=%s commands=%v", rec.Code, rec.Body.String(), commands)
			}
			if scenario == "legacy-api" {
				var permanent bool
				if err := config.App.DB.QueryRow("SELECT permanent FROM asterisk_operation_logs WHERE action = 'add_connection' ORDER BY id DESC LIMIT 1").Scan(&permanent); err != nil || !permanent {
					t.Fatalf("legacy permanence not recorded: %t %v", permanent, err)
				}
			}
		})
	}
}

func TestTargetedDisconnectRequiresConfirmedAbsence(t *testing.T) {
	var mu sync.Mutex
	var commands []string
	instance := setupRPTSafetyTest(t, func(fields map[string]string) string {
		if fields["Action"] == "RptStatus" {
			return "Response: Success\r\nConn: 50002 192.0.2.2 OUT 00:01\r\n\r\n"
		}
		mu.Lock()
		commands = append(commands, fields["Command"])
		mu.Unlock()
		return "Response: Follows\r\nOutput: OK\r\n--END COMMAND--\r\n\r\n"
	})
	err := disconnectRPTConnections(context.Background(), instance, 200*time.Millisecond, "50002")
	if err == nil {
		t.Fatal("expected timeout while the old link is still present")
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(commands, []string{"rpt cmd 50001 ilink 11 50002"}) {
		t.Fatalf("unexpected commands: %v", commands)
	}
}
