package controllers

import (
	"amerimon/internal/config"
	"amerimon/internal/db"
	"context"
	"fmt"
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

func TestRPTLinkGraph(t *testing.T) {
	graph := make(rptLinkGraph)
	for _, edge := range [][2]string{
		{"50001", "50002"}, {"50002", "50003"}, {"50003", "50004"},
		{"50002", "50005"}, {"60001", "60002"}, {"60002", "60003"}, {"60003", "60001"},
	} {
		graph.add(edge[0], edge[1])
	}
	for _, test := range []struct {
		origin, destination string
		want                []string
	}{
		{"50001", "50001", []string{"50001"}},
		{"50002", "50001", []string{"50002", "50001"}},
		{"50001", "50004", []string{"50001", "50002", "50003", "50004"}},
		{"50005", "50003", []string{"50005", "50002", "50003"}},
		{"50001", "60001", nil},
		{"60001", "70001", nil},
	} {
		if got := graph.path(test.origin, test.destination); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("path %s -> %s = %v, want %v", test.origin, test.destination, got, test.want)
		}
	}
}

func TestAddRPTVisibleLinks(t *testing.T) {
	for _, test := range []struct {
		name, raw string
		want      bool
		wantError bool
	}{
		{name: "empty", raw: "LinkedNodes:"},
		{name: "asterisk empty marker", raw: "LinkedNodes: <NONE>"},
		{name: "asterisk empty marker with whitespace", raw: "LinkedNodes:   <NONE>  \r\n"},
		{name: "direct connection with empty announcement", raw: "Conn: 50002 192.0.2.1 OUT 00:01\r\nLinkedNodes: <NONE>", want: true},
		{name: "direct", raw: "Conn: 50002 192.0.2.1 OUT 00:01", want: true},
		{name: "reverse", raw: "Conn: 50002 192.0.2.1 0 IN 00:01 ESTABLISHED", want: true},
		{name: "connecting", raw: "LinkedNodes: C50002\nConn: 50002 192.0.2.1 OUT 00:00", want: true},
		{name: "indirect announcement", raw: "LinkedNodes: T50003,T50002", want: true},
		{name: "disconnected", raw: "Conn: 50002 192.0.2.1 0 OUT 00:01 DISCONNECTED"},
		{name: "no list", raw: "Response: Success", wantError: true},
		{name: "SIP does not form an RPT edge", raw: "Conn: SIP/1001 192.0.2.1 IN 00:01"},
		{name: "malformed direct", raw: "Conn:", wantError: true},
		{name: "malformed indirect", raw: "LinkedNodes: T", wantError: true},
		{name: "empty marker mixed with nodes", raw: "LinkedNodes: <NONE>, T50002", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := make(rptLinkGraph)
			err := addRPTVisibleLinks(graph, "50001", test.raw)
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, want error %v", err, test.wantError)
			}
			if (len(graph.path("50002", "50001")) > 0) != test.want {
				t.Fatalf("unexpected reachability: %v", graph)
			}
		})
	}
}

func setupRPTSafetyTest(t *testing.T, respond func(map[string]string) string) allmonInstance {
	t.Helper()
	previous := config.App
	t.Cleanup(func() { config.App = previous })
	rptConnectionSafety.Lock()
	rptConnectionSafety.pending = make(map[pendingRPTLink]time.Time)
	rptConnectionSafety.Unlock()
	t.Cleanup(func() {
		rptConnectionSafety.Lock()
		rptConnectionSafety.pending = make(map[pendingRPTLink]time.Time)
		rptConnectionSafety.Unlock()
	})
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	config.App.DB = database
	config.App.AMI.RPTConfigPath = filepath.Join(dir, "rpt.conf")
	config.App.Sources.AllmonINIPath = filepath.Join(dir, "allmon.ini")
	config.App.Sources.PrivateNodesPath = filepath.Join(dir, "privatenodes.txt")
	if err := os.WriteFile(config.App.AMI.RPTConfigPath,
		[]byte("[50001]\n[50002]\n[50003]\n[nodes]\n50001 = radio@127.0.0.1/50001,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	host, port := mockConnectionAMI(t, respond)
	config.App.AMI.Host, config.App.AMI.Port = host, port
	config.App.AMI.User, config.App.AMI.Secret = "test", "test"
	return allmonInstance{Node: "50001", Host: host, Port: port, User: "test", Secret: "test"}
}

func TestConnectionAPIsRejectVisibleLoops(t *testing.T) {
	for _, test := range []struct {
		name, local, remote string
		statuses            map[string]string
		wantStatus          int
	}{
		{name: "self", local: "50001", remote: "50001", wantStatus: http.StatusConflict},
		{name: "mirror", local: "50001", remote: "50002", wantStatus: http.StatusConflict,
			statuses: map[string]string{"50002": "Conn: 50001 192.0.2.1 OUT 00:01"}},
		{name: "indirect chain", local: "50001", remote: "50003", wantStatus: http.StatusConflict,
			statuses: map[string]string{"50001": "Conn: 50002 192.0.2.1 OUT 00:01", "50002": "Conn: 50003 192.0.2.2 OUT 00:01"}},
		{name: "siblings", local: "50002", remote: "50003", wantStatus: http.StatusConflict,
			statuses: map[string]string{"50001": "Conn: 50002 192.0.2.1 OUT 00:01\nConn: 50003 192.0.2.2 OUT 00:01"}},
		{name: "indirect outside configured nodes", local: "50001", remote: "90001", wantStatus: http.StatusConflict,
			statuses: map[string]string{"50001": "LinkedNodes: T90001"}},
		{name: "new peer extends chain", local: "50003", remote: "90001", wantStatus: http.StatusOK,
			statuses: map[string]string{"50001": "Conn: 50002 192.0.2.1 OUT 00:01", "50002": "Conn: 50003 192.0.2.2 OUT 00:01"}},
		{name: "asterisk empty announcements allow connection", local: "50001", remote: "50002", wantStatus: http.StatusOK,
			statuses: map[string]string{"50001": "LinkedNodes: <NONE>", "50002": "LinkedNodes: <NONE>", "50003": "LinkedNodes: <NONE>"}},
		{name: "independent components", local: "50001", remote: "50003", wantStatus: http.StatusOK,
			statuses: map[string]string{"50001": "Conn: 90001 192.0.2.1 OUT 00:01", "50003": "Conn: 90002 192.0.2.2 OUT 00:01"}},
		{name: "unavailable topology", local: "50001", remote: "90001", wantStatus: http.StatusOK,
			statuses: map[string]string{"50002": "Response: Error\r\nMessage: unavailable"}},
		{name: "malformed topology", local: "50001", remote: "90001", wantStatus: http.StatusOK,
			statuses: map[string]string{"50002": "Response: Success"}},
		{name: "unavailable origin topology", local: "50001", remote: "90001", wantStatus: http.StatusOK,
			statuses: map[string]string{"50001": "Response: Error\r\nMessage: unavailable"}},
		{name: "visible loop with unavailable peer", local: "50001", remote: "90001", wantStatus: http.StatusConflict,
			statuses: map[string]string{"50001": "LinkedNodes: T90001", "50002": "Response: Error\r\nMessage: unavailable"}},
		{name: "malformed snapshot is not a partial route", local: "50001", remote: "90001", wantStatus: http.StatusOK,
			statuses: map[string]string{"50002": "LinkedNodes: T50001,T90001\nConn:"}},
	} {
		for _, mode := range []string{"connect", "monitor", "localmonitor", "temporary-connect", "temporary-monitor", "temporary-localmonitor", "transceiver", "permanent", "local_monitor", "add-monitor"} {
			t.Run(test.name+"/"+mode, func(t *testing.T) {
				var mu sync.Mutex
				var commands []string
				setupRPTSafetyTest(t, func(fields map[string]string) string {
					mu.Lock()
					defer mu.Unlock()
					if fields["Action"] == "RptStatus" {
						if len(commands) > 0 && fields["Node"] == test.local {
							return fmt.Sprintf("Response: Success\r\nConn: %s 192.0.2.1 OUT 00:01\r\n\r\n", test.remote)
						}
						status, exists := test.statuses[fields["Node"]]
						if !exists {
							status = "LinkedNodes:"
						}
						return "Response: Success\r\n" + strings.ReplaceAll(status, "\n", "\r\n") + "\r\n\r\n"
					}
					commands = append(commands, fields["Command"])
					return "Response: Follows\r\nOutput: OK\r\n--END COMMAND--\r\n\r\n"
				})
				action := strings.TrimPrefix(mode, "temporary-")
				supermon := action == "connect" || action == "monitor" || action == "localmonitor"
				body := fmt.Sprintf(`{"local_node":%q,"remote_node":%q,"mode":%q}`, test.local, test.remote, mode)
				if mode == "add-monitor" {
					body = fmt.Sprintf(`{"local_node":%q,"remote_node":%q,"mode":"monitor"}`, test.local, test.remote)
				}
				if supermon {
					body = fmt.Sprintf(`{"local_node":%q,"remote_node":%q,"action":%q,"permanent":%t}`, test.local, test.remote, action, !strings.HasPrefix(mode, "temporary-"))
				}
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				rec := httptest.NewRecorder()
				c := echo.New().NewContext(req, rec)
				var err error
				if supermon {
					err = ExecuteSupermonCommand(c)
				} else {
					err = AddNodeConnection(c)
				}
				if err != nil {
					t.Fatal(err)
				}
				mu.Lock()
				defer mu.Unlock()
				wantCommands := 0
				if test.wantStatus == http.StatusOK {
					wantCommands = 1
				}
				if rec.Code != test.wantStatus || len(commands) != wantCommands {
					t.Fatalf("status=%d body=%s commands=%v", rec.Code, rec.Body.String(), commands)
				}
				if wantCommands == 0 {
					var result, details string
					if err := config.App.DB.QueryRow("SELECT result, details FROM asterisk_operation_logs ORDER BY id DESC LIMIT 1").Scan(&result, &details); err != nil {
						t.Fatal(err)
					}
					if result != "connection_rejected" || details == "" {
						t.Fatalf("validation rejection was not recorded: %s %q", result, details)
					}
				}
			})
		}
	}
}

func TestRPTPendingLinksPreventUnobservedLoops(t *testing.T) {
	instance := setupRPTSafetyTest(t, func(map[string]string) string {
		return "Response: Success\r\nLinkedNodes:\r\n\r\n"
	})
	rptConnectionSafety.Lock()
	defer rptConnectionSafety.Unlock()
	rememberPendingRPTLink(instance, "50002", time.Minute)
	other := instance
	other.Node = "50002"
	rememberPendingRPTLink(other, "50003", time.Minute)
	if status, err := validateRPTConnection(context.Background(), instance, "50003"); status != http.StatusConflict || err == nil {
		t.Fatalf("pending chain must block closing a cycle: status=%d err=%v", status, err)
	}
	forgetPendingRPTLinks("50002", "50003")
	if _, err := validateRPTConnection(context.Background(), instance, "50003"); err != nil {
		t.Fatalf("disconnected pending edge still blocks: %v", err)
	}
	rememberPendingRPTLink(other, "50003", -time.Second)
	if _, err := validateRPTConnection(context.Background(), instance, "50003"); err != nil {
		t.Fatalf("expired pending edge still blocks: %v", err)
	}
}

func TestConnectionAPIsSerializeLinkChanges(t *testing.T) {
	setupRPTSafetyTest(t, func(map[string]string) string {
		t.Error("busy requests must not execute AMI")
		return ""
	})
	rptConnectionSafety.Lock()
	defer rptConnectionSafety.Unlock()
	for _, handler := range []echo.HandlerFunc{ExecuteSupermonCommand, AddNodeConnection} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
			`{"local_node":"50001","remote_node":"50002","action":"connect","mode":"permanent","permanent":true}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		if err := handler(echo.New().NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "operación de enlace en curso") {
			t.Fatalf("busy status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}

func TestRPTDisconnectRemainsAvailableWithoutTopology(t *testing.T) {
	var commands []string
	var mu sync.Mutex
	instance := setupRPTSafetyTest(t, func(fields map[string]string) string {
		mu.Lock()
		defer mu.Unlock()
		commands = append(commands, fields["Command"])
		return "Response: Follows\r\nOutput: OK\r\n--END COMMAND--\r\n\r\n"
	})
	rptConnectionSafety.Lock()
	rememberPendingRPTLink(instance, "50002", time.Minute)
	rptConnectionSafety.Unlock()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"local_node":"50001","remote_node":"50002","action":"disconnect"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := ExecuteSupermonCommand(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if rec.Code != http.StatusOK || !reflect.DeepEqual(commands, []string{"rpt cmd 50001 ilink 11 50002"}) {
		t.Fatalf("disconnect status=%d commands=%v", rec.Code, commands)
	}
	rptConnectionSafety.Lock()
	defer rptConnectionSafety.Unlock()
	if len(rptConnectionSafety.pending) != 0 {
		t.Fatal("disconnect must release pending reservations")
	}
}

func TestRPTSelfConnectionCannotDisconnectExistingLinks(t *testing.T) {
	setupRPTSafetyTest(t, func(map[string]string) string {
		t.Error("self connection must not query or modify AMI")
		return ""
	})
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		`{"local_node":"50001","remote_node":"50001","action":"connect","disconnect_before_connect":true}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := ExecuteSupermonCommand(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("self connection status=%d body=%s", rec.Code, rec.Body.String())
	}
}
