package controllers

// Este archivo contiene pruebas para el descubrimiento y las operaciones de nodos AllStarLink.

import (
	"amerimon/internal/config"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestSIPRXLinksAreNotIAXConnections(t *testing.T) {
	connections := parseRPTConnections(
		"Conn: SIP/phone-portal 127.0.0.1 0 IN 00:01:00 ESTABLISHED\nConn: 700001 127.0.0.1 0 IN 00:01:00 ESTABLISHED",
		"Conn: SIP/phone-portal 1 0 -1")
	if len(connections) != 1 || connections[0].Node != "700001" {
		t.Fatalf("named SIP link leaked into IAX table/actions/history: %+v", connections)
	}
}

func TestAllStarLinkDiscoveryDoesNotConnectToAMI(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var connections atomic.Int32
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			conn.Close()
		}
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	rptPath := filepath.Join(dir, "rpt.conf")
	allmonPath := filepath.Join(dir, "allmon.ini")
	astdbPath := filepath.Join(dir, "astdb.txt")
	if err := os.WriteFile(astdbPath, []byte("700001|EA4ABC|Servidor remoto|Madrid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rptPath, []byte("[616265]\n[nodes]\n616265 = radio@127.0.0.1/616265,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(allmonPath, []byte(fmt.Sprintf(
		"[700001]\nsystem=Nodes\nhost=192.0.2.10\nport=%s\nuser=remote\npasswd=test-secret\n", port)), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := config.App
	defer func() { config.App = previous }()
	config.App.AMI.RPTConfigPath = rptPath
	config.App.AMI.Host, config.App.AMI.Port = host, port
	config.App.AMI.User, config.App.AMI.Secret = "test-user", "test-secret"
	config.App.Sources.AllmonINIPath = allmonPath
	config.App.Sources.AstDBPath = astdbPath
	config.App.Sources.PrivateNodesPath = filepath.Join(dir, "privatenodes.txt")

	for _, query := range []string{"discovery=true", "discovery=true&local_node=700001"} {
		req := httptest.NewRequest(http.MethodGet, "/allstarlink/nodes?"+query, nil)
		rec := httptest.NewRecorder()
		start := time.Now()
		if err := GetAllStarLinkNodes(echo.New().NewContext(req, rec)); err != nil {
			t.Fatal(err)
		}
		if elapsed := time.Since(start); elapsed >= 500*time.Millisecond {
			t.Fatalf("discovery waited for AMI: %v", elapsed)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("discovery returned %d: %s", rec.Code, rec.Body.String())
		}
		var response struct {
			Status string `json:"status"`
			Data   struct {
				Primary string                  `json:"nodo_principal"`
				Tables  []AllStarLinkLocalTable `json:"tablas"`
				Rows    []AllStarLinkNode       `json:"resultados"`
			} `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		expectedTables := 2
		if strings.Contains(query, "local_node") {
			expectedTables = 1
		}
		if response.Status != "success" || response.Data.Primary != "616265" ||
			len(response.Data.Tables) != expectedTables || len(response.Data.Rows) != 0 {
			t.Fatalf("unexpected discovery response: %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "test-secret") || strings.Contains(rec.Body.String(), "test-user") {
			t.Fatal("discovery exposed AMI credentials")
		}
		for _, table := range response.Data.Tables {
			if table.NodoLocal == "700001" && (table.Descripcion != "Servidor remoto" || table.IP != "192.0.2.10" || table.Hostname != "") {
				t.Fatalf("remote metadata missing from discovery: %+v", table)
			}
		}
	}
	if connections.Load() != 0 {
		t.Fatalf("discovery opened %d AMI connections", connections.Load())
	}
}

func TestAllStarLinkTableHostMetadata(t *testing.T) {
	tables := buildAllStarLinkLocalTables([]allmonInstance{
		{Node: "700001", Host: "radio.example.org", User: "private-user", Secret: "private-secret"},
		{Node: "700002", Host: "2001:db8::1"},
	}, nil, nil)
	if tables[0].Hostname != "radio.example.org" || tables[0].IP != "" {
		t.Fatalf("unexpected hostname metadata: %+v", tables[0])
	}
	if tables[1].IP != "2001:db8::1" || tables[1].Hostname != "" {
		t.Fatalf("unexpected IPv6 metadata: %+v", tables[1])
	}
	raw, err := json.Marshal(tables)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private-") {
		t.Fatal("table metadata exposed AMI credentials")
	}
}

func TestLocalAllmonInstancesUseRPTConfiguration(t *testing.T) {
	dir := t.TempDir()
	rptPath := filepath.Join(dir, "rpt.conf")
	allmonPath := filepath.Join(dir, "allmon.ini")
	if err := os.WriteFile(rptPath, []byte("[616265]\n[nodes]\n616265 = radio@127.0.0.1/616265,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(allmonPath, []byte(`[616262]
system=Nodes
host=127.0.0.1
user=old-admin
passwd=old-secret
[700001]
system=Nodes
host=192.0.2.10
user=remote-admin
passwd=remote-secret
`), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := config.App
	defer func() { config.App = previous }()
	config.App.AMI.RPTConfigPath = rptPath
	config.App.AMI.Host = "127.0.0.1"
	config.App.AMI.Port = "5038"
	config.App.AMI.User = "admin"
	config.App.AMI.Secret = "current-secret"
	config.App.Sources.AllmonINIPath = allmonPath

	instances, err := allStarLinkActivityInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 2 || instances[0].Node != "616265" || instances[1].Node != "700001" {
		t.Fatalf("expected RPT local node and preserved remote server, got %+v", instances)
	}
	if instances[0].User != "admin" || instances[0].Secret != "current-secret" {
		t.Fatal("local node did not use global AMI credentials")
	}
	if instances[1].User != "remote-admin" || instances[1].Secret != "remote-secret" {
		t.Fatal("remote AMI configuration changed")
	}
	instance, configured, err := findAllmonInstance("616265")
	if err != nil || !configured || instance.Node != "616265" {
		t.Fatalf("control lookup did not resolve RPT local node: configured=%v error=%v", configured, err)
	}
	if err := os.Remove(rptPath); err != nil {
		t.Fatal(err)
	}
	if _, err := allStarLinkActivityInstances(); err == nil {
		t.Fatal("missing rpt.conf must not fall back to stale Allmon nodes")
	}
}

func TestReadPrimaryRPTNode(t *testing.T) {
	config := `[nodes]
	616261 = radio@192.168.1.10/616261,NONE
	616262 = radio@127.0.0.1/616262,NONE ; primary node
	616263 = radio@192.168.1.10/616263,NONE
`
	path := filepath.Join(t.TempDir(), "rpt.conf")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	node, err := readPrimaryRPTNode(path)
	if err != nil {
		t.Fatal(err)
	}
	if node != "616262" {
		t.Fatalf("expected primary node 616262, got %q", node)
	}
}

func TestReadPrimaryRPTNodeNotConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rpt.conf")
	if err := os.WriteFile(path, []byte("[nodes]\n616263 = radio@192.168.1.10/616263,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	node, err := readPrimaryRPTNode(path)
	if err != nil {
		t.Fatal(err)
	}
	if node != "" {
		t.Fatalf("expected no primary node, got %q", node)
	}
}

func TestPrivateNodeServerVisibilityKeepsPrimaryAndUsesGlobalAMI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rpt.conf")
	content := "[1998](node-main)\n; amerimon_show_as_server = yes\n" +
		"[1964](node-main)\n[616265](node-main)\n[nodes]\n" +
		"1998=radio@127.0.0.1/1998,NONE\n616265=radio@127.0.0.1/616265,NONE\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	privatePath := filepath.Join(dir, "privatenodes.txt")
	if err := os.WriteFile(privatePath, []byte("1998|PRIVATE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := config.App
	t.Cleanup(func() { config.App = previous })
	config.App.AMI.RPTConfigPath = path
	config.App.AMI.Host = "127.0.0.1"
	config.App.AMI.User = "global-user"
	config.App.AMI.Secret = "global-secret"
	config.App.Sources.PrivateNodesPath = privatePath
	for _, configured := range [][]allmonInstance{nil, {
		{Node: "1998", Host: "127.0.0.1", User: "wrong", Secret: "wrong"},
	}} {
		instances, err := reconcileLocalAllmonInstances(configured)
		if err != nil || len(instances) != 2 || instances[0].Node != "616265" || instances[1].Node != "1998" {
			t.Fatalf("incorrect private server selection: %#v, %v", instances, err)
		}
		if instances[1].User != "global-user" || instances[1].Secret != "global-secret" {
			t.Fatal("private server must use global AMI credentials")
		}
		tables := buildAllStarLinkLocalTables(instances, nil, map[string]bool{"616265": true, "1998": true})
		if len(tables) != 2 {
			t.Fatalf("missing private server table: %#v", tables)
		}
	}
}

func TestPrivateNodesAreConnectionsNotServers(t *testing.T) {
	for _, withAllmon := range []bool{false, true} {
		t.Run(fmt.Sprint(withAllmon), func(t *testing.T) {
			dir := t.TempDir()
			rptPath := filepath.Join(dir, "rpt.conf")
			privatePath := filepath.Join(dir, "privatenodes.txt")
			rpt := `[1964]
	[1998]
	[616265](node-main)
	[612345]
	[nodes]
	1998 = radio@127.0.0.1/1998,NONE
	612345 = radio@127.0.0.1/612345,NONE
	616265 = radio@127.0.0.1/616265,NONE
	[functions]
	616299 = radio@127.0.0.1/616299,NONE
	`
			if err := os.WriteFile(rptPath, []byte(rpt), 0o600); err != nil {
				t.Fatal(err)
			}
			// 1998 deliberately absent: use the existing private-node category too.
			if err := os.WriteFile(privatePath, []byte("1964|PRIVATE1\n612345|PRIVATE2\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			previous := config.App
			defer func() { config.App = previous }()
			config.App.AMI.RPTConfigPath = rptPath
			config.App.AMI.Host = "127.0.0.1"
			config.App.Sources.PrivateNodesPath = privatePath
			var configured []allmonInstance
			if withAllmon {
				configured = []allmonInstance{
					{Node: "1964", Host: "192.0.2.1"},
					{Node: "1998", Host: "127.0.0.1"},
					{Node: "612345", Host: "192.0.2.2"},
					{Node: "700001", Host: "192.0.2.3", User: "remote-user", Secret: "remote-secret"},
				}
			}
			primary, err := readPrimaryRPTNode(rptPath)
			if err != nil || primary != "616265" {
				t.Fatalf("primary = %q, error = %v", primary, err)
			}
			instances, err := reconcileLocalAllmonInstances(configured)
			if err != nil {
				t.Fatal(err)
			}
			expectedCount := 1
			if withAllmon {
				expectedCount++
			}
			if len(instances) != expectedCount || instances[0].Node != primary {
				t.Fatalf("private nodes became servers: %+v", instances)
			}
			if withAllmon && (instances[1].Node != "700001" || instances[1].Secret != "remote-secret") {
				t.Fatalf("remote configuration changed: %+v", instances[1])
			}
			connections := parseRPTConnections(
				"Conn: 1964 127.0.0.1 1 OUT 00:01:00 ESTABLISHED\nConn: 1998 127.0.0.1 1 OUT 00:02:00 ESTABLISHED\n", "")
			rows := make([]AllStarLinkNode, 0, len(connections))
			for _, connection := range connections {
				rows = append(rows, AllStarLinkNode{NodoLocal: primary, Nodo: connection.Node, Protocolo: "iax"})
			}
			tables := buildAllStarLinkLocalTables(instances, rows, map[string]bool{primary: true})
			if len(rows) != 2 || len(tables) != expectedCount || tables[0].NodoLocal != primary ||
				len(tables[0].NodosDisponibles) != 2 {
				t.Fatalf("private connections lost or given separate tables: rows=%+v tables=%+v", rows, tables)
			}
		})
	}
}

func TestPrimaryRPTNodeDoesNotChooseFirstAmbiguousMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rpt.conf")
	if err := os.WriteFile(path, []byte("[nodes]\n616265 = radio@127.0.0.1/616265,NONE\n616262 = radio@127.0.0.1/616262,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if node, err := readPrimaryRPTNode(path); err == nil || node != "" {
		t.Fatalf("ambiguous config selected %q with error %v", node, err)
	}
}
