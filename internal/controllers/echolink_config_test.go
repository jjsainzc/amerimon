package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func echoLinkFixture(t *testing.T) (echoLinkConfigControl, map[string]string) {
	t.Helper()
	primary, _, _ := primaryNodeFixture(t)
	dir := filepath.Dir(primary.rptPath)
	if err := os.WriteFile(primary.rptPath, []byte("[616262]\n[nodes]\n616262=radio@127.0.0.1/616262,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"call": "TEST-L", "name": "Test node", "qth": "Test", "email": "",
		"node": "100", "lat": "0", "lon": "0", "freq": "0", "tone": "0",
		"power": "0", "height": "0", "gain": "0", "dir": "0", "maxstns": "20",
		"rtcptimeout": "10", "context": "radio-secure", "server1": "servers.echolink.org",
		"server2": "", "server3": "", "server4": "", "message": "", "recfile": "",
	}
	p := echoLinkConfigControl{filepath.Join(dir, "echolink.conf"), filepath.Join(dir, "modules.conf"), primary}
	var content strings.Builder
	content.WriteString("[el0]\npwd=test-placeholder\nastnode=99999\ncustom=keep\n")
	for key, value := range values {
		content.WriteString(key + "=" + value + "\n")
	}
	content.WriteString("[el1]\npwd=other-placeholder\ncustom=untouched\n")
	for path, data := range map[string]string{p.path: content.String(), p.modulesPath: "[modules]\nautoload=yes\nnoload=chan_echolink.so\nload=other.so\n"} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p, values
}

func TestEchoLinkUsesLocalCLIForModuleControl(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"[ \"$1\" = '-rx' ] || exit 1\n" +
		"case \"$2\" in\n" +
		"  'module load chan_echolink.so') echo 'Loaded chan_echolink.so';;\n" +
		"  'module unload -f chan_echolink.so') echo 'Unloaded chan_echolink.so';;\n" +
		"  'module show like chan_echolink.so') echo '0 modules loaded';;\n" +
		"  *) exit 1;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "asterisk"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	p := newEchoLinkConfigControl()
	for command, want := range map[string]string{
		"module load chan_echolink.so":      "Loaded chan_echolink.so",
		"module unload -f chan_echolink.so": "Unloaded chan_echolink.so",
		"module show like chan_echolink.so": "0 modules loaded",
	} {
		output, err := p.primary.ami(command)
		if err != nil || output != want {
			t.Fatalf("local CLI command %q: %v %q", command, err, output)
		}
	}
}

func TestEchoLinkPasswordRequiresCurrentRevisionAndValidSection(t *testing.T) {
	p, _ := echoLinkFixture(t)
	data, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		section, revision string
		status            int
	}{
		{"el0", echoLinkRevision(data), 200},
		{"el0", "stale", 409},
		{"unknown", echoLinkRevision(data), 400},
	} {
		c, rec := echoLinkContext(http.MethodGet, nil)
		query := c.Request().URL.Query()
		query.Set("section", test.section)
		query.Set("revision", test.revision)
		c.Request().URL.RawQuery = query.Encode()
		if err := p.password(c); err != nil || rec.Code != test.status {
			t.Fatalf("password read: %v %d", err, rec.Code)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("password response may be cached")
		}
		if test.status == 200 {
			var result map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["password"] != "test-placeholder" {
				t.Fatal("wrong section password returned")
			}
		} else if strings.Contains(rec.Body.String(), "test-placeholder") {
			t.Fatal("password leaked on failed request")
		}
	}
}

func echoLinkContext(method string, payload any) (echo.Context, *httptest.ResponseRecorder) {
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(method, "/", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return echo.New().NewContext(req, rec), rec
}

func TestEchoLinkConfigRedactsPasswordAndPreservesOtherSections(t *testing.T) {
	p, values := echoLinkFixture(t)
	p.primary.ami = func(string) (string, error) { return "chan_echolink.so EchoLink Running", nil }
	c, rec := echoLinkContext(http.MethodGet, nil)
	if err := p.get(c); err != nil || rec.Code != 200 {
		t.Fatalf("get: %v %d", err, rec.Code)
	}
	if strings.Contains(rec.Body.String(), "test-placeholder") || strings.Contains(rec.Body.String(), "other-placeholder") {
		t.Fatal("password returned to browser")
	}
	data, _ := os.ReadFile(p.path)
	req := echoLinkRequest{Section: "el0", AstNode: "616262", Revision: echoLinkRevision(data), Values: values}
	c, rec = echoLinkContext(http.MethodPut, req)
	if err := p.save(c); err != nil || rec.Code != 200 {
		t.Fatalf("save: %v %d %s", err, rec.Code, rec.Body)
	}
	saved, _ := os.ReadFile(p.path)
	if !strings.Contains(string(saved), "pwd=test-placeholder") || !strings.Contains(string(saved), "astnode=616262") ||
		!strings.Contains(string(saved), "custom=keep") || !strings.Contains(string(saved), "[el1]\npwd=other-placeholder\ncustom=untouched") {
		t.Fatal("password, astnode, custom options or other section changed incorrectly")
	}
	info, err := os.Stat(p.path + ".amerimon.bak")
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("backup not private: %v", err)
	}
	c, rec = echoLinkContext(http.MethodPut, req)
	if err := p.save(c); err != nil || rec.Code != 409 {
		t.Fatalf("stale edit accepted: %v %d", err, rec.Code)
	}
}

func TestEchoLinkPermanentDisableWithArrowAssignments(t *testing.T) {
	p, _ := echoLinkFixture(t)
	original := "[modules]\nautoload => yes\nload=> app_system.so\nload=> app_transfer.so\nload=> chan_echolink.so\nnoload => other.so\n"
	if err := os.WriteFile(p.modulesPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded := true
	p.primary.ami = func(command string) (string, error) {
		switch command {
		case "module show like chan_echolink.so":
			if loaded {
				return "chan_echolink.so EchoLink Running", nil
			}
			return "0 modules loaded", nil
		case "module unload -f chan_echolink.so":
			loaded = false
			return "Output: Unloaded chan_echolink.so\n", nil
		default:
			t.Fatalf("unexpected command: %s", command)
			return "", errors.New("unexpected command")
		}
	}
	c, rec := echoLinkContext(http.MethodPost, echo.Map{
		"action": "disable", "persist": true, "confirmation": "APPLY ECHOLINK",
	})
	if err := p.activate(c); err != nil || rec.Code != 200 {
		t.Fatalf("disable: %v %d", err, rec.Code)
	}
	data, err := os.ReadFile(p.modulesPath)
	if err != nil {
		t.Fatal(err)
	}
	sections := parseManagerSections(string(data))
	loads := sections[0].values("load")
	noloads := sections[0].values("noload")
	if strings.Join(loads, ",") != "app_system.so,app_transfer.so" ||
		strings.Join(noloads, ",") != "chan_echolink.so,other.so" {
		t.Fatalf("incorrect permanent module state: loads=%v noloads=%v", loads, noloads)
	}
	if string(data) != strings.Replace(original, "load=> chan_echolink.so", "noload=> chan_echolink.so", 1) {
		t.Fatal("disable changed more than the existing EchoLink line")
	}
	enabled, err := echoLinkModuleConfig(string(data), true)
	if err != nil {
		t.Fatal(err)
	}
	sections = parseManagerSections(enabled)
	if strings.Join(sections[0].values("load"), ",") != "app_system.so,app_transfer.so,chan_echolink.so" ||
		strings.Join(sections[0].values("noload"), ",") != "other.so" {
		t.Fatal("reenable left conflicting module directives")
	}
	if enabled != original {
		t.Fatal("reenable did not restore the same line")
	}
}

func TestEchoLinkEnableDisableEnableWithActualCLIResponses(t *testing.T) {
	for _, persist := range []bool{false, true} {
		t.Run(strconv.FormatBool(persist), func(t *testing.T) {
			p, _ := echoLinkFixture(t)
			original, err := os.ReadFile(p.modulesPath)
			if err != nil {
				t.Fatal(err)
			}
			loaded := false
			var commands []string
			p.primary.ami = func(command string) (string, error) {
				switch command {
				case "module show like chan_echolink.so":
					if loaded {
						return "Output: chan_echolink.so Echolink Channel Driver 1 Running extended\nOutput: 1 modules loaded", nil
					}
					return "Output: 0 modules loaded", nil
				case "module load chan_echolink.so":
					commands = append(commands, command)
					loaded = true
					return "Output: Loaded chan_echolink.so", nil
				case "module unload -f chan_echolink.so":
					commands = append(commands, command)
					loaded = false
					return "Output: Unloaded chan_echolink.so", nil
				default:
					t.Fatalf("unexpected command: %s", command)
					return "", errors.New("unexpected command")
				}
			}
			for _, action := range []string{"enable", "disable", "enable"} {
				c, rec := echoLinkContext(http.MethodPost, echo.Map{
					"action": action, "persist": persist, "confirmation": "APPLY ECHOLINK",
				})
				if err := p.activate(c); err != nil || rec.Code != 200 {
					t.Fatalf("%s: %v %d", action, err, rec.Code)
				}
				var result struct {
					Loaded bool `json:"loaded"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Loaded != (action == "enable") {
					t.Fatal("incorrect runtime state")
				}
				data, err := os.ReadFile(p.modulesPath)
				if err != nil {
					t.Fatal(err)
				}
				if !persist && !bytes.Equal(data, original) {
					t.Fatal("temporary operation changed modules.conf")
				}
				if persist {
					section := parseManagerSections(string(data))[0]
					key, opposite := "load", "noload"
					if action == "disable" {
						key, opposite = opposite, key
					}
					if !strings.Contains(strings.Join(section.values(key), ","), "chan_echolink.so") ||
						strings.Contains(strings.Join(section.values(opposite), ","), "chan_echolink.so") {
						t.Fatal("persistent state conflicts with requested action")
					}
				}
			}
			if strings.Join(commands, ",") != "module load chan_echolink.so,module unload -f chan_echolink.so,module load chan_echolink.so" {
				t.Fatalf("incorrect lifecycle commands: %v", commands)
			}
		})
	}
}

func TestEchoLinkForcedUnloadRequiresUnloadedStatus(t *testing.T) {
	p, _ := echoLinkFixture(t)
	p.primary.ami = func(command string) (string, error) {
		switch command {
		case "module show like chan_echolink.so":
			return "chan_echolink.so Echolink Channel Driver 1 Running extended", nil
		case "module unload -f chan_echolink.so":
			return "Unloaded chan_echolink.so", nil
		default:
			t.Fatalf("unexpected command: %s", command)
			return "", errors.New("unexpected command")
		}
	}
	c, rec := echoLinkContext(http.MethodPost, echo.Map{
		"action": "disable", "persist": false, "confirmation": "APPLY ECHOLINK",
	})
	if err := p.activate(c); err != nil || rec.Code != 502 {
		t.Fatalf("loaded module reported as disabled: %v %d", err, rec.Code)
	}
}

func TestEchoLinkModuleRewritePreservesLineAndRemovesDuplicates(t *testing.T) {
	content := "[modules]\r\nload => other.so ; keep\r\n  load => chan_echolink.so ; EchoLink\r\nnoload=chan_echolink.so\r\n[other]\r\ncustom=yes\r\n"
	want := "[modules]\r\nload => other.so ; keep\r\n  noload => chan_echolink.so ; EchoLink\r\n[other]\r\ncustom=yes\r\n"
	updated, err := echoLinkModuleConfig(content, false)
	if err != nil || updated != want {
		t.Fatalf("same-line disable: %v\n%q", err, updated)
	}
	again, err := echoLinkModuleConfig(updated, false)
	if err != nil || again != want {
		t.Fatal("repeated disable changed the file")
	}
}

func TestEchoLinkModuleChangesTemporaryAndPermanent(t *testing.T) {
	p, _ := echoLinkFixture(t)
	loaded := false
	failUnload := false
	var commands []string
	p.primary.ami = func(cmd string) (string, error) {
		commands = append(commands, cmd)
		switch cmd {
		case "module show like chan_echolink.so":
			if loaded {
				return "chan_echolink.so EchoLink Running", nil
			}
			return "0 modules loaded", nil
		case "module load chan_echolink.so":
			loaded = true
			return "Module loaded successfully", nil
		case "module unload -f chan_echolink.so":
			if failUnload {
				return "", errors.New("in use")
			}
			loaded = false
			return "Unloaded chan_echolink.so", nil
		case "module reload chan_echolink.so":
			return "Module reloaded successfully", nil
		}
		return "", errors.New("unexpected command")
	}
	original, _ := os.ReadFile(p.modulesPath)
	for _, test := range []struct {
		action  string
		persist bool
		status  int
	}{
		{"enable", false, 200}, {"disable", false, 200}, {"enable", true, 200}, {"disable", true, 502},
	} {
		failUnload = test.status == 502
		c, rec := echoLinkContext(http.MethodPost, echo.Map{"action": test.action, "persist": test.persist, "confirmation": "APPLY ECHOLINK"})
		if err := p.activate(c); err != nil || rec.Code != test.status {
			t.Fatalf("%+v: %v %d %s", test, err, rec.Code, rec.Body)
		}
		data, _ := os.ReadFile(p.modulesPath)
		if !test.persist && !bytes.Equal(original, data) {
			t.Fatal("temporary action changed modules.conf")
		}
		if test.persist && !strings.Contains(string(data), "load=other.so") {
			t.Fatal("unrelated module lost")
		}
	}
	unloads := 0
	for _, cmd := range commands {
		if cmd == "module unload -f chan_echolink.so" {
			unloads++
		}
	}
	if unloads != 2 {
		t.Fatal("temporary and permanent disable must both force unload")
	}
	data, _ := os.ReadFile(p.modulesPath)
	if !strings.Contains(string(data), "noload=chan_echolink.so") {
		t.Fatal("permanent disable not saved")
	}
}

func TestEchoLinkValidationAndModuleRewrite(t *testing.T) {
	_, values := echoLinkFixture(t)
	if err := validateEchoLinkValues(values); err != nil {
		t.Fatal(err)
	}

	t.Run("absent optional include shows fields and survives save", func(t *testing.T) {
		p, values := echoLinkFixture(t)
		data, err := os.ReadFile(p.path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data) + "#tryinclude \"custom/echolink.conf\"\n"
		if err := os.WriteFile(p.path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		p.primary.ami = func(string) (string, error) {
			return "chan_echolink.so EchoLink Running", nil
		}
		c, rec := echoLinkContext(http.MethodGet, nil)
		if err := p.get(c); err != nil || rec.Code != 200 {
			t.Fatalf("optional include hid fields: %v %d", err, rec.Code)
		}
		var config struct {
			Sections []echoLinkSection `json:"sections"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &config); err != nil {
			t.Fatal(err)
		}
		if len(config.Sections) != 2 || config.Sections[0].Values["call"] != values["call"] {
			t.Fatal("configuration fields not returned")
		}
		c, rec = echoLinkContext(http.MethodPut, echoLinkRequest{
			Section: "el0", AstNode: "616262", Revision: echoLinkRevision([]byte(content)), Values: values,
		})
		if err := p.save(c); err != nil || rec.Code != 200 {
			t.Fatalf("save: %v %d", err, rec.Code)
		}
		saved, _ := os.ReadFile(p.path)
		if !strings.Contains(string(saved), "#tryinclude \"custom/echolink.conf\"") {
			t.Fatal("optional include removed from file")
		}
		target := filepath.Join(filepath.Dir(p.path), "custom", "echolink.conf")
		if err := os.Mkdir(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("[el0]\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := p.sections(content); err == nil {
			t.Fatal("existing include ignored")
		}
		for _, directive := range []string{"#include missing.conf", "#tryinclude custom/*.conf", "#exec command"} {
			if _, err := p.sections(string(data) + directive + "\n"); err == nil {
				t.Fatalf("unsupported directive accepted: %s", directive)
			}
		}
	})
	for key, value := range map[string]string{"lat": "91", "node": "1.5", "dir": "9", "name": "x\n[bad]", "server1": "bad/host", "pwd": "injected"} {
		copy := make(map[string]string)
		for k, v := range values {
			copy[k] = v
		}
		copy[key] = value
		if err := validateEchoLinkValues(copy); err == nil {
			t.Fatalf("accepted invalid %s", key)
		}
	}
	for _, content := range []string{"#include modules-extra.conf\n[modules]\n", "[modules]\n[modules]\n"} {
		if _, err := echoLinkModuleConfig(content, true); err == nil {
			t.Fatal("ambiguous module file accepted")
		}
	}
	content := "[modules]\nautoload=yes\nload=other.so\nnoload=chan_echolink.so\n"
	updated, err := echoLinkModuleConfig(content, true)
	if err != nil || strings.Contains(updated, "noload=chan_echolink.so") {
		t.Fatal("enable did not remove noload")
	}
	again, err := echoLinkModuleConfig(updated, true)
	if err != nil || again != updated {
		t.Fatal("rewrite not idempotent")
	}
}
