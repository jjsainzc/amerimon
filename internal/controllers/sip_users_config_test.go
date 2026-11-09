package controllers

// Este archivo contiene pruebas para la lectura y administración de usuarios IAX y extensiones PJSIP.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

func sipTestControl(t *testing.T, iax, pjsip string) sipUsersControl {
	t.Helper()
	dir := t.TempDir()
	p := sipUsersControl{
		iaxPath: filepath.Join(dir, "iax.conf"), pjsipPath: filepath.Join(dir, "pjsip.conf"),
		reload: func(command string) (string, error) {
			if command != "iax2 reload" && command != "module reload res_pjsip.so" {
				t.Fatalf("unexpected reload %q", command)
			}
			return "Module reloaded", nil
		},
	}
	for path, content := range map[string]string{p.iaxPath: iax, p.pjsipPath: pjsip} {
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func sipTestRequest(t *testing.T, handler echo.HandlerFunc, method, body, param string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if param != "" {
		c.SetParamNames("name", "id")
		c.SetParamValues(param, param)
	}
	if err := handler(c); err != nil {
		e.HTTPErrorHandler(err, c)
	}
	return rec
}

func TestSIPUsersConfiguredPaths(t *testing.T) {
	original := config.CurrentAMI()
	defer config.SetAMI(original)
	config.SetAMI(config.AMIConfig{RPTConfigPath: "/custom/rpt.conf", PJSIPConfigPath: "/other/pjsip.conf"})
	p := newSIPUsersControl()
	if p.iaxPath != "/custom/iax.conf" || p.pjsipPath != "/other/pjsip.conf" {
		t.Fatalf("wrong paths: %#v", p)
	}
}

func TestSIPUsersIAXSavePreservesOtherSettings(t *testing.T) {
	original := "[general]\nbindport=4569\n#include iax-extra.conf\n[HC1LNM] ; station\nsecret=old\ncontext=old\nqualify=yes\n[other]\nsecret=untouched\n[radio]\nsecret=reserved\n"
	p := sipTestControl(t, original, "")
	rec := sipTestRequest(t, p.getIAX, http.MethodGet, "", "")
	var users map[string]IAXUser
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || len(users) != 2 || users["HC1LNM"].Secret != "old" {
		t.Fatalf("unexpected users: %s", rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("credentials may be cached")
	}
	rec = sipTestRequest(t, p.saveIAX, http.MethodPost, `{"name":"HC1LNM","context":"iax-client","secret":"new-secret","allow":["ulaw"],"disallow":["all"]}`, "")
	if rec.Code != 200 {
		t.Fatalf("save: %s", rec.Body.String())
	}
	data, _ := os.ReadFile(p.iaxPath)
	for _, expected := range []string{"qualify=yes", "bindport=4569", "#include iax-extra.conf", "secret=untouched", "secret=new-secret", "[radio]\nsecret=reserved"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("lost setting %s: %s", expected, data)
		}
	}
	backup, _ := os.ReadFile(p.iaxPath + ".amerimon.bak")
	if string(backup) != original {
		t.Fatal("backup missing or changed")
	}
	info, err := os.Stat(p.iaxPath)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("permissions changed: %v", err)
	}
}

func TestSIPUsersIAXTemplateUsesIncludedFileAndHidesReservedSections(t *testing.T) {
	mainContent := "[general]\nbindport=4569\n[iaxclient](!)\ntype=friend\ncontext=iax-client\n; You can use whatever you want for the section name (username), but a callsign\n; is probably a good idea. NOTE: If the user has multiple clients (ie an entry in\n; this file, as well as iaxrpt-users), the section names MUST be unique. That could\n; mean naming the sections something like [<callsign>-m] and [<callsign>-p].\n#tryinclude custom/iax/iaxrpt-users.conf\n[radio]\nsecret=radio-secret\n[allstar-sys]\nsecret=sys-secret\n[allstar-public]\nsecret=public-secret\n"
	p := sipTestControl(t, mainContent, "")
	includePath := filepath.Join(filepath.Dir(p.iaxPath), "custom", "iax", "iaxrpt-users.conf")
	if err := os.MkdirAll(filepath.Dir(includePath), 0o750); err != nil {
		t.Fatal(err)
	}
	includeContent := "[HC1LNM](iaxclient)\n    user = HC1LNM\n    secret = old-secret\n    callerid = \"HC1LNM\"\n"
	if err := os.WriteFile(includePath, []byte(includeContent), 0o640); err != nil {
		t.Fatal(err)
	}

	rec := sipTestRequest(t, p.getIAX, http.MethodGet, "", "")
	var users map[string]IAXUser
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(users) != 1 || users["HC1LNM"].Secret != "old-secret" {
		t.Fatalf("unexpected users: %d %s", rec.Code, rec.Body.String())
	}

	rec = sipTestRequest(t, p.saveIAX, http.MethodPost, `{"name":"JJSC","context":"iax-client","secret":"new-secret"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("create templated user: %d %s", rec.Code, rec.Body.String())
	}
	updated, err := os.ReadFile(includePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"[HC1LNM](iaxclient)", "secret = old-secret",
		"[JJSC](iaxclient)", "user = JJSC", "secret = new-secret ;",
		`callerid = "JJSC"`,
	} {
		if !strings.Contains(string(updated), expected) {
			t.Fatalf("missing %q from included users: %s", expected, updated)
		}
	}
	mainAfter, err := os.ReadFile(p.iaxPath)
	if err != nil || string(mainAfter) != mainContent {
		t.Fatalf("iax.conf changed unexpectedly: %v %s", err, mainAfter)
	}

	rec = sipTestRequest(t, p.saveIAX, http.MethodPost, `{"name":"HC1LNM","context":"ignored","secret":"updated-secret"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("update templated user: %d %s", rec.Code, rec.Body.String())
	}
	updated, err = os.ReadFile(includePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "secret = updated-secret") || !strings.Contains(string(updated), `callerid = "HC1LNM"`) {
		t.Fatalf("template user update lost required fields: %s", updated)
	}

	rec = sipTestRequest(t, p.deleteIAX, http.MethodDelete, "", "HC1LNM")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete templated user: %d %s", rec.Code, rec.Body.String())
	}
	updated, err = os.ReadFile(includePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updated), "[HC1LNM]") || !strings.Contains(string(updated), "[JJSC](iaxclient)") {
		t.Fatalf("delete modified unrelated users: %s", updated)
	}
}

func TestSIPUsersIAXTemplateCreatesMissingIncludedFile(t *testing.T) {
	content := "[iaxclient](!)\ncontext=iax-client\n#tryinclude custom/clients/test-users.conf\n"
	p := sipTestControl(t, content, "")
	rec := sipTestRequest(t, p.saveIAX, http.MethodPost, `{"name":"HC1LNM","context":"iax-client","secret":"secret"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("create included file: %d %s", rec.Code, rec.Body.String())
	}
	includePath := filepath.Join(filepath.Dir(p.iaxPath), "custom", "clients", "test-users.conf")
	created, err := os.ReadFile(includePath)
	if err != nil || !strings.Contains(string(created), "[HC1LNM](iaxclient)") {
		t.Fatalf("included user file missing: %v %s", err, created)
	}
	info, err := os.Stat(includePath)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("unexpected include file permissions: %v %v", err, info)
	}
}

func TestSIPUsersIAXTemplateRequiresTryInclude(t *testing.T) {
	p := sipTestControl(t, "[iaxclient](!)\ncontext=iax-client\n; You can use whatever you want for the section name (username), but a callsign\n", "")
	rec := sipTestRequest(t, p.getIAX, http.MethodGet, "", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("missing include was not reported: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSIPUsersIAXWithoutTemplateStayInIAXConfig(t *testing.T) {
	content := "[iaxclient]\ncontext=iax-client\n; You can use whatever you want for the section name (username), but a callsign\n#tryinclude custom/iax/iaxrpt-users.conf\n"
	p := sipTestControl(t, content, "")
	rec := sipTestRequest(t, p.saveIAX, http.MethodPost, `{"name":"HC1LNM","context":"iax-client","secret":"secret"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("save without template: %d %s", rec.Code, rec.Body.String())
	}
	main, err := os.ReadFile(p.iaxPath)
	if err != nil || !strings.Contains(string(main), "[HC1LNM]") {
		t.Fatalf("user was not created in iax.conf: %v %s", err, main)
	}
	includePath := filepath.Join(filepath.Dir(p.iaxPath), "custom", "iax", "iaxrpt-users.conf")
	if _, err := os.Stat(includePath); !os.IsNotExist(err) {
		t.Fatalf("unexpected include file created: %v", err)
	}
}

func TestIAXClientUsersPathUsesOnlyTemplateStanza(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
		templated, invalid  bool
	}{
		{"relative", "[iaxclient](!)\n#tryinclude custom/my-users.conf\n[next]\n#tryinclude wrong.conf\n", "/etc/asterisk/custom/my-users.conf", true, false},
		{"absolute", "[iaxclient](!)\n#tryinclude /opt/asterisk/users.conf\n", "/opt/asterisk/users.conf", true, false},
		{"quoted", "[iaxclient](!) ; template\n #tryinclude \"custom/my users.conf\" ; users\n", "/etc/asterisk/custom/my users.conf", true, false},
		{"outside-stanza", "[iaxclient](!)\ncontext=clients\n[next]\n#tryinclude wrong.conf\n", "", true, true},
		{"before-stanza", "[general]\n#tryinclude wrong.conf\n[iaxclient](!)\n", "", true, true},
		{"commented", "[iaxclient](!)\n;#tryinclude wrong.conf\n", "", true, true},
		{"no-template", "[iaxclient]\n#tryinclude ignored.conf\n", "", false, false},
		{"multiple", "[iaxclient](!)\n#tryinclude one.conf\n#tryinclude two.conf\n", "", true, true},
		{"wildcard", "[iaxclient](!)\n#tryinclude custom/*.conf\n", "", true, true},
		{"duplicate-template", "[iaxclient](!)\n#tryinclude one.conf\n[iaxclient](!)\n#tryinclude two.conf\n", "", true, true},
		{"self", "[iaxclient](!)\n#tryinclude iax.conf\n", "", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path, templated, err := iaxClientUsersPath(tc.content, "/etc/asterisk/iax.conf")
			if path != tc.want || templated != tc.templated || (err != nil) != tc.invalid {
				t.Fatalf("got path=%q templated=%t err=%v", path, templated, err)
			}
		})
	}
}

func TestSIPUsersIAXTemplateRejectsIncludeInAnotherStanzaWithoutWriting(t *testing.T) {
	content := "[iaxclient](!)\ncontext=iax-client\n[other]\n#tryinclude custom/users.conf\n"
	p := sipTestControl(t, content, "")
	p.reload = func(string) (string, error) {
		t.Fatal("unexpected reload")
		return "", nil
	}
	for _, request := range []struct {
		handler             echo.HandlerFunc
		method, body, param string
	}{
		{p.getIAX, http.MethodGet, "", ""},
		{p.saveIAX, http.MethodPost, `{"name":"HC1LNM","context":"iax-client","secret":"secret"}`, ""},
		{p.deleteIAX, http.MethodDelete, "", "HC1LNM"},
	} {
		rec := sipTestRequest(t, request.handler, request.method, request.body, request.param)
		if rec.Code != http.StatusConflict {
			t.Fatalf("include outside stanza accepted: %d %s", rec.Code, rec.Body.String())
		}
	}
	after, err := os.ReadFile(p.iaxPath)
	if err != nil || string(after) != content {
		t.Fatalf("main config modified: %v %s", err, after)
	}
}

func TestSIPUsersIAXHidesAndProtectsReservedSections(t *testing.T) {
	content := "[radio]\nsecret=radio\n[allstar-sys]\nsecret=sys\n[allstar-public]\nsecret=public\n[HC1LNM]\nsecret=radio-user\ncontext=iax-client\n"
	p := sipTestControl(t, content, "")
	rec := sipTestRequest(t, p.getIAX, http.MethodGet, "", "")
	var users map[string]IAXUser
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users["HC1LNM"].Name != "HC1LNM" {
		t.Fatalf("reserved sections were exposed: %s", rec.Body.String())
	}
	rec = sipTestRequest(t, p.saveIAX, http.MethodPost, `{"name":"radio","context":"iax-client","secret":"new"}`, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reserved section save was allowed: %d %s", rec.Code, rec.Body.String())
	}
	rec = sipTestRequest(t, p.deleteIAX, http.MethodDelete, "", "allstar-public")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reserved section delete was allowed: %d %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(p.iaxPath)
	if err != nil || string(data) != content {
		t.Fatalf("reserved config changed: %v %s", err, data)
	}
}

func TestSIPUsersIAXDeleteAllowsUsernamesAndProtectsIAXRPT(t *testing.T) {
	content := "[test-user]\nsecret=user-secret\n[iaxrpt]\nsecret=system-secret\n"
	p := sipTestControl(t, content, "")

	rec := sipTestRequest(t, p.deleteIAX, http.MethodDelete, "", "iaxrpt")
	data, err := os.ReadFile(p.iaxPath)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusForbidden || string(data) != content {
		t.Fatalf("system section was not protected: %d %s %s", rec.Code, rec.Body.String(), data)
	}

	rec = sipTestRequest(t, p.deleteIAX, http.MethodDelete, "", "test-user")
	if rec.Code != http.StatusOK {
		t.Fatalf("callsign delete failed: %d %s", rec.Code, rec.Body.String())
	}
	data, err = os.ReadFile(p.iaxPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "[test-user]") || !strings.Contains(string(data), "[iaxrpt]") {
		t.Fatalf("unexpected IAX sections after callsign delete: %s", data)
	}
}

const sipSeparateObjects = `[global]
type=global
[udp]
type=transport
protocol=udp
[1001] ; endpoint
type=endpoint
context=sip-phones
auth=1001-auth
aors=1001-aor
transport=udp
qualify_frequency=60
[1001-auth]
type=auth
auth_type=userpass
username=custom-login
password=old-secret
[1001-aor]
type=aor
max_contacts=2
remove_existing=yes
`

func TestSIPUsersPJSIPSeparateReferencesCRUD(t *testing.T) {
	p := sipTestControl(t, "", sipSeparateObjects)
	rec := sipTestRequest(t, p.getPJSIP, http.MethodGet, "", "")
	var extensions []PJSIPExtension
	if err := json.Unmarshal(rec.Body.Bytes(), &extensions); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || len(extensions) != 1 || extensions[0].ID != "1001" || extensions[0].Password != "old-secret" || extensions[0].MaxContacts != "2" {
		t.Fatalf("incorrect grouping: %s", rec.Body.String())
	}
	rec = sipTestRequest(t, p.savePJSIP, http.MethodPost, `{"id":"1001","context":"phones","password":"new-secret","max_contacts":"5","allow":["ulaw"]}`, "")
	if rec.Code != 200 {
		t.Fatalf("save: %s", rec.Body.String())
	}
	data, _ := os.ReadFile(p.pjsipPath)
	for _, expected := range []string{"password=new-secret", "max_contacts=5", "transport=udp", "qualify_frequency=60", "username=custom-login", "remove_existing=yes", "auth=1001-auth", "aors=1001-aor"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("lost setting %s: %s", expected, data)
		}
	}
	rec = sipTestRequest(t, p.deletePJSIP, http.MethodDelete, "", "1001")
	if rec.Code != 200 {
		t.Fatalf("delete: %s", rec.Body.String())
	}
	data, _ = os.ReadFile(p.pjsipPath)
	if strings.Contains(string(data), "1001") || !strings.Contains(string(data), "[udp]") {
		t.Fatalf("bad delete: %s", data)
	}
}

func TestSIPUsersPJSIPCreateRoundTrip(t *testing.T) {
	p := sipTestControl(t, "", "[udp]\ntype=transport\nprotocol=udp\n")
	reloads := 0
	p.reload = func(command string) (string, error) {
		if command != "module reload res_pjsip.so" {
			t.Fatalf("unexpected PJSIP reload %q", command)
		}
		reloads++
		return "Module res_pjsip.so reloaded successfully.", nil
	}
	body := `{"id":"2001","context":"phones","password":"new-secret","max_contacts":"4"}`
	rec := sipTestRequest(t, p.savePJSIP, http.MethodPost, body, "")
	if rec.Code != 200 {
		t.Fatalf("create: %s", rec.Body.String())
	}
	if reloads != 1 {
		t.Fatalf("expected one reload after creation, got %d", reloads)
	}
	rec = sipTestRequest(t, p.getPJSIP, http.MethodGet, "", "")
	var list []PJSIPExtension
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Password != "new-secret" || len(list[0].Allow) != 4 {
		t.Fatalf("bad roundtrip: %s", rec.Body.String())
	}
	rec = sipTestRequest(t, p.savePJSIP, http.MethodPost, body, "")
	if rec.Code != 200 {
		t.Fatalf("update same-named objects: %s", rec.Body.String())
	}
	if reloads != 2 {
		t.Fatalf("expected two reloads after update, got %d", reloads)
	}
	rec = sipTestRequest(t, p.deletePJSIP, http.MethodDelete, "", "2001")
	if rec.Code != 200 {
		t.Fatalf("delete same-named objects: %s", rec.Body.String())
	}
	if reloads != 3 {
		t.Fatalf("expected three reloads after deletion, got %d", reloads)
	}
}

func TestSIPUsersRejectInvalidDataWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		iax        bool
	}{
		{"reserved", `{"name":"general","context":"test","secret":"secret"}`, true},
		{"newline", `{"name":"HC1LNM","context":"test","secret":"bad\n[general]"}`, true},
		{"header", `{"id":"bad]","context":"phones","password":"secret"}`, false},
		{"contacts", `{"id":"1001","context":"phones","password":"secret","max_contacts":"0"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := sipTestControl(t, "[general]\n", sipSeparateObjects)
			handler, path := p.savePJSIP, p.pjsipPath
			if tc.iax {
				handler, path = p.saveIAX, p.iaxPath
			}
			before, _ := os.ReadFile(path)
			rec := sipTestRequest(t, handler, http.MethodPost, tc.body, "")
			after, _ := os.ReadFile(path)
			if rec.Code != 400 || string(before) != string(after) {
				t.Fatalf("invalid request changed config: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSIPUsersRejectSharedPJSIPObjects(t *testing.T) {
	content := sipSeparateObjects + "[1002]\ntype=endpoint\nauth=1001-auth\naors=1001-aor\n"
	p := sipTestControl(t, "", content)
	rec := sipTestRequest(t, p.deletePJSIP, http.MethodDelete, "", "1001")
	if rec.Code != 409 {
		t.Fatalf("expected conflict: %d %s", rec.Code, rec.Body.String())
	}
	data, _ := os.ReadFile(p.pjsipPath)
	if string(data) != content {
		t.Fatal("modified shared configuration")
	}
}

func TestSIPUsersReloadErrorsAreNotSuccess(t *testing.T) {
	for _, fail := range []func(string) (string, error){
		func(string) (string, error) { return "", errors.New("AMI unavailable") },
		func(string) (string, error) { return "No such module 'res_pjsip.so'", nil },
	} {
		p := sipTestControl(t, "", sipSeparateObjects)
		p.reload = fail
		rec := sipTestRequest(t, p.savePJSIP, http.MethodPost, `{"id":"1001","context":"phones","password":"new-secret"}`, "")
		if rec.Code < 500 || !strings.Contains(rec.Body.String(), "se guard") {
			t.Fatalf("reload failure hidden: %d %s", rec.Code, rec.Body.String())
		}
	}

}

func TestSIPUsersRejectAmbiguousPJSIPSectionsWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name, content, id string
	}{
		{"pjsip-template", strings.Replace(sipSeparateObjects, "[1001] ; endpoint", "[1001](base)", 1), "1001"},
		{"pjsip-duplicate", sipSeparateObjects + "[1001]\ntype=endpoint\n", "1001"},
		{"pjsip-include", sipSeparateObjects + "#include extra.conf\n", "1001"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := sipTestControl(t, tc.content, tc.content)
			p.reload = func(string) (string, error) { t.Fatal("unexpected reload"); return "", nil }
			rec := sipTestRequest(t, p.deletePJSIP, http.MethodDelete, "", tc.id)
			data, err := os.ReadFile(p.pjsipPath)
			if err != nil {
				t.Fatal(err)
			}
			if rec.Code != 409 || string(data) != tc.content {
				t.Fatalf("ambiguous section modified: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
