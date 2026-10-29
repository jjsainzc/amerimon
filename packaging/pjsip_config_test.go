package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPJSIPConfigurationPreservesExistingSettings(t *testing.T) {
	dir := t.TempDir()
	astDir := filepath.Join(dir, "asterisk")
	moduleDir := filepath.Join(dir, "modules")
	if err := os.MkdirAll(astDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, module := range []string{"chan_pjsip.so", "res_pjsip.so", "func_pjsip_endpoint.so"} {
		if err := os.WriteFile(filepath.Join(moduleDir, module), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	initialModules := "[modules]\nautoload=no\nnoload => chan_pjsip.so ; intentionally disabled\nload => app_rpt.so\n[custom]\nload => customer_module.so\n"
	initialExtensions := "[sip-phones]\nexten => 100,1,NoOp(customer route)\n[custom-context]\nexten => 200,1,NoOp(keep)\n"
	modulesPath := filepath.Join(astDir, "modules.conf")
	extensionsPath := filepath.Join(astDir, "extensions.conf")
	if err := os.WriteFile(modulesPath, []byte(initialModules), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extensionsPath, []byte(initialExtensions), 0o640); err != nil {
		t.Fatal(err)
	}

	for run := 0; run < 2; run++ {
		output := runPJSIPConfiguration(t, astDir, moduleDir)
		if strings.Contains(string(output), "ya define _X.") {
			t.Fatal("unrelated custom extensions should coexist with the PJSIP route")
		}
		modules, err := os.ReadFile(modulesPath)
		if err != nil {
			t.Fatal(err)
		}
		moduleText := string(modules)
		for _, required := range []string{
			"noload => chan_pjsip.so ; intentionally disabled",
			"load => app_rpt.so",
			"load   = res_pjsip.so",
			"load   = func_pjsip_endpoint.so",
			"noload = res_pjsip_t38.so",
		} {
			if !strings.Contains(moduleText, required) {
				t.Fatalf("missing or changed module configuration %q:\n%s", required, moduleText)
			}
		}
		if strings.Contains(moduleText, "load   = chan_pjsip.so") ||
			strings.Count(moduleText, "load   = res_pjsip.so") != 1 {
			t.Fatalf("existing module policy was overwritten or duplicated:\n%s", moduleText)
		}

		extensions, err := os.ReadFile(extensionsPath)
		if err != nil {
			t.Fatal(err)
		}
		extensionText := string(extensions)
		for _, required := range []string{
			"exten => 100,1,NoOp(customer route)",
			"exten => 200,1,NoOp(keep)",
			"exten => _X.,1,Ringing()",
			"[rpt-internal]",
			"exten => _X.,1,Rpt(${NODE},Pv)",
		} {
			if !strings.Contains(extensionText, required) {
				t.Fatalf("missing or changed dialplan configuration %q:\n%s", required, extensionText)
			}
		}
		if strings.Count(extensionText, "exten => _X.,1,Ringing()") != 1 ||
			strings.Count(extensionText, "[rpt-internal]") != 1 {
			t.Fatalf("an existing context was overwritten or a patch was duplicated:\n%s", extensionText)
		}
	}

	backupModules, err := os.ReadFile(modulesPath + ".bak-pjsip")
	if err != nil || string(backupModules) != initialModules {
		t.Fatalf("original modules.conf backup was not preserved: %v", err)
	}
	backupExtensions, err := os.ReadFile(extensionsPath + ".bak-pjsip")
	if err != nil || string(backupExtensions) != initialExtensions {
		t.Fatalf("original extensions.conf backup was not preserved: %v", err)
	}
}

func TestPJSIPConfigurationDoesNotOverrideConflictingCustomRoute(t *testing.T) {
	dir := t.TempDir()
	astDir := filepath.Join(dir, "asterisk")
	moduleDir := filepath.Join(dir, "modules")
	if err := os.MkdirAll(astDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	extensionsPath := filepath.Join(astDir, "extensions.conf")
	customRoute := "[sip-phones]\nexten => _X.,1,Dial(PJSIP/${EXTEN},60,rT)\n"
	if err := os.WriteFile(extensionsPath, []byte(customRoute), 0o640); err != nil {
		t.Fatal(err)
	}

	output := runPJSIPConfiguration(t, astDir, moduleDir)
	extensions, err := os.ReadFile(extensionsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "ya define _X.") {
		t.Fatalf("expected a warning for the conflicting custom route; output: %s\ndialplan: %s", output, extensions)
	}
	if !strings.Contains(string(extensions), "Dial(PJSIP/${EXTEN},60,rT)") ||
		strings.Contains(string(extensions), "exten => _X.,1,Ringing()") {
		t.Fatalf("conflicting custom route was changed or duplicated:\n%s", extensions)
	}
	if !strings.Contains(string(extensions), "[rpt-internal]") {
		t.Fatalf("a non-conflicting PJSIP context was not added:\n%s", extensions)
	}
}

func TestPJSIPConfigurationCreatesMissingFilesAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	astDir := filepath.Join(dir, "asterisk")
	moduleDir := filepath.Join(dir, "modules")
	if err := os.MkdirAll(astDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, module := range []string{"chan_pjsip.so", "res_pjsip.so"} {
		if err := os.WriteFile(filepath.Join(moduleDir, module), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runPJSIPConfiguration(t, astDir, moduleDir)
	modulesPath := filepath.Join(astDir, "modules.conf")
	extensionsPath := filepath.Join(astDir, "extensions.conf")
	firstModules, err := os.ReadFile(modulesPath)
	if err != nil {
		t.Fatal(err)
	}
	firstExtensions, err := os.ReadFile(extensionsPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"[modules]", "autoload=yes", "load   = chan_pjsip.so", "load   = res_pjsip.so"} {
		if !strings.Contains(string(firstModules), required) {
			t.Fatalf("new modules.conf is missing %q:\n%s", required, firstModules)
		}
	}
	for _, required := range []string{"[sip-phones]", "[rpt-internal]", "Local/${NODE}@rpt-internal"} {
		if !strings.Contains(string(firstExtensions), required) {
			t.Fatalf("new extensions.conf is missing %q:\n%s", required, firstExtensions)
		}
	}

	runPJSIPConfiguration(t, astDir, moduleDir)
	secondModules, err := os.ReadFile(modulesPath)
	if err != nil {
		t.Fatal(err)
	}
	secondExtensions, err := os.ReadFile(extensionsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstModules) != string(secondModules) || string(firstExtensions) != string(secondExtensions) {
		t.Fatal("a second installation modified PJSIP configuration")
	}
	if _, err := os.Stat(modulesPath + ".bak-pjsip"); !os.IsNotExist(err) {
		t.Fatal("new modules.conf should not have a backup of an empty file")
	}
	if _, err := os.Stat(extensionsPath + ".bak-pjsip"); !os.IsNotExist(err) {
		t.Fatal("new extensions.conf should not have a backup of an empty file")
	}
}

func runPJSIPConfiguration(t *testing.T, astDir, moduleDir string) []byte {
	t.Helper()
	modulePatch, err := filepath.Abs("pjsip-modules-patch.conf")
	if err != nil {
		t.Fatal(err)
	}
	extensionsPatch, err := filepath.Abs("pjsip-extensions-sip-phones-patch.conf")
	if err != nil {
		t.Fatal(err)
	}
	merger, err := filepath.Abs("merge-pjsip-modules.awk")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "configure-pjsip.sh")
	command.Env = append(os.Environ(),
		"AST_DIR="+astDir,
		"ASTERISK_MODULE_DIRS="+moduleDir,
		"PJSIP_MODULES_PATCH="+modulePatch,
		"PJSIP_EXTENSIONS_PATCH="+extensionsPatch,
		"PJSIP_MODULES_MERGER="+merger,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("PJSIP configuration failed: %v\n%s", err, output)
	}
	return output
}
