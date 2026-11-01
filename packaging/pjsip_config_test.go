package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
			"load => chan_pjsip.so ; intentionally disabled",
			"load => app_rpt.so",
			"load   = res_pjsip.so",
			"load   = func_pjsip_endpoint.so",
			"noload = res_pjsip_t38.so",
		} {
			if !strings.Contains(moduleText, required) {
				t.Fatalf("missing or changed module configuration %q:\n%s", required, moduleText)
			}
		}
		if strings.Contains(moduleText, "noload => chan_pjsip.so") ||
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
	patch, err := os.ReadFile("pjsip-modules-patch.conf")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(patch), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "load ") || strings.HasPrefix(line, "noload ") {
			if strings.Count(string(firstModules), line) != 1 {
				t.Fatalf("every patch directive must be added once even without its .so file: %q", line)
			}
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

func TestPJSIPConfigurationPreservesExistingTransportAndCustomSettings(t *testing.T) {
	dir := t.TempDir()
	initial := "[global]\nuser_agent=Customer PBX\n\n[transport-udp]\ntype=transport\nlocal_net=192.168.1.0/24\n"
	pjsipPath := filepath.Join(dir, "pjsip.conf")
	if err := os.WriteFile(pjsipPath, []byte(initial), 0o640); err != nil {
		t.Fatal(err)
	}

	runPJSIPConfiguration(t, dir, dir)

	config, err := os.ReadFile(pjsipPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(config) != initial {
		t.Fatalf("existing transport or custom settings were changed:\n%s", config)
	}
	if _, err := os.Stat(pjsipPath + ".bak-pjsip"); !os.IsNotExist(err) {
		t.Fatalf("existing transport should not be backed up or modified: %v", err)
	}

	runPJSIPConfiguration(t, dir, dir)
	config, err = os.ReadFile(pjsipPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(config) != initial {
		t.Fatalf("existing transport changed on a repeated install:\n%s", config)
	}
}

func TestPJSIPConfigurationAddsTransportWhenMissingOrCommented(t *testing.T) {
	for name, initial := range map[string]string{
		"missing":   "[global]\nuser_agent=Customer PBX\n",
		"commented": ";[transport-udp]\n;type=transport\n;protocol=udp\n;bind=0.0.0.0\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			pjsipPath := filepath.Join(dir, "pjsip.conf")
			if err := os.WriteFile(pjsipPath, []byte(initial), 0o640); err != nil {
				t.Fatal(err)
			}

			runPJSIPConfiguration(t, dir, dir)

			config, err := os.ReadFile(pjsipPath)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				"[transport-udp]",
				"type=transport",
				"protocol=udp",
				"bind=0.0.0.0",
			} {
				if !strings.Contains(string(config), required) {
					t.Fatalf("PJSIP configuration is missing %q:\n%s", required, config)
				}
			}
			if !strings.HasPrefix(string(config), initial) {
				t.Fatalf("existing or commented configuration was changed:\n%s", config)
			}
			backup, err := os.ReadFile(pjsipPath + ".bak-pjsip")
			if err != nil || string(backup) != initial {
				t.Fatalf("original pjsip.conf backup was not preserved: %v", err)
			}

			first := string(config)
			runPJSIPConfiguration(t, dir, dir)
			config, err = os.ReadFile(pjsipPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(config) != first {
				t.Fatalf("PJSIP transport merge is not idempotent:\n%s", config)
			}
		})
	}
}

func TestPJSIPConfigurationVerifiesActiveUDPTransport(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	if err := os.Mkdir(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "loaded.log")
	output := string(runPJSIPConfiguration(t, dir, dir,
		"PJSIP_TEST_LOG="+logPath,
		"PJSIP_TEST_STATE="+stateDir,
		"PJSIP_TEST_TRANSPORT_ACTIVE=yes"))
	if !strings.Contains(output, "[transport-udp] está activo en 0.0.0.0:5060/UDP") {
		t.Fatalf("active UDP transport was not verified: %s", output)
	}
	if strings.Contains(output, "se intenta recargar res_pjsip.so") {
		t.Fatalf("active UDP transport should not trigger a module reload: %s", output)
	}

	output = string(runPJSIPConfiguration(t, dir, dir,
		"PJSIP_TEST_LOG="+filepath.Join(dir, "socket-missing.log"),
		"PJSIP_TEST_STATE="+stateDir,
		"PJSIP_TEST_TRANSPORT_ACTIVE=yes",
		"PJSIP_TEST_SOCKET_INACTIVE=yes"))
	if !strings.Contains(output, "incluso después de intentar module reload res_pjsip.so") {
		t.Fatalf("configured transport without a UDP listener was not reloaded and reported: %s", output)
	}

	reloadStateDir := filepath.Join(dir, "reload-state")
	if err := os.Mkdir(reloadStateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reloadLogPath := filepath.Join(dir, "reload.log")
	output = string(runPJSIPConfiguration(t, dir, dir,
		"PJSIP_TEST_LOG="+filepath.Join(dir, "reload-modules.log"),
		"PJSIP_TEST_RELOAD_LOG="+reloadLogPath,
		"PJSIP_TEST_STATE="+reloadStateDir,
		"PJSIP_TEST_TRANSPORT_AFTER_RELOAD=yes"))
	if !strings.Contains(output, "está activo en 0.0.0.0:5060/UDP después de recargar res_pjsip.so") {
		t.Fatalf("UDP transport was not verified after module reload: %s", output)
	}
	reloadLog, err := os.ReadFile(reloadLogPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reloadLog), "reload:res_pjsip.so\n") {
		t.Fatalf("module reload was not attempted: %s", reloadLog)
	}

	failedStateDir := filepath.Join(dir, "failed-state")
	if err := os.Mkdir(failedStateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	output = string(runPJSIPConfiguration(t, dir, dir,
		"PJSIP_TEST_LOG="+filepath.Join(dir, "failed.log"),
		"PJSIP_TEST_STATE="+failedStateDir))
	if !strings.Contains(output, "incluso después de intentar module reload res_pjsip.so") {
		t.Fatalf("inactive UDP transport was not reported after module reload: %s", output)
	}
}

func runPJSIPConfiguration(t *testing.T, astDir, moduleDir string, extraEnv ...string) []byte {
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
	binDir := t.TempDir()
	mock := `#!/bin/sh
[ -n "$PJSIP_TEST_LOG" ] || exit 1
case "$2" in
    'core show version') printf 'Asterisk test\n' ;;
    'pjsip show transport transport-udp')
        if [ "$PJSIP_TEST_TRANSPORT_ACTIVE" = yes ] || [ -f "$PJSIP_TEST_STATE/transport-active" ]; then
            printf 'Transport: transport-udp\nProtocol: UDP\nBind: 0.0.0.0:5060\n'
        else
            printf 'Unable to find object transport-udp\n'
        fi
        ;;
    'module show like '*)
        module=${2#module show like }
        printf 'Module                         Description                              Use Count  Status      Support Level\n'
        if [ -f "$PJSIP_TEST_STATE/$module" ]; then
            printf '%s PJSIP resource 0 Running core\n' "$module"
        else
            printf '%s PJSIP resource 0 Not Running core\n' "$module"
        fi
        ;;
    'module load '*)
        module=${2#module load }
        printf '%s\n' "$module" >> "$PJSIP_TEST_LOG"
        if [ "$PJSIP_TEST_CHAIN" = yes ]; then
            dependency=
            case "$module" in
                res_pjsip_caller_id.so) dependency=res_pjsip_dialog_info_body_generator.so ;;
                res_pjsip_dialog_info_body_generator.so) dependency=res_pjsip_diversion.so ;;
                res_pjsip_diversion.so) dependency=res_pjsip_dtmf_info.so ;;
            esac
            if [ -n "$dependency" ] && [ ! -f "$PJSIP_TEST_STATE/$dependency" ]; then
                printf 'Dependency not running: %s\n' "$dependency"
                exit 0
            fi
        fi
        case " $PJSIP_TEST_FAIL " in
        *" $module "*)
            printf 'Dependency unavailable\nModule initialization declined\n'
            exit 0
            ;;
        esac
        : > "$PJSIP_TEST_STATE/$module"
        ;;
    'module reload res_pjsip.so')
        if [ -n "$PJSIP_TEST_RELOAD_LOG" ]; then
            printf 'reload:res_pjsip.so\n' >> "$PJSIP_TEST_RELOAD_LOG"
        fi
        if [ "$PJSIP_TEST_TRANSPORT_AFTER_RELOAD" = yes ]; then
            : > "$PJSIP_TEST_STATE/transport-active"
        fi
        printf "Module 'res_pjsip.so' reloaded successfully.\n"
        ;;
    *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "asterisk"), []byte(mock), 0o755); err != nil {
		t.Fatal(err)
	}
	netstatMock := `#!/bin/sh
if [ "$PJSIP_TEST_SOCKET_INACTIVE" != yes ] &&
   { [ "$PJSIP_TEST_TRANSPORT_ACTIVE" = yes ] || [ -f "$PJSIP_TEST_STATE/transport-active" ]; }; then
    printf 'udp        0      0 0.0.0.0:5060            0.0.0.0:*\n'
fi
`
	if err := os.WriteFile(filepath.Join(binDir, "netstat"), []byte(netstatMock), 0o755); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "configure-pjsip.sh")
	command.Env = append(os.Environ(),
		"AST_DIR="+astDir,
		"ASTERISK_MODULE_DIRS="+moduleDir,
		"PJSIP_MODULES_PATCH="+modulePatch,
		"PJSIP_EXTENSIONS_PATCH="+extensionsPatch,
		"PJSIP_MODULES_MERGER="+merger,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	command.Env = append(command.Env, extraEnv...)
	output, err := command.CombinedOutput()
	if slices.Contains(extraEnv, "PJSIP_TEST_EXPECT_FAILURE=yes") ||
		slices.Contains(extraEnv, "PJSIP_TEST_FAIL=chan_pjsip.so") {
		if err == nil {
			t.Fatal("installation must fail explicitly when a required module cannot run")
		}
		return output
	}
	if err != nil {
		t.Fatalf("PJSIP configuration failed: %v\n%s", err, output)
	}

	return output
}

func TestPJSIPOptionalPublicationModuleIsNotLoaded(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	moduleDir := filepath.Join(dir, "custom modules")
	for _, path := range []string{stateDir, moduleDir} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	module := "res_pjsip_publish_asterisk.so"
	if err := os.WriteFile(filepath.Join(moduleDir, module), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "loads.log")
	output := string(runPJSIPConfiguration(t, dir, dir,
		"PJSIP_TEST_LOG="+logPath,
		"PJSIP_TEST_STATE="+stateDir,
		"PJSIP_TEST_FAIL="+module))
	if strings.Contains(output, module) || strings.Contains(output, "Unable to load module") {
		t.Fatalf("optional publication module should not be loaded or reported:\n%s", output)
	}
	loads, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(loads), module) {
		t.Fatalf("optional publication module was passed to Asterisk:\n%s", loads)
	}
	modules, err := os.ReadFile(filepath.Join(dir, "modules.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(modules), "noload = "+module) {
		t.Fatalf("optional publication module must be disabled persistently:\n%s", modules)
	}
	if !strings.Contains(output, "todos los módulos requeridos están en estado Running") {
		t.Fatalf("required PJSIP modules should still report success:\n%s", output)
	}

	// Upgrade from a previous release should replace only its own explicit
	// load directive and must not keep attempting to load the optional module.
	if err := os.WriteFile(filepath.Join(dir, "modules.conf"), []byte(
		"[modules]\nautoload=yes\nload = "+module+" ; PJSIP Asterisk Event PUBLISH Support\n"), 0o640,
	); err != nil {
		t.Fatal(err)
	}
	runPJSIPConfiguration(t, dir, dir,
		"PJSIP_TEST_LOG="+logPath,
		"PJSIP_TEST_STATE="+stateDir)
	modules, err = os.ReadFile(filepath.Join(dir, "modules.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(modules), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "load = "+module) {
			t.Fatalf("upgrade retained the installer's old load directive:\n%s", modules)
		}
	}
	if !strings.Contains(string(modules), "noload = "+module) {
		t.Fatalf("upgrade must disable the optional module:\n%s", modules)
	}
}

func TestPJSIPLoadsDependenciesUntilAllModulesAreRunning(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	if err := os.Mkdir(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "loaded.log")
	output := runPJSIPConfiguration(t, dir, dir,
		"PJSIP_TEST_LOG="+logPath, "PJSIP_TEST_STATE="+stateDir, "PJSIP_TEST_CHAIN=yes")
	if strings.Contains(string(output), "no confirmó la carga") ||
		!strings.Contains(string(output), "todos los módulos requeridos están en estado Running") {
		t.Fatalf("dependency retries must complete without false warnings: %s", output)
	}
	loads, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(loads)), "\n")
	position := func(module string) int { return slices.Index(lines, module) }
	if position("res_sorcery_config.so") >= position("res_pjsip.so") ||
		position("res_pjsip.so") >= position("res_pjsip_session.so") ||
		position("res_pjsip_session.so") >= position("chan_pjsip.so") {
		t.Fatalf("infrastructure must load before SIP resources and the channel driver:\n%s", loads)
	}
	if strings.Count(string(loads), "res_pjsip_caller_id.so\n") != 4 {
		t.Fatalf("must resolve chains requiring more than two loading passes:\n%s", loads)
	}
	runPJSIPConfiguration(t, dir, dir,
		"PJSIP_TEST_LOG="+logPath, "PJSIP_TEST_STATE="+stateDir, "PJSIP_TEST_CHAIN=yes")
	repeatedLoads, err := os.ReadFile(logPath)
	if err != nil || string(repeatedLoads) != string(loads) {
		t.Fatalf("running modules must not be loaded again: %v", err)
	}
}

func TestPJSIPModuleDiscoveryAndLoading(t *testing.T) {
	for _, failedModule := range []string{"", "chan_pjsip.so", "res_pjsip.so"} {
		t.Run("failed-module="+failedModule, func(t *testing.T) {
			dir := t.TempDir()
			astDir := filepath.Join(dir, "asterisk")
			multiarchDir := filepath.Join(dir, "lib", "aarch64-linux-gnu", "asterisk", "modules")
			standardDir := filepath.Join(dir, "lib", "asterisk", "modules")
			stateDir := filepath.Join(dir, "state")
			for _, path := range []string{astDir, multiarchDir, standardDir, stateDir} {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}

			}
			for path, content := range map[string]string{
				filepath.Join(multiarchDir, "chan_pjsip.so"): "",
				filepath.Join(standardDir, "res_pjsip.so"):   "",
				filepath.Join(astDir, "modules.conf"): "[modules]\nautoload=no\n" +
					"noload=chan_pjsip.so\nload => chan_pjsip.so\nnoload => customer.so\n",
			} {
				if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			logPath := filepath.Join(dir, "loaded.log")
			dirs := filepath.Join(dir, "lib", "*", "asterisk", "modules") + " " + standardDir
			for run := 0; run < 2; run++ {
				env := []string{"PJSIP_TEST_LOG=" + logPath, "PJSIP_TEST_STATE=" + stateDir, "PJSIP_TEST_FAIL=" + failedModule}
				if failedModule != "" {
					env = append(env, "PJSIP_TEST_EXPECT_FAILURE=yes")
				}
				output := runPJSIPConfiguration(t, astDir, dirs, env...)
				for _, module := range []string{"chan_pjsip.so", "res_pjsip.so"} {
					if strings.Contains(string(output), "no instalado: "+module+";") {
						t.Fatalf("installed module was not discovered: %s", output)
					}
				}
				if failedModule != "" && !strings.Contains(string(output), "no confirmó la carga de "+failedModule) {
					t.Fatalf("failed runtime load was not reported: %s", output)
				}
				if failedModule != "" && !strings.Contains(string(output), "Respuesta de Asterisk: Dependency unavailable") {
					t.Fatalf("Asterisk error text must be reported even when the CLI exits successfully: %s", output)
				}
				if failedModule != "" {
					expectedPath := filepath.Join(multiarchDir, failedModule)
					if failedModule == "res_pjsip.so" {
						expectedPath = filepath.Join(standardDir, failedModule)
					}
					if !strings.Contains(string(output), expectedPath) {
						t.Fatalf("module discovery must report its actual location: %s", output)
					}
				}
				if failedModule == "" && strings.Contains(string(output), "no confirmó la carga") {
					t.Fatalf("running modules must not generate load warnings: %s", output)
				}
			}
			modules, err := os.ReadFile(filepath.Join(astDir, "modules.conf"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(modules), "noload=chan_pjsip.so") ||
				strings.Count(string(modules), "chan_pjsip.so") != 1 ||
				!strings.Contains(string(modules), "noload => customer.so") {
				t.Fatalf("required modules must be enabled once without changing other modules:\n%s", modules)
			}
			loads, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			expectedResourceLoads := 1
			if failedModule == "res_pjsip.so" {
				expectedResourceLoads = 3
			}
			if strings.Count(string(loads), "res_pjsip.so\n") != expectedResourceLoads {
				t.Fatalf("module must be loaded once and skipped when already loaded: %s", loads)
			}
			expectedLoads := 1
			if failedModule == "chan_pjsip.so" {
				expectedLoads = 3
			}
			if strings.Count(string(loads), "chan_pjsip.so\n") != expectedLoads {
				t.Fatalf("unexpected module load attempts: %s", loads)
			}
		})
	}
}
