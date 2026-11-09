package controllers

// Este archivo contiene pruebas para la administración de nodos privados definidos en rpt.conf.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

func TestParsePrivateRPTTemplateSettingsIncludesParameterComments(t *testing.T) {
	content := "[other]\n; Unrelated comment\nunrelated=1\n" +
		"[node-main](!)\n; Maximum transmit duration\n; In milliseconds\ntotime = 180000 ; Default timeout\n" +
		"duplex = 2\n; Comment for a disabled setting\n;disabled = 1\nhangtime = 5000\n" +
		"[1998](node-main)\n; Node override\ntotime=1000\n"
	settings, descriptions := parsePrivateRPTTemplateSettings(strings.ReplaceAll(content, "\n", "\r\n"))
	if !reflect.DeepEqual(settings, map[string]string{"totime": "180000", "duplex": "2", "hangtime": "5000"}) {
		t.Fatalf("incorrect template parameters: %#v", settings)
	}
	if !reflect.DeepEqual(descriptions, map[string]string{
		"totime": "Maximum transmit duration\nIn milliseconds\nDefault timeout",
		"duplex": "", "hangtime": "",
	}) {
		t.Fatalf("incorrect parameter documentation: %#v", descriptions)
	}
	emptySettings, emptyDescriptions := parsePrivateRPTTemplateSettings("[1998]\ntotime=1000\n")
	if len(emptySettings) != 0 || len(emptyDescriptions) != 0 {
		t.Fatal("node parameters must not be returned as template parameters")
	}
}

func TestPrivateRPTServerFlagRoundTripsWithoutChangingAsteriskSettings(t *testing.T) {
	content := "[1998](node-main)\nrxchannel=Local/pseudo\ncustom=keep\n[nodes]\n1998=radio@127.0.0.1/1998,NONE\n"
	node := privateRPTNode{ID: "1998", Rxchannel: "Local/pseudo", ShowAsServer: true}
	updated, err := rewritePrivateRPTNodes(content, []privateRPTNode{node})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := parsePrivateRPTNodes(updated)
	if err != nil || len(nodes) != 1 || !nodes[0].ShowAsServer || nodes[0].Settings["custom"] != "keep" {
		t.Fatalf("visibility flag not preserved: %#v, %v", nodes, err)
	}
	if _, exists := nodes[0].Settings["amerimon_show_as_server"]; exists {
		t.Fatal("Amerimon flag must not be an Asterisk parameter")
	}
	node.ShowAsServer = false
	hidden, err := rewritePrivateRPTNodes(updated, []privateRPTNode{node})
	if err != nil || strings.Contains(hidden, "amerimon_show_as_server") || !strings.Contains(hidden, "custom=keep") {
		t.Fatalf("unable to disable visibility without removing custom settings: %s, %v", hidden, err)
	}
}

func TestRewritePrivateRPTNodesAddsAndUpdatesWithoutRemovingCustomSettings(t *testing.T) {
	input := "[general]\n" +
		"; keep this comment\n" +
		"[nodes]\n" +
		"700001 = radio@192.0.2.1/700001,NONE\n" +
		"[custom]\n" +
		"setting = keep\n"
	got, err := rewritePrivateRPTNodes(input, []privateRPTNode{{
		ID: "1998", Rxchannel: "USRP/127.0.0.1:34001:32001",
		Settings: map[string]string{"duplex": "0", "linktolink": "no"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[nodes]\n700001 = radio@192.0.2.1/700001,NONE\n1998 = radio@127.0.0.1/1998,NONE",
		"[1998](node-main)\nduplex = 0\nlinktolink = no\nrxchannel = USRP/127.0.0.1:34001:32001",
		"; keep this comment",
		"[custom]\nsetting = keep",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected preserved/created content %q, got:\n%s", want, got)
		}
	}

	got, err = rewritePrivateRPTNodes(got, []privateRPTNode{{
		ID: "1998", Rxchannel: "USRP/127.0.0.1:35001:33001",
		Settings: map[string]string{"duplex": "1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"duplex = 1",
		"linktolink = no",
		"rxchannel = USRP/127.0.0.1:35001:33001",
		"1998 = radio@127.0.0.1/1998,NONE",
		"[custom]\nsetting = keep",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected retained content %q after update, got:\n%s", want, got)
		}
	}
}

func TestRewritePrivateRPTNodesDeleteRemovesOnlySelectedNodeConfiguration(t *testing.T) {
	input := "[nodes]\n1998 = radio@127.0.0.1/1998,NONE\n1964 = radio@127.0.0.1/1964,NONE\n" +
		"[1998](node-main)\nduplex=0\ncustom=keep-on-delete\n" +
		"[1964](node-main)\nduplex=0\nrxchannel=Local/pseudo\n" +
		"[other]\nsetting=keep\n"
	got, err := rewritePrivateRPTNodes(input, []privateRPTNode{{
		ID: "1964", Rxchannel: "Local/pseudo", Settings: map[string]string{"duplex": "0"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "1998") || strings.Contains(got, "custom=keep-on-delete") {
		t.Fatalf("deleted node configuration remains:\n%s", got)
	}
	for _, want := range []string{"1964 = radio@127.0.0.1/1964,NONE", "[1964](node-main)", "[other]\nsetting=keep"} {
		if !strings.Contains(got, want) {
			t.Fatalf("other node/configuration missing %q:\n%s", want, got)
		}
	}
}

func TestRewritePrivateRPTNodesRejectsDuplicateIDsAndRxchannels(t *testing.T) {
	for _, nodes := range [][]privateRPTNode{
		{
			{ID: "1998", Rxchannel: "Local/pseudo"},
			{ID: "1998", Rxchannel: "Radio/1998"},
		},
		{
			{ID: "1998", Rxchannel: "Local/pseudo"},
			{ID: "1964", Rxchannel: "local/pseudo"},
		},
		{
			{ID: "1998", Rxchannel: "USRP/127.0.0.1:34001:32001"},
			{ID: "1964", Rxchannel: "USRP/192.0.2.1:34001:32001"},
		},
		{{ID: "999", Rxchannel: "Radio/999"}},
	} {
		if _, err := rewritePrivateRPTNodes("[nodes]\n", nodes); err == nil {
			t.Fatalf("expected invalid nodes to be rejected: %#v", nodes)
		}
	}
	if _, err := rewritePrivateRPTNodes("[700001](node-main)\nrxchannel=USRP/127.0.0.1:34001:32001\n", []privateRPTNode{
		{ID: "1998", Rxchannel: "USRP/127.0.0.1:34001:32001"},
	}); err == nil {
		t.Fatal("expected a private node to conflict with an existing public node rxchannel")
	}
}

func TestSavePrivateRPTNodesWritesBackupAndKeepsUnmanagedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rpt.conf")
	if err := os.WriteFile(filepath.Join(dir, "pjsip.conf"), []byte("[1001]\ntype=endpoint\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	original := "[node-main](!)\n; Maximum transmit duration\ntotime=180000\n" +
		"[nodes]\n700001 = radio@192.0.2.1/700001,NONE\n[other]\nsetting=keep\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	previous := config.App
	t.Cleanup(func() { config.App = previous })
	config.App.AMI.RPTConfigPath = path
	var commands []string
	control := privateRPTNodeControl{path: path, ami: func(command string) (string, error) {
		commands = append(commands, command)
		switch command {
		case "module reload app_rpt.so":
			return "Module 'app_rpt.so' reloaded successfully.", nil
		case "module show like app_rpt.so":
			return "Output: app_rpt.so Radio Repeater 1 Running extended\nOutput: 1 modules loaded", nil
		default:
			t.Fatalf("unexpected AMI command %q", command)
			return "", nil
		}
	}}
	ctx, rec := nodeTestContext("PUT", privateRPTNodesRequest{Nodes: []privateRPTNode{{
		ID: "1998", Rxchannel: "USRP/127.0.0.1:34001:32001",
		Settings: map[string]string{"duplex": "0"},
	}}})
	if err := control.save(ctx); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body)
	}
	if !reflect.DeepEqual(commands, []string{"module reload app_rpt.so", "module show like app_rpt.so"}) {
		t.Fatalf("expected reload followed by module status verification, got %#v", commands)
	}
	if !strings.Contains(rec.Body.String(), `"reloaded":true`) || !strings.Contains(rec.Body.String(), `"module_loaded":true`) {
		t.Fatalf("reload verification was not reported: %s", rec.Body)
	}
	backup, err := os.ReadFile(path + ".amerimon.bak")
	if err != nil || string(backup) != original {
		t.Fatalf("backup mismatch: %v, %q", err, backup)
	}
	saved, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(saved), "[other]\nsetting=keep") {
		t.Fatalf("unmanaged configuration was not preserved: %v, %s", err, saved)
	}

	ctx, rec = nodeTestContext("GET", nil)
	if err := control.get(ctx); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Nodes                []privateRPTNode  `json:"nodes"`
		TemplateSettings     map[string]string `json:"template_settings"`
		TemplateDescriptions map[string]string `json:"template_descriptions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || len(response.Nodes) != 1 {
		t.Fatalf("unexpected node response: %v, %s", err, rec.Body)
	}
	if response.TemplateSettings["totime"] != "180000" ||
		response.TemplateDescriptions["totime"] != "Maximum transmit duration" {
		t.Fatalf("template metadata missing from response: %s", rec.Body)
	}
}

func TestSavePrivateRPTNodesFailsWhenReloadIsNotConfirmedOrModuleIsNotLoaded(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reload      string
		status      string
		wantCommand string
	}{
		{name: "reload-rejected", reload: "No such module"},
		{name: "module-not-loaded", reload: "Module 'app_rpt.so' reloaded successfully.", status: "Output: 0 modules loaded", wantCommand: "module show like app_rpt.so"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "rpt.conf")
			if err := os.WriteFile(filepath.Join(filepath.Dir(path), "pjsip.conf"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("[nodes]\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			var commands []string
			control := privateRPTNodeControl{path: path, ami: func(command string) (string, error) {
				commands = append(commands, command)
				if command == "module reload app_rpt.so" {
					return tc.reload, nil
				}
				return tc.status, nil
			}}
			ctx, rec := nodeTestContext("PUT", privateRPTNodesRequest{Nodes: []privateRPTNode{{
				ID: "1998", Rxchannel: "USRP/127.0.0.1:34001:32001",
			}}})
			err := control.save(ctx)
			httpErr, ok := err.(*echo.HTTPError)
			if !ok || httpErr.Code != http.StatusBadGateway {
				t.Fatalf("expected reload failure status, got %v (response %d: %s)", err, rec.Code, rec.Body)
			}
			if tc.wantCommand != "" && (len(commands) != 2 || commands[1] != tc.wantCommand) {
				t.Fatalf("expected reload verification command, got %#v", commands)
			}
			if tc.wantCommand == "" && len(commands) != 1 {
				t.Fatalf("did not expect status check after rejected reload: %#v", commands)
			}
		})
	}
}

func TestSavePrivateRPTNodesRejectsPJSIPCollisionBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name, content string
	}{
		{"endpoint", "[1998]\ntype=endpoint\n"},
		{"auth-object", "[1998]\ntype=auth\n"},
		{"included-endpoint", "#include endpoints/*.conf\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "rpt.conf")
			pjsipPath := filepath.Join(dir, "custom-pjsip.conf")
			original := "[nodes]\n"
			if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "endpoints"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "endpoints", "phone.conf"), []byte("[1998]\ntype=endpoint\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(pjsipPath, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			control := privateRPTNodeControl{path: path, pjsipPath: pjsipPath, ami: func(string) (string, error) {
				t.Fatal("must not reload after a PJSIP collision")
				return "", nil
			}}
			ctx, _ := nodeTestContext(http.MethodPut, privateRPTNodesRequest{Nodes: []privateRPTNode{
				{ID: "1998", Rxchannel: "Local/pseudo"},
			}})
			err := control.save(ctx)
			httpErr, ok := err.(*echo.HTTPError)
			if !ok || httpErr.Code != http.StatusConflict {
				t.Fatalf("expected collision error, got %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != original {
				t.Fatalf("rpt.conf changed despite collision: %v, %q", err, data)
			}
			if _, err := os.Stat(path + ".amerimon.bak"); !os.IsNotExist(err) {
				t.Fatalf("backup should not be written: %v", err)
			}
		})
	}
}

func TestReadPJSIPSectionIDsRejectsIncompleteConfiguration(t *testing.T) {
	for _, content := range []string{"#include missing.conf\n", "#include pjsip.conf\n", "#exec external-command\n"} {
		path := filepath.Join(t.TempDir(), "pjsip.conf")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readPJSIPSectionIDs(path); err == nil {
			t.Fatalf("expected invalid/incomplete config to fail: %q", content)
		}
	}
	path := filepath.Join(t.TempDir(), "pjsip.conf")
	if err := os.WriteFile(path, []byte("#tryinclude missing.conf\n[1001]\ntype=endpoint\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ids, err := readPJSIPSectionIDs(path)
	if err != nil || !ids["1001"] {
		t.Fatalf("optional include or non-conflicting extension failed: %v, %#v", err, ids)
	}
}
