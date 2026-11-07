package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

func setupReconnectionTest(t *testing.T, respond func(map[string]string) string) {
	t.Helper()
	setupRPTSafetyTest(t, respond)
	config.App.DBPath = filepath.Join(filepath.Dir(config.App.AMI.RPTConfigPath), "test.db")
	if err := os.WriteFile(config.App.AMI.RPTConfigPath, []byte("[50001]\n[nodes]\n50001 = radio@127.0.0.1/50001,NONE\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func reconnectionRequest(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.GET("/entries", ListNodeReconnections)
	e.POST("/entries", SaveNodeReconnection)
	e.PUT("/entries/:id", SaveNodeReconnection)
	e.DELETE("/entries/:id", DeleteNodeReconnection)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestNodeReconnectionCRUDPreservesOtherEntriesAndConfiguration(t *testing.T) {
	var commands int
	setupReconnectionTest(t, func(fields map[string]string) string {
		commands++
		return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
	})
	body := `{"node":"50002","enabled":true,"interval_seconds":60,"action":"monitor","permanent":true}`
	if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 200 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 409 {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body)
	}
	if rec := reconnectionRequest(t, "POST", "/entries", strings.Replace(body, "50002", "3123456", 1)); rec.Code != 200 {
		t.Fatalf("EchoLink create: %d %s", rec.Code, rec.Body)
	}
	if rec := reconnectionRequest(t, "PUT", "/entries/1", `{"node":"50002","enabled":false,"interval_seconds":120,"action":"localmonitor","permanent":false}`); rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	entries, err := loadNodeReconnections(context.Background())
	if err != nil || len(entries) != 2 || entries[0].Enabled || entries[0].IntervalSeconds != 120 || entries[1].Node != "3123456" {
		t.Fatalf("unexpected entries: %+v %v", entries, err)
	}
	// Reopening the database must keep both client settings and existing application data.
	path := config.App.DBPath
	if _, err := config.App.DB.Exec("INSERT INTO favoritos (numero, descripcion) VALUES ('1998', 'client favorite')"); err != nil {
		t.Fatal(err)
	}
	if err := config.App.DB.Close(); err != nil {
		t.Fatal(err)
	}
	config.App.DB, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.App.DB.Close() })
	entries, err = loadNodeReconnections(context.Background())
	if err != nil || len(entries) != 2 || entries[0].Action != "localmonitor" || !entries[1].Permanent {
		t.Fatalf("persistence failed: %+v %v", entries, err)
	}
	if rec := reconnectionRequest(t, "DELETE", "/entries/1", ""); rec.Code != 200 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	rec := reconnectionRequest(t, "GET", "/entries", "")
	var response struct {
		Entries []nodeReconnection `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || len(response.Entries) != 1 || response.Entries[0].Node != "3123456" {
		t.Fatalf("deleted wrong entry: %s %v", rec.Body, err)
	}
	var favorites int
	if err := config.App.DB.QueryRow("SELECT COUNT(*) FROM favoritos WHERE numero = '1998'").Scan(&favorites); err != nil || favorites != 1 {
		t.Fatalf("client data changed: %d %v", favorites, err)
	}
	if commands != 0 {
		t.Fatal("CRUD must not connect, change mode or disconnect")
	}
}

func TestNodeReconnectionValidation(t *testing.T) {
	setupReconnectionTest(t, func(fields map[string]string) string { return "" })
	for _, body := range []string{
		`{`, `{"node":"50002;restart","interval_seconds":60,"action":"connect"}`,
		`{"node":"12345678901","interval_seconds":60,"action":"connect"}`,
		`{"node":"50002","interval_seconds":0,"action":"connect"}`,
		`{"node":"50002","interval_seconds":86401,"action":"connect"}`,
		`{"node":"50002","interval_seconds":1.5,"action":"connect"}`,
		`{"node":"50002","interval_seconds":60,"action":"disconnect"}`,
	} {
		if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 400 {
			t.Fatalf("accepted %s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if rec := reconnectionRequest(t, "POST", "/entries", `{"node":"50001","interval_seconds":60,"action":"connect"}`); rec.Code != 409 {
		t.Fatalf("self accepted: %d %s", rec.Code, rec.Body)
	}
	for _, method := range []string{"PUT", "DELETE"} {
		body := `{"node":"50002","interval_seconds":60,"action":"connect"}`
		if rec := reconnectionRequest(t, method, "/entries/99", body); rec.Code != 404 {
			t.Fatalf("missing entry: %d %s", rec.Code, rec.Body)
		}
		if rec := reconnectionRequest(t, method, "/entries/bad", body); rec.Code != 400 {
			t.Fatalf("invalid ID: %d %s", rec.Code, rec.Body)
		}
	}
}

func TestNodeReconnectionUsesAllSixModesAndEchoLink(t *testing.T) {
	for _, action := range []string{"connect", "monitor", "localmonitor"} {
		for _, permanent := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", action, permanent), func(t *testing.T) {
				var commands []string
				var mu sync.Mutex
				setupReconnectionTest(t, func(fields map[string]string) string {
					mu.Lock()
					defer mu.Unlock()
					if fields["Action"] == "RptStatus" {
						return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
					}
					commands = append(commands, fields["Command"])
					return "Response: Follows\r\nOutput: Command sent\r\n--END COMMAND--\r\n\r\n"
				})
				body := fmt.Sprintf(`{"node":"3123456","enabled":true,"interval_seconds":60,"action":%q,"permanent":%t}`, action, permanent)
				if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 200 {
					t.Fatalf("create: %s", rec.Body)
				}
				runNodeReconnectionCycle(context.Background(), time.Now())
				mu.Lock()
				defer mu.Unlock()
				codes := map[string][2]int{"connect": {3, 13}, "monitor": {2, 12}, "localmonitor": {8, 18}}
				code := codes[action][0]
				if permanent {
					code = codes[action][1]
				}
				want := fmt.Sprintf("rpt cmd 50001 ilink %d 3123456", code)
				if len(commands) != 1 || commands[0] != want {
					t.Fatalf("commands=%v want=%s", commands, want)
				}
				entries, err := loadNodeReconnections(context.Background())
				if err != nil || entries[0].LastResult != "RECONNECT_REQUESTED" {
					t.Fatalf("unconfirmed command misreported: %+v %v", entries, err)
				}
			})
		}
	}
}

func TestNodeReconnectionDoesNotTouchExistingLinksAndRejectsUnknownStatus(t *testing.T) {
	for _, test := range []struct {
		name, raw, result string
	}{
		{"connected in another mode", "Response: Success\r\nConn: 50002 127.0.0.1 0 IN 00:01 ESTABLISHED\r\nLinkedNodes: R50002\r\n\r\n", "RECONNECT_PRESENT"},
		{"connecting", "Response: Success\r\nConn: 50002 127.0.0.1 OUT 00:00\r\nLinkedNodes: C50002\r\n\r\n", "RECONNECT_PRESENT"},
		{"no list", "Response: Success\r\n\r\n", "RECONNECT_STATUS_ERROR"},
		{"AMI failure", "Response: Error\r\nMessage: status failed\r\n\r\n", "RECONNECT_STATUS_ERROR"},
		{"indirect route", "Response: Success\r\nLinkedNodes: T50002\r\n\r\n", "RECONNECT_REJECTED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var commands int
			setupReconnectionTest(t, func(fields map[string]string) string {
				if fields["Action"] != "RptStatus" {
					commands++
				}
				return test.raw
			})
			result, _ := checkNodeReconnection(context.Background(), nodeReconnection{Node: "50002", Action: "connect", Permanent: true})
			if result != test.result || commands != 0 {
				t.Fatalf("result=%s commands=%d", result, commands)
			}
		})
	}
}

func TestNodeReconnectionIntervalsDisabledAndExactMatching(t *testing.T) {
	var commands []string
	var mu sync.Mutex
	setupReconnectionTest(t, func(fields map[string]string) string {
		mu.Lock()
		defer mu.Unlock()
		if fields["Action"] == "RptStatus" {
			// 500020 must not match 50002, unlike substring grep.
			return "Response: Success\r\nConn: 500020 127.0.0.1 OUT 00:01\r\nConn: SIP/1001 127.0.0.1 IN 00:01\r\nLinkedNodes: T500020\r\n\r\n"
		}
		commands = append(commands, fields["Command"])
		return "Response: Success\r\n\r\n"
	})
	for _, body := range []string{
		`{"node":"50002","enabled":true,"interval_seconds":60,"action":"connect"}`,
		`{"node":"50003","enabled":false,"interval_seconds":1,"action":"connect"}`,
		`{"node":"50004","enabled":true,"interval_seconds":120,"action":"monitor"}`,
	} {
		if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 200 {
			t.Fatalf("create: %s", rec.Body)
		}
	}
	future := time.Now().Add(time.Hour).Unix()
	if _, err := config.App.DB.Exec("UPDATE node_reconnections SET next_check_at = ? WHERE node = '50004'", future); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	runNodeReconnectionCycle(context.Background(), now)
	runNodeReconnectionCycle(context.Background(), now.Add(time.Second))
	mu.Lock()
	defer mu.Unlock()
	if len(commands) != 1 || commands[0] != "rpt cmd 50001 ilink 3 50002" {
		t.Fatalf("wrong matching or interval: %v", commands)
	}
	entries, err := loadNodeReconnections(context.Background())
	if err != nil || entries[0].NextCheckAt < now.Unix()+60 || entries[1].LastCheckAt != 0 || entries[2].NextCheckAt != future {
		t.Fatalf("intervals not preserved: %+v %v", entries, err)
	}
}

func TestNodeReconnectionBusyPendingAndCommandFailure(t *testing.T) {
	setupReconnectionTest(t, func(fields map[string]string) string {
		if fields["Action"] == "RptStatus" {
			return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
		}
		return "Response: Error\r\nMessage: rejected\r\n\r\n"
	})
	entry := nodeReconnection{Node: "50002", Action: "connect"}
	rptConnectionSafety.Lock()
	result, _ := checkNodeReconnection(context.Background(), entry)
	rptConnectionSafety.Unlock()
	if result != "RECONNECT_BUSY" {
		t.Fatalf("did not serialize: %s", result)
	}
	result, _ = checkNodeReconnection(context.Background(), entry)
	if result != "RECONNECT_COMMAND_ERROR" {
		t.Fatalf("failed command accepted: %s", result)
	}
	rptConnectionSafety.Lock()
	ami := config.CurrentAMI()
	rememberPendingRPTLink(allmonInstance{Node: "50001", Host: ami.Host, Port: ami.Port}, entry.Node, time.Minute)
	rptConnectionSafety.Unlock()
	result, _ = checkNodeReconnection(context.Background(), entry)
	if result != "RECONNECT_REJECTED" {
		t.Fatalf("duplicated pending link: %s", result)
	}
}

func TestNodeReconnectionRechecksEditedOrDeletedEntries(t *testing.T) {
	var queries int
	setupReconnectionTest(t, func(fields map[string]string) string {
		queries++
		return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
	})
	body := `{"node":"50002","enabled":true,"interval_seconds":60,"action":"connect"}`
	if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 200 {
		t.Fatalf("create: %s", rec.Body)
	}
	entries, err := loadNodeReconnections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rec := reconnectionRequest(t, "PUT", "/entries/1", strings.Replace(body, "true", "false", 1)); rec.Code != 200 {
		t.Fatalf("disable: %s", rec.Body)
	}
	runNodeReconnectionEntry(context.Background(), entries[0], time.Now())
	if rec := reconnectionRequest(t, "DELETE", "/entries/1", ""); rec.Code != 200 {
		t.Fatalf("delete: %s", rec.Body)
	}
	runNodeReconnectionEntry(context.Background(), entries[0], time.Now())
	if queries != 0 {
		t.Fatal("stale snapshot connected a disabled or deleted entry")
	}
	nodeReconnectionMu.Lock()
	defer nodeReconnectionMu.Unlock()
	if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 409 {
		t.Fatalf("concurrent save was not rejected: %d %s", rec.Code, rec.Body)
	}
}

func TestNodeReconnectionSelectedSourcesAndCredentials(t *testing.T) {
	var localCommands, remoteCommands []string
	var remoteQueries []string
	var mu sync.Mutex
	setupReconnectionTest(t, func(fields map[string]string) string {
		mu.Lock()
		defer mu.Unlock()
		if fields["Action"] != "RptStatus" {
			localCommands = append(localCommands, fields["Command"])
		}
		return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
	})
	_, port := mockConnectionAMI(t, func(fields map[string]string) string {
		mu.Lock()
		defer mu.Unlock()
		if fields["Action"] == "RptStatus" {
			remoteQueries = append(remoteQueries, fields["Node"])
		} else {
			remoteCommands = append(remoteCommands, fields["Command"])
		}
		return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
	})
	// A hostname alias represents a distinct Allmon host while keeping AMI inside the test.
	host := "localhost."
	content := fmt.Sprintf("[70001]\nsystem=Nodes\nhost=%s\nport=%s\nuser=remote-user\npasswd=remote-secret\n", host, port)
	if err := os.WriteFile(config.App.Sources.AllmonINIPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	instance, err := resolveReconnectionSource("70001")
	if err != nil || instance.Host != host || instance.Port != port || instance.User != "remote-user" || instance.Secret != "remote-secret" {
		t.Fatalf("wrong source credentials: %+v %v", instance, err)
	}
	// Allmon permits empty AMI passwords, just as the manual connection path does.
	if err := os.WriteFile(config.App.Sources.AllmonINIPath, []byte(strings.Replace(content, "passwd=remote-secret", "passwd=", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	instance, err = resolveReconnectionSource("70001")
	if err != nil || instance.Secret != "" {
		t.Fatalf("empty remote password rejected: %+v %v", instance, err)
	}
	body := `{"source_node":"70001","node":"3123456","enabled":true,"interval_seconds":60,"action":"connect"}`
	if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 200 {
		t.Fatalf("remote source save: %d %s", rec.Code, rec.Body)
	}
	if rec := reconnectionRequest(t, "POST", "/entries", strings.Replace(body, "70001", "50001", 1)); rec.Code != 200 {
		t.Fatalf("same target from another source: %d %s", rec.Code, rec.Body)
	}
	if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 409 {
		t.Fatalf("duplicate pair: %d %s", rec.Code, rec.Body)
	}
	if rec := reconnectionRequest(t, "POST", "/entries", strings.Replace(body, `"source_node":"70001",`, "", 1)); rec.Code != 409 {
		t.Fatalf("legacy primary alias bypassed duplicate protection: %d %s", rec.Code, rec.Body)
	}
	runNodeReconnectionCycle(context.Background(), time.Now())
	mu.Lock()
	if len(remoteCommands) != 1 || remoteCommands[0] != "rpt cmd 70001 ilink 3 3123456" {
		t.Fatalf("remote source command: %v", remoteCommands)
	}
	// The shared topology guard can defer the second pair: no command from the wrong source is allowed.
	for _, command := range localCommands {
		if command != "rpt cmd 50001 ilink 3 3123456" {
			t.Fatalf("wrong local source: %v", localCommands)
		}
	}
	for _, node := range remoteQueries {
		if node != "70001" {
			t.Fatalf("wrong source status query: %v", remoteQueries)
		}
	}
	mu.Unlock()
	entries, err := loadNodeReconnections(context.Background())
	if err != nil || len(entries) != 2 || entries[0].SourceNode != "70001" {
		t.Fatalf("source did not persist: %+v %v", entries, err)
	}
	// Removing a source never redirects its timer to the principal.
	if err := os.Remove(config.App.Sources.AllmonINIPath); err != nil {
		t.Fatal(err)
	}
	result, _ := checkNodeReconnection(context.Background(), entries[0])
	if result != "RECONNECT_SOURCE_ERROR" {
		t.Fatalf("missing source silently replaced: %s", result)
	}
	if rec := reconnectionRequest(t, "PUT", "/entries/1", strings.Replace(body, "true", "false", 1)); rec.Code != 200 {
		t.Fatalf("could not disable removed source: %d %s", rec.Code, rec.Body)
	}
}

func TestNodeReconnectionPrivateSourceAndSourceValidation(t *testing.T) {
	var commands []string
	setupReconnectionTest(t, func(fields map[string]string) string {
		if fields["Action"] != "RptStatus" {
			commands = append(commands, fields["Command"])
		}
		return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
	})
	content := "[50001]\n[1998]\n; amerimon_show_as_server = yes\n[nodes]\n50001 = radio@127.0.0.1/50001,NONE\n1998 = radio@127.0.0.1/1998,NONE\n"
	if err := os.WriteFile(config.App.AMI.RPTConfigPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	body := `{"source_node":"1998","node":"50002","enabled":true,"interval_seconds":60,"action":"monitor"}`
	if rec := reconnectionRequest(t, "POST", "/entries", body); rec.Code != 200 {
		t.Fatalf("private source save: %d %s", rec.Code, rec.Body)
	}
	runNodeReconnectionCycle(context.Background(), time.Now())
	if len(commands) != 1 || commands[0] != "rpt cmd 1998 ilink 2 50002" {
		t.Fatalf("private source command: %v", commands)
	}
	if rec := reconnectionRequest(t, "POST", "/entries", strings.Replace(body, "50002", "1998", 1)); rec.Code != 409 {
		t.Fatalf("self link accepted: %d %s", rec.Code, rec.Body)
	}
	if rec := reconnectionRequest(t, "POST", "/entries", strings.Replace(body, "1998", "99999", 1)); rec.Code != 503 {
		t.Fatalf("unknown source accepted: %d %s", rec.Code, rec.Body)
	} else {
		var response struct {
			Message     string `json:"message"`
			Source      string `json:"source_node"`
			Destination string `json:"destination_node"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil ||
			response.Message != "RECONNECT_SOURCE_SAVE_ERROR" || response.Source != "99999" || response.Destination != "50002" {
			t.Fatalf("source error lacks context: %s %v", rec.Body, err)
		}
	}
	if rec := reconnectionRequest(t, "POST", "/entries", strings.Replace(body, "1998", "bad;cmd", 1)); rec.Code != 400 {
		t.Fatalf("invalid source accepted: %d %s", rec.Code, rec.Body)
	}
}

func TestNodeReconnectionSchedulerStopsAndTracksChangedPrimary(t *testing.T) {
	setupReconnectionTest(t, func(fields map[string]string) string {
		return "Response: Success\r\nLinkedNodes: <NONE>\r\n\r\n"
	})
	// Changing the principal to the destination must never request a self-link.
	if err := os.WriteFile(config.App.AMI.RPTConfigPath, []byte("[50002]\n[nodes]\n50002 = radio@127.0.0.1/50002,NONE\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, _ := checkNodeReconnection(context.Background(), nodeReconnection{Node: "50002", Action: "connect"})
	if result != "RECONNECT_SELF" {
		t.Fatalf("stale primary: %s", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := StartNodeReconnectionScheduler(ctx)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop")
	}
	// An unreadable primary must be reported, not silently replaced with a default.
	config.App.AMI.RPTConfigPath = filepath.Join(t.TempDir(), "missing.conf")
	result, _ = checkNodeReconnection(context.Background(), nodeReconnection{Node: "50003", Action: "connect"})
	if result != "RECONNECT_SOURCE_ERROR" {
		t.Fatalf("missing primary ignored: %s", result)
	}
}
