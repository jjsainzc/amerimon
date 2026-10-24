package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAudioModuleInstallation(t *testing.T) {
	data, err := os.ReadFile("postinstall.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	start := strings.Index(script, `    MODULES_CONF=`)
	end := strings.Index(script, `    EXTENSIONS_CONF=`)
	if start < 0 || end < start {
		t.Fatal("module installation block missing")
	}
	for _, installed := range []bool{true, false} {
		t.Run(map[bool]string{true: "installed", false: "missing"}[installed], func(t *testing.T) {
			dir := t.TempDir()
			moduleDir := filepath.Join(dir, "modules")
			if err := os.Mkdir(moduleDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if installed {
				if err := os.WriteFile(filepath.Join(moduleDir, "res_mutestream.so"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			initial := "[modules]\nautoload=no\nnoload => res_mutestream.so ; disabled\nload => app_rpt.so\n"
			path := filepath.Join(dir, "modules.conf")
			if err := os.WriteFile(path, []byte(initial), 0o640); err != nil {
				t.Fatal(err)
			}
			block := script[start:end]
			block = strings.ReplaceAll(block, `/usr/lib/*/asterisk/modules/"$module" /usr/lib/asterisk/modules/"$module"`, moduleDir+`/"$module"`)
			var previous string
			for run := 0; run < 2; run++ {
				cmd := exec.Command("sh", "-ec", block)
				cmd.Env = append(os.Environ(), "AST_DIR="+dir)
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%v: %s", err, output)
				}
				result, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				text := string(result)
				if !strings.Contains(text, "load => app_rpt.so") {
					t.Fatal("unrelated module changed")
				}
				if installed {
					if strings.Count(text, "load => res_mutestream.so") != 1 || strings.Contains(text, "noload => res_mutestream.so") {
						t.Fatalf("invalid mute configuration: %s", text)
					}
				} else if text != initial || !strings.Contains(string(output), "res_mutestream.so") {
					t.Fatal("missing module must warn without editing configuration")
				}
				if run > 0 && text != previous {
					t.Fatal("module installation is not idempotent")
				}
				previous = text
			}
			backup, err := os.ReadFile(path + ".bak-listen-live")
			if err != nil || string(backup) != initial {
				t.Fatal("original backup not preserved")
			}
		})
	}
}
