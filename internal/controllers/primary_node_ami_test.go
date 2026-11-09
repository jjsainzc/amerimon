package controllers

// Este archivo contiene pruebas para la configuración y comunicación AMI del nodo principal.

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"amerimon/internal/config"
	"github.com/labstack/echo/v4"
)

func TestPrimaryNodeAMIConfigSavePreservesCustomSettingsAndProtectsLoopback(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	managerPath := p.managerPath
	manager := "[general]\nbindaddr = 127.0.0.1\nwebenabled = no\n[amerimon]\nsecret = old_secret\nread = system,call\npermit=172.16.0.0/255.240.0.0\ndeny=192.0.2.0/255.255.255.0\n"
	if err := os.WriteFile(managerPath, []byte(manager), 0o600); err != nil {
		t.Fatal(err)
	}
	amiCalls := 0
	p.ami = func(command string) (string, error) {
		if command != "manager reload" {
			t.Fatalf("unexpected AMI command: %s", command)
		}
		amiCalls++
		return "Response: Success", nil
	}
	p.amiLogin = func(host, port, user, secret, command string) (string, error) {
		if host != "127.0.0.1" || port != "5038" || user != "amerimon" ||
			secret != "new_secret" || command != "manager show settings" {
			t.Fatalf("unexpected AMI verification parameters: %q %q %q %q %q", host, port, user, secret, command)
		}
		return "Response: Success", nil
	}
	c, rec := nodeTestContext(http.MethodPut, primaryAMIRequest{
		Secret: "new_secret", BindAddress: "0.0.0.0",
		Permit: []string{"192.168.1.37/24"}, Deny: []string{"10.5.6.7/8"},
	})
	if err := p.saveAMIConfig(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || amiCalls != 1 {
		t.Fatalf("save failed or manager reload count incorrect: status=%d body=%s reloads=%d", rec.Code, rec.Body, amiCalls)
	}
	updated, err := os.ReadFile(managerPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"bindaddr = 0.0.0.0", "webenabled = no", "secret = new_secret", "read = system,call",
		"permit=172.16.0.0/255.240.0.0", "deny=192.0.2.0/255.255.255.0",
		"permit=192.168.1.0/255.255.255.0", "permit=127.0.0.1/255.255.255.255", "permit=::1/128",
		"deny=10.0.0.0/255.0.0.0",
	} {
		if !strings.Contains(string(updated), expected) {
			t.Fatalf("manager.conf missing %q after update:\n%s", expected, updated)
		}
	}
	yamlData, err := os.ReadFile(config.App.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(yamlData), `secret: "${AMI_SECRET}"`) ||
		!strings.Contains(string(yamlData), `host: "127.0.0.1"`) {
		t.Fatalf("AMI yaml values were not updated safely:\n%s", yamlData)
	}
	envData, err := os.ReadFile(config.App.EnvPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envData), `AMI_SECRET="new_secret"`) ||
		!strings.Contains(string(envData), `AMI_USER="amerimon"`) {
		t.Fatalf("AMI environment values were not updated:\n%s", envData)
	}
	if config.App.AMI.Secret != "new_secret" || config.App.AMI.Host != "127.0.0.1" {
		t.Fatal("runtime AMI configuration was not updated")
	}
}

func TestPrimaryNodeAMIReadReturnsCurrentUserSecretWithoutCaching(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	c, rec := nodeTestContext(http.MethodGet, nil)
	if err := p.getAMIConfig(c); err != nil {
		t.Fatal(err)
	}
	var got primaryAMIConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.User != "amerimon" || got.Secret != "old_secret" || !got.SecretSet ||
		got.BindAddress != "127.0.0.1" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected AMI config response: %s", rec.Body)
	}
}

func TestPrimaryNodeAMISaveRequiresExplicitEnvironmentPath(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	config.App.EnvPath = ""
	managerBefore, err := os.ReadFile(p.managerPath)
	if err != nil {
		t.Fatal(err)
	}
	yamlBefore, err := os.ReadFile(config.App.Path)
	if err != nil {
		t.Fatal(err)
	}

	c, rec := nodeTestContext(http.MethodPut, primaryAMIRequest{
		Secret: "new_secret", BindAddress: "127.0.0.1",
	})
	err = p.saveAMIConfig(c)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected missing environment path to be rejected: status=%d body=%s err=%v", rec.Code, rec.Body, err)
	}
	managerAfter, _ := os.ReadFile(p.managerPath)
	yamlAfter, _ := os.ReadFile(config.App.Path)
	if string(managerAfter) != string(managerBefore) || string(yamlAfter) != string(yamlBefore) {
		t.Fatal("save without an explicit environment path changed configuration files")
	}
}

func TestPrimaryNodeAMIDenyCannotBlockBackendOrChangeFiles(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	managerBefore, _ := os.ReadFile(p.managerPath)
	yamlBefore, _ := os.ReadFile(config.App.Path)
	envBefore, _ := os.ReadFile(config.App.EnvPath)
	called := false
	p.ami = func(string) (string, error) {
		called = true
		return "", nil
	}
	c, rec := nodeTestContext(http.MethodPut, primaryAMIRequest{
		Secret: "new_secret", BindAddress: "127.0.0.1", Deny: []string{"127.0.0.0/8"},
	})
	err := p.saveAMIConfig(c)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatal("expected the loopback-blocking rule to be rejected")
	}
	if called {
		t.Fatalf("expected safe conflict without AMI reload, called=%v body=%s", called, rec.Body)
	}
	for path, before := range map[string][]byte{
		p.managerPath: managerBefore, config.App.Path: yamlBefore,
		config.App.EnvPath: envBefore,
	} {
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("refused ACL update changed %s: %v", path, err)
		}
	}
}

func TestPrimaryNodeAMIConfigRollsBackWhenNewCredentialsFail(t *testing.T) {
	p, _, _ := primaryNodeFixture(t)
	managerBefore, _ := os.ReadFile(p.managerPath)
	yamlBefore, _ := os.ReadFile(config.App.Path)
	envPath := config.App.EnvPath
	envBefore, _ := os.ReadFile(envPath)
	reloads := 0
	p.ami = func(command string) (string, error) {
		if command != "manager reload" {
			t.Fatalf("unexpected AMI command: %s", command)
		}
		reloads++
		return "Response: Success", nil
	}
	p.amiLogin = func(_, _, _, _, _ string) (string, error) {
		return "", errors.New("new credentials rejected")
	}
	c, rec := nodeTestContext(http.MethodPut, primaryAMIRequest{
		Secret: "new_secret", BindAddress: "0.0.0.0", Permit: []string{"192.168.1.0/24"},
	})
	err := p.saveAMIConfig(c)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusInternalServerError {
		t.Fatal("expected the failed verification to return an error")
	}
	if reloads != 2 {
		t.Fatalf("expected verification failure and rollback reload, reloads=%d body=%s", reloads, rec.Body)
	}
	for path, before := range map[string][]byte{
		p.managerPath: managerBefore, config.App.Path: yamlBefore, envPath: envBefore,
	} {
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("rollback did not restore %s: %v", path, err)
		}
	}
	if config.App.AMI.Secret != "old_secret" {
		t.Fatal("runtime secret changed despite failed verification")
	}
}

func TestManagerNetworkHelpersNormalizeAndRejectInvalidMasks(t *testing.T) {
	got, err := normalizeManagerNetworks([]string{"192.168.1.37/24", "10.1.2.3/255.255.0.0"})
	if err != nil || strings.Join(got, ",") != "192.168.1.0/24,10.1.0.0/16" {
		t.Fatalf("unexpected normalized networks %v (%v)", got, err)
	}
	if _, err := normalizeManagerNetworks([]string{"192.168.1.0/255.0.255.0"}); err == nil {
		t.Fatal("non-contiguous netmask should be rejected")
	}
}
