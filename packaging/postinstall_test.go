package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostinstallContinuesAfterConfigurationFailures(t *testing.T) {
	data, err := os.ReadFile("postinstall.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, step string
	}{
		{"missing-1999", ""},
		{"files", "ficheros y credenciales"},
		{"services", "servicios y proxy web"},
		{"allstar", "allstar-public"},
		{"peerip", "EchoLink peerip"},
		{"telemetry", "telemetría EchoLink"},
		{"pjsip", "PJSIP"},
		{"modules", "módulos de audio"},
		{"listen-live", "dialplan Listen Live"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, path := range []string{
				"etc/asterisk", "etc/amerimon", "run/systemd/system",
				"usr/lib/amerimon", "usr/share/amerimon", "bin",
				"usr/lib/asterisk/modules",
			} {
				if err := os.MkdirAll(filepath.Join(root, path), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			write := func(path, content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			write("etc/amerimon/amerimon.env", "")
			write("etc/asterisk/extensions.conf", "[other]\nexten => 100,1,NoOp(Keep)\n")
			write("etc/asterisk/modules.conf", "[modules]\nautoload=yes\n")
			write("etc/asterisk/rpt.conf", "[41777]\nduplex = 1\n")
			echo := "[el0]\nastnode = 1999\n"
			if tc.name == "telemetry" {
				echo = "[el0]\nastnode = invalid\n"
			}
			write("etc/asterisk/echolink.conf", echo)
			write("usr/share/amerimon/extensions-allstar-public-patch.conf", "[allstar-public]\n")
			for name, failure := range map[string]string{
				"merge-allstar-public.awk": "allstar",
				"echolink-peerip.awk":      "peerip",
				"listen-live-dialplan.awk": "listen-live",
			} {
				patch := "{ print }\n"
				if tc.name == failure {
					patch = "BEGIN { print \"BROKEN\"; exit 1 }\n"
				}
				write("usr/lib/amerimon/"+name, patch)
			}
			telemetry, err := os.ReadFile("echolink-telemetry.awk")
			if err != nil {
				t.Fatal(err)
			}
			write("usr/lib/amerimon/echolink-telemetry.awk", string(telemetry))
			pjsip := "#!/bin/sh\nprintf 'pjsip-attempted\\n'\n"
			if tc.name == "pjsip" {
				pjsip += "echo 'PJSIP configuration error' >&2\nexit 1\n"
			}
			write("usr/lib/amerimon/configure-pjsip.sh", pjsip)
			write("usr/lib/asterisk/modules/res_agi.so", "")
			write("bin/getent", "#!/bin/sh\nexit 1\n")
			chown := "#!/bin/sh\nexit 0\n"
			if tc.name == "files" {
				chown = "#!/bin/sh\necho 'credentials permission error' >&2\nexit 1\n"
			}
			write("bin/chown", chown)
			systemctl := "#!/bin/sh\nexit 0\n"
			if tc.name == "services" {
				systemctl = "#!/bin/sh\necho 'service configuration error' >&2\nexit 1\n"
			}
			write("bin/systemctl", systemctl)
			// Keep all service and web-server probes inside the fixture.
			for _, name := range []string{"nginx", "a2enmod"} {
				write("bin/"+name, "#!/bin/sh\nexit 1\n")
			}
			write("bin/asterisk", "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$POSTINSTALL_TEST_ROOT/commands\"\nexit 0\n")
			if tc.name == "modules" {
				write("bin/sed", "#!/bin/sh\necho 'modules configuration error' >&2\nexit 1\n")
			}
			script := string(data)
			for _, prefix := range []string{"/etc/", "/var/lib/", "/run/", "/usr/lib/", "/usr/share/", "/usr/local/"} {
				script = strings.ReplaceAll(script, prefix, root+prefix)
			}
			cmd := exec.Command("sh", "-c", script)
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+":"+os.Getenv("PATH"), "POSTINSTALL_TEST_ROOT="+root)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("configuration failure aborted postinstall: %v\n%s", err, output)
			}
			if tc.step != "" && !strings.Contains(string(output), "Aviso: falló la configuración de "+tc.step) {
				t.Fatalf("failed step was not reported: %s", output)
			}
			if tc.name == "missing-1999" && !strings.Contains(string(output), "missing RPT section [1999], skipped") {
				t.Fatalf("missing remote node was not reported: %s", output)
			}
			extensions, err := os.ReadFile(filepath.Join(root, "etc/asterisk/extensions.conf"))
			if err != nil || strings.Contains(string(extensions), "BROKEN") {
				t.Fatalf("failed patch wrote partial configuration: %v %s", err, extensions)
			}
			if tc.name != "listen-live" && !strings.Contains(string(extensions), "[amerimon-listen-node]") {
				t.Fatalf("later configuration did not run: %s", extensions)
			}
			rpt, err := os.ReadFile(filepath.Join(root, "etc/asterisk/rpt.conf"))
			if err != nil || string(rpt) != "[41777]\nduplex = 1\n" {
				t.Fatalf("skipped or failed telemetry modified rpt.conf: %v %s", err, rpt)
			}
			commands, err := os.ReadFile(filepath.Join(root, "commands"))
			if tc.name != "listen-live" && (err != nil || !strings.Contains(string(commands), "dialplan reload")) {
				t.Fatalf("postinstall did not reach its last step: %v %s", err, commands)
			}
		})
	}
}
