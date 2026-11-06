package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEchoLinkTelemetryPatch(t *testing.T) {
	for _, setting := range []string{"", ";echolinkdefault = 2\n;echolinkdynamic = 1\n", "echolinkdefault = 2\n", "EchoLinkDefault = 0\necholinkdefault = 2\n", "echolinkdefault = 1\n"} {
		t.Run(setting, func(t *testing.T) {
			dir := t.TempDir()
			echoPath := filepath.Join(dir, "echolink.conf")
			rptPath := filepath.Join(dir, "rpt.conf")
			writeTelemetryFixture(t, echoPath, "[el0]\ncall = *TEST*\nastnode = 41777 ; main\n")
			template := "[node-main](!)\necholinkdefault = 2\nduplex = 1\n"
			other := "[99999](node-main)\necholinkdefault = 0\n[nodes]\n41777 = radio@127.0.0.1/41777,NONE\n"
			initial := template + "[41777](node-main)\n" + setting + "telemdefault = 2\n" + other
			for run := 0; run < 2; run++ {
				writeTelemetryFixture(t, rptPath, initial)
				output, err := exec.Command("awk", "-f", "echolink-telemetry.awk", echoPath, rptPath).Output()
				if err != nil {
					t.Fatal(err)
				}
				result := string(output)
				if !strings.HasPrefix(result, template+"[41777](node-main)\necholinkdefault = 1\n") ||
					!strings.HasSuffix(result, "telemdefault = 2\n"+other) {
					t.Fatalf("target setting missing or unrelated configuration changed: %s", result)
				}
				active := 0
				for _, line := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(result, template), other), "\n") {
					if strings.HasPrefix(strings.ToLower(line), "echolinkdefault") {
						active++
					}
				}
				if active != 1 {
					t.Fatal("target must have exactly one active echolinkdefault")
				}
				if run > 0 && result != initial {
					t.Fatal("patch is not idempotent")
				}
				initial = result
			}
		})
	}
}

func TestEchoLinkTelemetryMultipleNodes(t *testing.T) {
	dir := t.TempDir()
	echoPath := filepath.Join(dir, "echolink.conf")
	rptPath := filepath.Join(dir, "rpt.conf")
	writeTelemetryFixture(t, echoPath, "[el0]\nastnode = 41777\n[el1]\nastnode = 616262\n")
	writeTelemetryFixture(t, rptPath, "[41777]\necholinkdefault = 2\n[616262](node-main)\necholinkdefault = 0\n")
	output, err := exec.Command("awk", "-f", "echolink-telemetry.awk", echoPath, rptPath).Output()
	if err != nil || string(output) != "[41777]\necholinkdefault = 1\n[616262](node-main)\necholinkdefault = 1\n" {
		t.Fatalf("both EchoLink nodes must be patched: %v %s", err, output)
	}
}

func TestEchoLinkTelemetryRejectsUnsafeConfiguration(t *testing.T) {
	for _, tc := range []struct{ echo, rpt string }{
		{"[el0]\nastnode = 41777\n", "[99999]\n"},
		{"[el0]\nastnode = invalid\n", "[41777]\n"},
		{"[el0]\nastnode = 41777\n", "[41777]\n[41777]\n"},
		{"[el0]\nastnode = 41777\n", "[41777](!)\n"},
		{"[el0]\n;astnode = 41777\n", "[41777]\n"},
	} {
		dir := t.TempDir()
		echoPath := filepath.Join(dir, "echolink.conf")
		rptPath := filepath.Join(dir, "rpt.conf")
		writeTelemetryFixture(t, echoPath, tc.echo)
		writeTelemetryFixture(t, rptPath, tc.rpt)
		output, err := exec.Command("awk", "-f", "echolink-telemetry.awk", echoPath, rptPath).Output()
		if err == nil || len(output) != 0 {
			t.Fatalf("unsafe configuration must fail without partial output: %v %s", err, output)
		}
	}
}

func TestEchoLinkTelemetryInstallation(t *testing.T) {
	data, err := os.ReadFile("postinstall.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	start := strings.Index(script, `    ECHOLINK_CONF=`)
	end := strings.Index(script, `    if [ -r /usr/lib/amerimon/configure-pjsip.sh ]; then`)
	if start < 0 || end < start {
		t.Fatal("EchoLink telemetry installation block missing")
	}
	patch, err := filepath.Abs("echolink-telemetry.awk")
	if err != nil {
		t.Fatal(err)
	}
	block := strings.ReplaceAll(script[start:end], "/usr/lib/amerimon/echolink-telemetry.awk", patch)
	dir := t.TempDir()
	rptPath := filepath.Join(dir, "rpt.conf")
	initial := "[41777](node-main)\nduplex = 1\n;echolinkdefault = 2\n"
	writeTelemetryFixture(t, rptPath, initial)
	writeTelemetryFixture(t, filepath.Join(dir, "echolink.conf"), "[el0]\nastnode = 41777\n")
	mock := filepath.Join(dir, "asterisk")
	writeTelemetryFixture(t, mock, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$AST_DIR/commands\"\nexit 0\n")
	if err := os.Chmod(mock, 0o700); err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		cmd := exec.Command("sh", "-ec", block)
		cmd.Env = append(os.Environ(), "AST_DIR="+dir, "PATH="+dir+":"+os.Getenv("PATH"))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, output)
		}
		result, err := os.ReadFile(rptPath)
		if err != nil || !strings.Contains(string(result), "[41777](node-main)\necholinkdefault = 1\n") {
			t.Fatalf("patch not applied: %v %s", err, result)
		}
		if run == 0 && !strings.Contains(string(output), "desconecta y reconecta") {
			t.Fatal("installation must warn about reconnecting links")
		}
		info, err := os.Stat(rptPath)
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Fatal("configuration permissions changed")
		}
	}
	backup, err := os.ReadFile(rptPath + ".bak-echolink-telemetry")
	if err != nil || string(backup) != initial {
		t.Fatal("original backup not preserved")
	}
	commands, err := os.ReadFile(filepath.Join(dir, "commands"))
	if err != nil || strings.Count(string(commands), "module reload app_rpt.so") != 1 {
		t.Fatal("reload must happen only after a configuration change")
	}
	writeTelemetryFixture(t, filepath.Join(dir, "echolink.conf"), "[el0]\nastnode = 99999\n")
	before, err := os.ReadFile(rptPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-ec", block)
	cmd.Env = append(os.Environ(), "AST_DIR="+dir, "PATH="+dir+":"+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(output), "no se modific") {
		t.Fatalf("invalid target must explicitly fail: %v %s", err, output)
	}
	after, err := os.ReadFile(rptPath)
	if err != nil || string(after) != string(before) {
		t.Fatal("failed patch must preserve rpt.conf")
	}
	temps, err := filepath.Glob(rptPath + ".echolink-telemetry.*")
	if err != nil || len(temps) != 0 {
		t.Fatal("failed patch left temporary files")
	}
	release, err := os.ReadFile("../.goreleaser.yaml")
	if err != nil || !strings.Contains(string(release), "src: packaging/echolink-telemetry.awk\n        dst: /usr/lib/amerimon/echolink-telemetry.awk") {
		t.Fatal("Debian package must include the telemetry patch")
	}
}

func writeTelemetryFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}
