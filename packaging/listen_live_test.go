package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalInstallerAppliesListenLiveMigration(t *testing.T) {
	data, err := os.ReadFile("../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	hook := strings.Index(script, `"${SUDO[@]}" sh packaging/postinstall.sh`)
	if hook < 0 {
		t.Fatal("local installer must execute the shared postinstall hook")
	}
	for _, resource := range []string{
		"packaging/listen-live-dialplan.awk",
		"packaging/audio_eagi.sh",
		"packaging/astdb.sh",
		"scripts/limpiar_canales_listen_live.sh",
		"packaging/amerimon-astdb.service",
		"packaging/amerimon-astdb.timer",
		"packaging/amerimon.service",
	} {
		index := strings.Index(script, resource)
		if index < 0 || index > hook {
			t.Fatalf("resource %s must be installed before running postinstall", resource)
		}
	}
	if strings.Contains(script, "systemctl enable --now amerimon.service") {
		t.Fatal("local installer must use the shared hook to restart an active service")
	}
}

func TestListenLiveDialplanInstallation(t *testing.T) {
	postinstall, err := os.ReadFile("postinstall.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(postinstall), "app_verbose.so") {
		t.Fatal("Verbose and Log require app_verbose.so in the installed module list")
	}
	start := strings.Index(string(postinstall), `    EXTENSIONS_CONF="$AST_DIR/extensions.conf"`)
	end := strings.Index(string(postinstall), `        if command -v asterisk`)
	if start < 0 || end < start {
		t.Fatal("Listen Live installation block not found")
	}
	script := string(postinstall[start:end]) + "\nfi\n"
	migration, err := filepath.Abs("listen-live-dialplan.awk")
	if err != nil {
		t.Fatal(err)
	}
	script = strings.ReplaceAll(script, "/usr/lib/amerimon/listen-live-dialplan.awk", migration)
	pjsip := "exten => _.,1,NoOp(Espiando PJSIP/${EXTEN})\n same => n,Answer()\n same => n,ChanSpy(PJSIP/${EXTEN},q)\n same => n,Hangup()\n"
	old := "exten => nodo,1,NoOp(Espiando nodo local ${NODE})\n same => n,Answer()\n same => n,ChanSpy(SimpleUSB/${NODE},q)\n same => n,Hangup()\n"
	pseudo := strings.ReplaceAll(old, "SimpleUSB/${NODE}", "Local/pseudo@default")
	custom := "exten => nodo,1,Answer()\n same => n,ChanSpy(Radio/custom,q)\n same => n,Hangup()\n"
	monitor := "[amerimon-listen-node]\nexten => h,1,NoOp(Listen Live node channel ended)\n same => n,Hangup()\nexten => _X.,1,NoOp(Listen Live monitor nodo ${EXTEN})\n same => n,Answer()\n same => n,Rpt(${EXTEN},Pm)\n same => n,Hangup()\n"
	protected := "[other-context]\nexten => 100,1,NoOp(Keep this)\n same => n,Hangup()\n"
	for _, tc := range []struct {
		name, initial string
		custom        bool
	}{
		{"fresh", protected, false},
		{"simpleusb", "[listen-live-spy]\n" + pjsip + old + protected, false},
		{"pseudo", "[listen-live-spy]\n" + pjsip + pseudo + protected, false},
		{"duplicate", "[listen-live-spy]\n" + pjsip + old + pseudo + protected, false},
		{"custom", "[listen-live-spy]\n" + pjsip + custom + protected, true},
		{"intermediate-monitor", monitor + "[listen-live-spy]\n" + pjsip + strings.ReplaceAll(old, "ChanSpy(SimpleUSB/${NODE},q)", "Rpt(${NODE},Pm)") + protected, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "extensions.conf")
			if err := os.WriteFile(path, []byte(tc.initial), 0o640); err != nil {
				t.Fatal(err)
			}
			var first string
			for run := 0; run < 2; run++ {
				cmd := exec.Command("sh", "-ec", script)
				cmd.Env = append(os.Environ(), "AST_DIR="+dir)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("install failed: %v\n%s", err, output)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				result := string(data)
				for _, required := range []string{protected, pjsip, "[amerimon-listen-node]", "AST_CONFIG(${AMERIMON_RPT_CONFIG},${EXTEN},rxchannel,-1)", "ChanSpy(${AMERIMON_RX},q)", "AMERIMON_RX=${AMERIMON_RX}@default", "EAGI(/usr/local/bin/audio_eagi.sh,${EXTEN},8086)"} {
					if !strings.Contains(result, required) {
						t.Fatalf("missing or modified %q\n%s", required, result)
					}
				}
				if strings.Count(result, "exten => nodo,") != 1 {
					t.Fatalf("duplicate node extension:\n%s", result)
				}
				if strings.Contains(result, "Rpt(") || strings.Contains(result, "SimpleUSB/${NODE}") {
					t.Fatalf("unvalidated monitor or stale hardcoded channel:\n%s", result)
				}
				if tc.custom && !strings.Contains(result, custom) {
					t.Fatal("custom extension overwritten")
				}
				if run == 0 {
					first = result
				} else if result != first {
					t.Fatal("second installation changed dialplan")
				}
			}
			backup, err := os.ReadFile(path + ".bak-listen-live")
			if err != nil || string(backup) != tc.initial {
				t.Fatalf("original backup not preserved: %v", err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o640 {
				t.Fatalf("permissions changed: %v", err)
			}
		})
	}
}
