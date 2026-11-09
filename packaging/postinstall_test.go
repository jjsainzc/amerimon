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
		{"existing-credentials", ""},
		{"jwt", "secreto JWT"},
		{"ami", "credenciales AMI"},
		{"allmon", "allmon.ini"},
		{"state", "directorio de estado"},
		{"audio", "directorio de audio"},
		{"terminal", "clave de terminal"},
		{"reload", "recarga de systemd"},
		{"enable", "habilitación de Amerimon"},
		{"restart", "reinicio de Amerimon"},
		{"timer", "temporizador astdb"},
		{"proxy", "proxy web"},
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
				"var/lib/amerimon",
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
			write("etc/asterisk/manager.conf", "[general]\nenabled = yes\n[testuser]\nsecret = fixture-only\n")
			if tc.name == "existing-credentials" {
				write("etc/amerimon/amerimon.env", "AMI_USER=\"existing-user\"\nAMI_SECRET=\"existing-fixture\"\n")
				write("etc/amerimon/jwt.secret", "existing-jwt-fixture")
				write("var/lib/amerimon/terminal-credentials.key", "existing-terminal-fixture")
			}
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
			write("bin/getent", "#!/bin/sh\nexit 0\n")
			chown := "#!/bin/sh\nexit 0\n"
			for name, target := range map[string]string{
				"jwt": "/etc/amerimon/jwt.secret", "ami": "/etc/amerimon/amerimon.env",
				"allmon": "/etc/amerimon/allmon.ini", "state": "/var/lib/amerimon",
				"terminal": "/var/lib/amerimon/terminal-credentials.key",
			} {
				if tc.name == name {
					chown = "#!/bin/sh\nfor arg do\nif [ \"$arg\" = \"$POSTINSTALL_TEST_ROOT" + target + "\" ]; then\necho 'permission error' >&2\nexit 1\nfi\ndone\nexit 0\n"
				}
			}
			write("bin/chown", chown)
			write("bin/install", "#!/bin/sh\nexit "+map[bool]string{true: "1", false: "0"}[tc.name == "audio"]+"\n")
			systemctl := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$POSTINSTALL_TEST_ROOT/services\"\n"
			for name, args := range map[string]string{
				"reload": "daemon-reload", "enable": "enable amerimon.service",
				"restart": "restart amerimon.service", "timer": "enable --now amerimon-astdb.timer",
			} {
				if tc.name == name {
					systemctl += "if [ \"$*\" = \"" + args + "\" ]; then\necho 'service configuration error' >&2\nexit 1\nfi\n"
				}
			}
			write("bin/systemctl", systemctl+"exit 0\n")
			proxy := "#!/bin/sh\necho 'proxy-attempted'\nexit 0\n"
			if tc.name == "proxy" {
				proxy = "#!/bin/sh\necho 'proxy-attempted'\nexit 1\n"
			}
			write("usr/lib/amerimon/configure-nginx.sh", proxy)
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
			warning := "Aviso: falló la configuración de " + tc.step
			if tc.name == "proxy" {
				warning = "Aviso: no se pudo configurar Nginx"
			}
			if tc.step != "" && !strings.Contains(string(output), warning) {
				t.Fatalf("failed step was not reported: %s", output)
			}
			services, err := os.ReadFile(filepath.Join(root, "services"))
			if err != nil {
				t.Fatal(err)
			}
			for _, attempted := range []string{"daemon-reload", "enable amerimon.service", "restart amerimon.service", "enable --now amerimon-astdb.timer"} {
				if !strings.Contains(string(services), attempted+"\n") {
					t.Fatalf("service step was skipped: %s\n%s", attempted, services)
				}
			}
			if !strings.Contains(string(output), "proxy-attempted") {
				t.Fatalf("proxy configuration was skipped: %s", output)
			}
			env, err := os.ReadFile(filepath.Join(root, "etc/amerimon/amerimon.env"))
			if err != nil {
				t.Fatal(err)
			}
			expectedEnv := "AMI_USER=\"testuser\"\nAMI_SECRET=\"fixture-only\"\n"
			if tc.name == "existing-credentials" {
				expectedEnv = "AMI_USER=\"existing-user\"\nAMI_SECRET=\"existing-fixture\"\n"
				for path, expected := range map[string]string{
					"etc/amerimon/jwt.secret":                   "existing-jwt-fixture",
					"var/lib/amerimon/terminal-credentials.key": "existing-terminal-fixture",
				} {
					value, err := os.ReadFile(filepath.Join(root, path))
					if err != nil || string(value) != expected {
						t.Fatalf("existing credential was changed: %s: %v", path, err)
					}
				}
			}
			if string(env) != expectedEnv {
				t.Fatalf("AMI credentials were not preserved or populated correctly: %s", env)
			}
			for name, path := range map[string]string{
				"jwt": "etc/amerimon/jwt.secret", "ami": "etc/amerimon/amerimon.env",
				"allmon": "etc/amerimon/allmon.ini", "terminal": "var/lib/amerimon/terminal-credentials.key",
			} {
				if tc.name == name {
					continue
				}
				info, err := os.Stat(filepath.Join(root, path))
				if err != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("credential permissions not applied: %s: %v", path, err)
				}
			}
			if tc.name == "jwt" {
				for _, path := range []string{"etc/amerimon/allmon.ini", "var/lib/amerimon/terminal-credentials.key"} {
					if _, err := os.Stat(filepath.Join(root, path)); err != nil {
						t.Fatalf("later file configuration was skipped: %s: %v", path, err)
					}
				}
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

func TestLocalInstallerServiceStatusFailureIsNonFatal(t *testing.T) {
	data, err := os.ReadFile("../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	start := strings.Index(script, `if ! "${SUDO[@]}" systemctl --no-pager --full status amerimon.service; then`)
	if start < 0 {
		t.Fatal("service status must not abort the local installer")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nexit 3\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", "set -euo pipefail\nSUDO=()\n"+script[start:])
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "Aviso: Amerimon no está activo") {
		t.Fatalf("inactive service must warn without aborting: %v\n%s", err, output)
	}
	for _, duplicate := range []string{"AMI_USER=", "jwt.secret", "allmon.ini", "AUDIO_DIR="} {
		if strings.Contains(script, duplicate) {
			t.Fatalf("configuration %s must run only in the isolated shared hook", duplicate)
		}
	}
}
