package controllers

// Este archivo contiene pruebas para las operaciones principales de control del nodo local.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

type nodeTestDB map[string]string

func (db nodeTestDB) command(command string) (string, error) {
	fields := strings.Fields(command)
	switch fields[1] {
	case "show":
		var lines []string
		for key, reason := range db {
			if strings.HasPrefix(key, fields[2]+"/") {
				lines = append(lines, "Output: /"+key+" : "+reason)
			}
		}
		return strings.Join(lines, "\n") + fmt.Sprintf("\nOutput: %d results found.\n", len(lines)), nil
	case "put":
		key := fields[2] + "/" + fields[3]
		db[key] = strings.Trim(strings.TrimPrefix(command, strings.Join(fields[:4], " ")+" "), "\"")
		return "Output: Updated database successfully", nil
	case "del":
		delete(db, fields[2]+"/"+fields[3])
		return "Output: Database entry removed.", nil
	default:
		return "", fmt.Errorf("unexpected test command")
	}
}

func primaryNodeFixture(t *testing.T) (primaryNodeControl, nodeTestDB, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	previous := config.App
	t.Cleanup(func() { config.App = previous })
	config.App.Sources.PrivateNodesPath = filepath.Join(dir, "privatenodes.txt")
	config.App.Path = filepath.Join(dir, "amerimon.yaml")
	config.App.EnvPath = filepath.Join(dir, "amerimon.env")
	config.App.AMI = config.AMIConfig{Host: "127.0.0.1", Port: "5038", User: "amerimon", Secret: "old_secret"}
	for path, data := range map[string]string{
		filepath.Join(dir, "manager.conf"): "[general]\nbindaddr = 127.0.0.1\n[amerimon]\nsecret = old_secret\n",
		config.App.Path:                    "sources:\n  astdb_path: /var/lib/asterisk/astdb.txt\nami:\n  host: 127.0.0.1\n  port: \"5038\"\n  user: ${AMI_USER}\n  secret: ${AMI_SECRET}\n",
		config.App.EnvPath:                 "AMI_USER=\"amerimon\"\nAMI_SECRET=\"old_secret\"\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"rpt.conf":                    "[616265](node-main)\nidrecording = |iOLD1\nduplex = 1\nrxchannel = SimpleUSB/616265\nstatpost_url = http://stats.example/\n[700001](node-main)\nidrecording = |iOTHER\n[nodes]\n616265 = radio@127.0.0.1/616265,NONE\n700001 = radio@192.0.2.1/700001,NONE\n",
		"rpt_http_registrations.conf": "[registrations]\nregister => 616265:old_password@register.allstarlink.org\nregister => 700001:other_password@register.allstarlink.org\n",
		"iax.conf":                    "[general]\n;register => 616265:old_password@register.allstarlink.org\nbindport=4569\n[radio]\nsecret=unrelated\n",
		"savenode.conf":               "NODE=616265\nPASSWORD=old_password\nENABLE=0\n",
		"extensions.conf":             "[globals]\nNODE = 616265\nOTHER = 616265\n",
		"simpleusb.conf":              "[616265](node-main)\ndevstr=1-2\n[700001]\ndevstr=1-3\n",
		"usbradio.conf":               "[general]\n[616265]\ndevstr=1-2\n",
		"voter.conf":                  "[616265]\npassword=unrelated\n",
		"allmon.ini":                  "[616265]\nsystem=Nodes\nhost=127.0.0.1\n[700001]\nhost=192.0.2.1\n",
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	db := nodeTestDB{
		"allowlist/616265/448511": "Permitido para pruebas",
		"denylist/616265/TEST2":   "Bloqueado",
		"denylist/700001/9999":    "Otro servidor",
	}
	return primaryNodeControl{
		rptPath: filepath.Join(dir, "rpt.conf"), allmonPaths: []string{filepath.Join(dir, "allmon.ini")},
		managerPath: filepath.Join(dir, "manager.conf"), ami: func(command string) (string, error) {
			if command == "module reload app_rpt.so" {
				return "Module 'app_rpt.so' reloaded successfully.", nil
			}
			return db.command(command)
		},
	}, db, files
}

func nodeTestContext(method string, body any) (echo.Context, *httptest.ResponseRecorder) {
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "/primary-node", strings.NewReader(string(data)))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return echo.New().NewContext(req, rec), rec
}

func assertNodeFile(t *testing.T, dir, name, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || string(data) != want {
		t.Fatalf("%s mismatch: %v, got %q want %q", name, err, data, want)
	}
}

func TestPrimaryNodeReadReturnsOnlyPrimaryPasswordWithoutCaching(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	c, rec := nodeTestContext(http.MethodGet, nil)
	if err := p.get(c); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Node, Callsign string
		PasswordSet    bool   `json:"password_set"`
		Password       string `json:"password"`
		Duplex         int    `json:"duplex"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Node != "616265" || got.Callsign != "OLD1" || !got.PasswordSet || got.Duplex != 1 {
		t.Fatalf("unexpected response: %s", rec.Body)
	}
	if got.Password != "old_password" || strings.Contains(rec.Body.String(), "other_password") {
		t.Fatal("GET returned the wrong node password")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("password response must not be cached")
	}
}

func TestPrimaryNodePasswordPrefersActiveHTTPAndFallsBackToIAX(t *testing.T) {
	for _, tc := range []struct{ name, http, want string }{
		{"active-http", "[registrations]\nregister => 616265:http_password@register.allstarlink.org\n", "http_password"},
		{"commented-http", "[registrations]\n;register => 616265:disabled_password@register.allstarlink.org\n", "old_password"},
		{"wrong-section", "[other]\nregister => 616265:unrelated_password@register.allstarlink.org\n", "old_password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, _ := primaryNodeFixture(t)
			if err := os.WriteFile(filepath.Join(filepath.Dir(p.rptPath), "rpt_http_registrations.conf"), []byte(tc.http), 0o600); err != nil {
				t.Fatal(err)
			}
			c, rec := nodeTestContext(http.MethodGet, nil)
			if err := p.get(c); err != nil {
				t.Fatal(err)
			}
			var got struct {
				Password string `json:"password"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Password != tc.want {
				t.Fatal("incorrect registration password selected")
			}
		})
	}
}

func TestPrimaryNodeSaveRenamesAllReferencesAndMigratesAccess(t *testing.T) {
	p, db, files := primaryNodeFixture(t)
	reloads := 0
	ami := p.ami
	p.ami = func(command string) (string, error) {
		if command == "module reload app_rpt.so" {
			reloads++
		}
		return ami(command)
	}
	duplex := 2
	c, rec := nodeTestContext(http.MethodPut, primaryNodeRequest{CurrentNode: "616265", Node: "616266", Callsign: "new2", Password: "new_password", Duplex: &duplex})
	if err := p.save(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"restart_required":false`) {
		t.Fatalf("save failed: %s", rec.Body)
	}
	if reloads != 1 {
		t.Fatalf("expected one app_rpt reload, got %d", reloads)
	}
	dir := filepath.Dir(p.rptPath)
	for name, original := range files {
		assertNodeFile(t, dir, name+".amerimon.bak", original)
		info, err := os.Stat(filepath.Join(dir, name+".amerimon.bak"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("backup permissions: %v", err)
		}
		info, err = os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("file permissions: %v", err)
		}
	}
	assertNodeFile(t, dir, "rpt.conf", strings.NewReplacer("[616265]", "[616266]", "|iOLD1", "|iNEW2", "duplex = 1", "duplex = 2", "SimpleUSB/616265", "SimpleUSB/616266", "616265 = radio@127.0.0.1/616265", "616266 = radio@127.0.0.1/616266").Replace(files["rpt.conf"]))
	assertNodeFile(t, dir, "rpt_http_registrations.conf", strings.ReplaceAll(files["rpt_http_registrations.conf"], "616265:old_password", "616266:new_password"))
	assertNodeFile(t, dir, "iax.conf", strings.ReplaceAll(files["iax.conf"], "616265:old_password", "616266:new_password"))
	assertNodeFile(t, dir, "savenode.conf", "NODE=616266\nPASSWORD=new_password\nENABLE=1\n")
	assertNodeFile(t, dir, "extensions.conf", "[globals]\nNODE = 616266\nOTHER = 616265\n")
	if db["allowlist/616266/448511"] != "Permitido para pruebas" || db["denylist/616266/TEST2"] != "Bloqueado" || db["denylist/700001/9999"] != "Otro servidor" {
		t.Fatalf("access migration mismatch: %+v", db)
	}
	if _, exists := db["allowlist/616265/448511"]; exists {
		t.Fatal("old access remains")
	}
	node, err := p.primary()
	if err != nil || node != "616266" {
		t.Fatalf("renamed primary not discoverable: %s %v", node, err)
	}
}

func TestPrimaryNodeBlankPasswordPreservesAllSecrets(t *testing.T) {
	p, _, files := primaryNodeFixture(t)
	c, _ := nodeTestContext(http.MethodPut, primaryNodeRequest{CurrentNode: "616265", Node: "616265", Callsign: "NEW2"})
	if err := p.save(c); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"iax.conf", "rpt_http_registrations.conf", "savenode.conf"} {
		assertNodeFile(t, filepath.Dir(p.rptPath), name, files[name])
	}
}

func TestPrimaryNodeFailedMigrationRestoresFilesAndAccess(t *testing.T) {
	p, db, files := primaryNodeFixture(t)
	originalDB := nodeTestDB{}
	for k, v := range db {
		originalDB[k] = v
	}
	failed := false
	p.ami = func(cmd string) (string, error) {
		if !failed && strings.HasPrefix(cmd, "database del allowlist/616265 ") {
			failed = true
			return "", errors.New("test failure")
		}
		return db.command(cmd)
	}
	c, _ := nodeTestContext(http.MethodPut, primaryNodeRequest{CurrentNode: "616265", Node: "616266", Callsign: "NEW2", Password: "new_password"})
	if err := p.save(c); err == nil {
		t.Fatal("expected migration error")
	}
	for name, original := range files {
		assertNodeFile(t, filepath.Dir(p.rptPath), name, original)
	}
	if !reflect.DeepEqual(db, originalDB) {
		t.Fatalf("access not restored: %+v", db)
	}
}

func TestPrimaryNodeCannotSaveWithoutBackups(t *testing.T) {
	p, _, files := primaryNodeFixture(t)
	if err := os.Mkdir(p.rptPath+".amerimon.bak", 0o700); err != nil {
		t.Fatal(err)
	}
	c, _ := nodeTestContext(http.MethodPut, primaryNodeRequest{CurrentNode: "616265", Node: "616265", Callsign: "NEW2"})
	if err := p.save(c); err == nil {
		t.Fatal("expected backup error")
	}
	assertNodeFile(t, filepath.Dir(p.rptPath), "rpt.conf", files["rpt.conf"])
}

func TestPrimaryNodeRejectsStaleStateAndExistingNumber(t *testing.T) {
	for _, tc := range []struct{ name, current, target string }{
		{"stale", "123456", "616266"}, {"duplicate", "616265", "700001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _, files := primaryNodeFixture(t)
			c, _ := nodeTestContext(http.MethodPut, primaryNodeRequest{CurrentNode: tc.current, Node: tc.target, Callsign: "NEW2"})
			if err := p.save(c); err == nil {
				t.Fatal("expected rejection")
			}
			for name, original := range files {
				assertNodeFile(t, filepath.Dir(p.rptPath), name, original)
			}
		})
	}
}

func TestPrimaryNodeValidations(t *testing.T) {
	outOfRange := 5
	for _, req := range []primaryNodeRequest{
		{Node: "1998", Callsign: "TEST1"},
		{Node: "016265", Callsign: "TEST1"},
		{Node: "616265", Callsign: "BAD\nCALL"},
		{Node: "616265", Callsign: "TEST1", Password: "bad@secret"},
		{Node: "616265", Callsign: "TEST1", Password: "short"},
		{Node: "616265", Callsign: "TEST1", Duplex: &outOfRange},
	} {
		if err := validatePrimaryNodeRequest(req); err == nil {
			t.Fatalf("accepted invalid configuration: node %s", req.Node)
		}
	}
	for _, entry := range []nodeAccessEntry{
		{Type: "evil", Node: "448511"},
		{Type: "allowlist", Node: "616265"},
		{Type: "denylist", Node: "BAD\nCALL"},
		{Type: "denylist", Node: "TEST1", Reason: "bad \"argument\""},
		{Type: "denylist", Node: "TEST1", Reason: "bad\ncommand"},
	} {
		if err := validateNodeAccess(entry, "616265"); err == nil {
			t.Fatal("accepted invalid access entry")
		}
	}
}

func TestPrimaryNodeReportsRejectedAppRptReload(t *testing.T) {
	p, db, _ := primaryNodeFixture(t)
	p.ami = func(command string) (string, error) {
		if command == "module reload app_rpt.so" {
			return "Unable to reload module", nil
		}
		return db.command(command)
	}
	duplex := 3
	c, rec := nodeTestContext(http.MethodPut, primaryNodeRequest{
		CurrentNode: "616265", Node: "616265", Callsign: "OLD1", Duplex: &duplex,
	})
	if err := p.save(c); err == nil || !strings.Contains(err.Error(), "rechazó module reload") {
		t.Fatalf("reload failure was reported as success: err=%v body=%s", err, rec.Body)
	}
	data, err := os.ReadFile(p.rptPath)
	if err != nil || !strings.Contains(string(data), "duplex = 3") {
		t.Fatalf("saved configuration is not explicit after reload failure: %v, %s", err, data)
	}
}

func TestPrimaryNodeAccessReadWriteAndDelete(t *testing.T) {
	p, db, _ := primaryNodeFixture(t)
	c, rec := nodeTestContext(http.MethodGet, nil)
	if err := p.getAccess(c); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), "Otro servidor") || !strings.Contains(rec.Body.String(), "Permitido para pruebas") {
		t.Fatalf("incorrect node scope: %s", rec.Body)
	}
	body := map[string]string{"primary_node": "616265", "type": "denylist", "node": "ea4abc/p", "reason": "Prueba"}
	c, _ = nodeTestContext(http.MethodPut, body)
	if err := p.changeAccess(c, false); err != nil {
		t.Fatal(err)
	}
	if db["denylist/616265/EA4ABC/P"] != "Prueba" {
		t.Fatalf("entry not persisted: %+v", db)
	}
	c, _ = nodeTestContext(http.MethodDelete, body)
	if err := p.changeAccess(c, true); err != nil {
		t.Fatal(err)
	}
	if _, exists := db["denylist/616265/EA4ABC/P"]; exists {
		t.Fatal("entry not deleted")
	}
}

func TestPrimaryPublicAccessReadWriteDeleteAndValidation(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	var commands []string
	p.ami = func(command string) (string, error) {
		commands = append(commands, command)
		switch command {
		case "database show whitelist":
			return "Output: /whitelist/HC1LNM : JJSC\nOutput: 1 results found.\n", nil
		case "database show blacklist":
			return "Output: 0 results found.\n", nil
		case `database put blacklist EA4ABC "Denied for testing"`:
			return "Output: Updated database successfully\n", nil
		case "database del whitelist HC1LNM":
			return "Output: Database entry removed.\n", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	}

	c, rec := nodeTestContext(http.MethodGet, nil)
	if err := p.getPublicAccess(c); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), `"callsign":"HC1LNM"`) ||
		!strings.Contains(rec.Body.String(), `"reason":"JJSC"`) {
		t.Fatalf("unexpected public lists: %s", rec.Body)
	}

	c, _ = nodeTestContext(http.MethodPut, publicCallsignEntry{
		Type: "blacklist", Callsign: "ea4abc", Reason: "Denied for testing",
	})
	if err := p.changePublicAccess(c, false); err != nil {
		t.Fatal(err)
	}
	c, _ = nodeTestContext(http.MethodDelete, publicCallsignEntry{
		Type: "whitelist", Callsign: "HC1LNM",
	})
	if err := p.changePublicAccess(c, true); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(commands, "\n"); !strings.Contains(got, `database put blacklist EA4ABC "Denied for testing"`) ||
		!strings.Contains(got, "database del whitelist HC1LNM") {
		t.Fatalf("unexpected AstDB commands: %s", got)
	}

	c, _ = nodeTestContext(http.MethodPut, publicCallsignEntry{
		Type: "whitelist", Callsign: "BAD\nCALL", Reason: "unsafe",
	})
	if err := p.changePublicAccess(c, false); err == nil {
		t.Fatal("invalid callsign should not reach AMI")
	}
}

func TestPrimaryNodeWarningExplainsMissingNodesMapping(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	if err := os.WriteFile(p.rptPath, []byte("[nodes]\n700001 = radio@192.0.2.1/700001,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := p.primary()
	if err == nil || !strings.Contains(err.Error(), "[nodes]") || !strings.Contains(err.Error(), "41777 = radio@127.0.0.1/41777,NONE") {
		t.Fatalf("missing actionable [nodes] warning: %v", err)
	}
}

func TestPrimaryNodeDoesNotTreatAMICommandFailureAsSuccess(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	p.ami = func(string) (string, error) { return "Permission denied", nil }
	if _, err := p.access("616265"); err == nil {
		t.Fatal("invalid read accepted")
	}
	if err := p.putAccess("616265", nodeAccessEntry{Type: "allowlist", Node: "448511"}); err == nil {
		t.Fatal("invalid write accepted")
	}
	if err := p.removeAccess("616265", nodeAccessEntry{Type: "allowlist", Node: "448511"}); err == nil {
		t.Fatal("invalid deletion accepted")
	}
}

func TestPrimaryNodeAddsRegistrationAndDirectCallsignOverride(t *testing.T) {
	req := primaryNodeRequest{Node: "616265", Callsign: "NEW2", Password: "new_password"}
	for _, tc := range []struct{ name, input, want string }{
		{"rpt.conf", "[616265](node-main)\n[nodes]\n616265 = radio@127.0.0.1/616265,NONE\n", "idrecording = |iNEW2"},
		{"iax.conf", "[general]\nbindport=4569\n", ";register => 616265:new_password@register.allstarlink.org"},
		{"rpt_http_registrations.conf", "[registrations]\n", "register => 616265:new_password@register.allstarlink.org"},
		{"savenode.conf", "NODE=0\nPASSWORD=\nENABLE=0\n", "NODE=616265\nPASSWORD=new_password\nENABLE=1\n"},
	} {
		got, err := rewritePrimaryFile(tc.name, tc.input, "616265", req)
		if err != nil || !strings.Contains(got, tc.want) {
			t.Fatalf("%s: %q %v", tc.name, got, err)
		}
	}
}

func TestPrimaryNodeDuplexOverrideIsAddedForInheritedValue(t *testing.T) {
	duplex := 3
	got, err := rewritePrimaryFile("rpt.conf", "[node-main](!)\nduplex = 1\n[616265](node-main)\nidrecording = |iTEST1\n", "616265", primaryNodeRequest{
		Node: "616265", Callsign: "TEST1", Duplex: &duplex,
	})
	if err != nil || !strings.Contains(got, "[616265](node-main)\nduplex = 3\n") ||
		!strings.Contains(got, "[node-main](!)\nduplex = 1\n") {
		t.Fatalf("inherited duplex was not overridden locally: %q %v", got, err)
	}
}

func TestPrimaryNodeRejectsRemoteNumberAlreadyInNodesTable(t *testing.T) {
	content := "[616265]\n[nodes]\n616265 = radio@127.0.0.1/616265,NONE\n616266 = radio@192.0.2.1/616266,NONE\n"
	if _, err := rewritePrimaryFile("rpt.conf", content, "616265", primaryNodeRequest{Node: "616266", Callsign: "TEST1"}); err == nil {
		t.Fatal("accepted an existing remote number")
	}
}

func TestPrimaryNodeClearAccessIsScopedAndIdempotent(t *testing.T) {
	for _, family := range []string{"allowlist", "denylist"} {
		t.Run(family, func(t *testing.T) {
			p, db, _ := primaryNodeFixture(t)
			db[family+"/6162650/448511"] = "Similar node"
			original := nodeTestDB{}
			for key, reason := range db {
				original[key] = reason
			}
			for range 2 {
				c, rec := nodeTestContext(http.MethodPost, map[string]string{
					"primary_node": "616265", "type": family, "confirmation": "VACIAR " + family,
				})
				if err := p.clearAccess(c); err != nil {
					t.Fatal(err)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("unexpected response %s", rec.Body)
				}
			}
			for key, reason := range original {
				if strings.HasPrefix(key, family+"/616265/") {
					if _, found := db[key]; found {
						t.Fatalf("entry not cleared: %s", key)
					}
				} else if db[key] != reason {
					t.Fatalf("unrelated entry changed: %s", key)
				}
			}
		})
	}
}

func TestPrimaryNodeClearAccessRejectsInvalidRequestsBeforeAMI(t *testing.T) {
	for _, body := range []map[string]string{
		{"primary_node": "616265", "type": "allowlist"},
		{"primary_node": "616265", "type": "other", "confirmation": "VACIAR other"},
		{"primary_node": "616266", "type": "denylist", "confirmation": "VACIAR denylist"},
	} {
		p, _, _ := primaryNodeFixture(t)
		p.ami = func(string) (string, error) { t.Fatal("invalid request reached AMI"); return "", nil }
		c, _ := nodeTestContext(http.MethodPost, body)
		if err := p.clearAccess(c); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}

func TestPrimaryNodeClearAccessReportsPartialFailure(t *testing.T) {
	p, db, _ := primaryNodeFixture(t)
	db["allowlist/616265/448512"] = "Second"
	p.ami = func(cmd string) (string, error) {
		if cmd == "database del allowlist/616265 448512" {
			return "", errors.New("AMI unavailable")
		}
		return db.command(cmd)
	}
	c, _ := nodeTestContext(http.MethodPost, map[string]string{
		"primary_node": "616265", "type": "allowlist", "confirmation": "VACIAR allowlist",
	})
	err := p.clearAccess(c)
	if err == nil || !strings.Contains(err.Error(), "algunas entradas") {
		t.Fatalf("partial failure not surfaced: %v", err)
	}
	if _, found := db["allowlist/616265/448511"]; found {
		t.Fatal("test did not exercise partial deletion")
	}
	if db["denylist/616265/TEST2"] != "Bloqueado" {
		t.Fatal("other list changed")
	}
}

func TestPrimaryNodePreservesCRLFAndTemplateHeadersWithWhitespace(t *testing.T) {
	content := "  [ 616265 ](node-main)\r\n[nodes]\r\n616265 = radio@127.0.0.1/616265,NONE\r\n"
	got, err := rewritePrimaryFile("rpt.conf", content, "616265", primaryNodeRequest{Node: "616266", Callsign: "TEST1"})
	if err != nil || !strings.Contains(got, "[616266](node-main)\r\nidrecording = |iTEST1\r\n") ||
		strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("line endings or section header not preserved: %q %v", got, err)
	}
}
