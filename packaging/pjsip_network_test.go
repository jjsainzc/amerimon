package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPJSIPNetworkSelectsPrivateBind(t *testing.T) {
	for _, test := range []struct {
		name, addresses, routes, want string
		fail                          bool
	}{
		{"default private", "2: eth0 inet 10.1.2.3/24\n3: eth1 inet 192.168.1.2/24\n", "default dev eth1\n", "192.168.1.2", false},
		{"public default private vpn", "2: eth0 inet 203.0.113.2/24\n3: tun0 inet 172.31.1.2/24\n", "default dev eth0\n", "172.31.1.2", false},
		{"private no route", "3: wg0 inet 10.4.0.2/24\n", "", "10.4.0.2", false},
		{"only public", "3: eth0 inet 172.32.0.2/24\n", "default dev eth0\n", "0.0.0.0", false},
		{"loopback and link local", "1: lo inet 127.0.0.1/8\n2: eth0 inet 169.254.1.2/16\n", "", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bin, env := mockPJSIPNetwork(t, "", test.addresses, test.routes)
			cmd := exec.Command("/bin/sh", "pjsip-network.sh", "bind")
			cmd.Env = append(env, "PATH="+bin)
			output, err := cmd.CombinedOutput()
			if test.fail {
				if err == nil || !strings.Contains(string(output), "no hay una IPv4 utilizable") {
					t.Fatalf("missing address must fail explicitly: %v %s", err, output)
				}
			} else if err != nil || strings.TrimSpace(string(output)) != test.want {
				t.Fatalf("bind=%q error=%v want=%q", output, err, test.want)
			}
		})
	}
}

func TestPJSIPFirewallIsPersistentAndIdempotent(t *testing.T) {
	for _, backend := range []string{"iptables", "firewalld", "firewalld-stopped", "install"} {
		t.Run(backend, func(t *testing.T) {
			bin, env := mockPJSIPNetwork(t, backend,
				"2: eth0 inet 203.0.113.2/24\n3: tun0 inet 198.51.100.2/24\n", "default dev eth0\n")
			for run := 0; run < 2; run++ {
				cmd := exec.Command("/bin/sh", "pjsip-network.sh", "protect")
				cmd.Env = append(env, "PATH="+bin)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("firewall protection failed: %v %s", err, output)
				}
			}
			log, err := os.ReadFile(filepath.Join(bin, "log"))
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				"--dst-type LOCAL -j DROP",
				"-d 10.0.0.0/8 -j RETURN",
				"-d 172.16.0.0/12 -j RETURN",
				"-d 192.168.0.0/16 -j RETURN",
				"-p udp --dport 5060 -j AMERIMON-PJSIP",
			} {
				if !strings.Contains(string(log), required) {
					t.Fatalf("missing firewall rule %q: %s", required, log)
				}
			}
			expectedDrops := 1
			if strings.HasPrefix(backend, "firewalld") {
				expectedDrops = 2 // Una regla runtime y otra permanente.
				if !strings.Contains(string(log), "--permanent --direct --add-rule") {
					t.Fatalf("firewalld rules must persist: %s", log)
				}
				if strings.Contains(string(log), "iptables ") {
					t.Fatalf("must reuse running firewalld: %s", log)
				}
				if backend == "firewalld-stopped" &&
					strings.Count(string(log), "systemctl enable --now firewalld") != 1 {
					t.Fatalf("must activate existing firewalld only once: %s", log)
				}
			}
			if strings.Count(string(log), "--dst-type LOCAL -j DROP") != expectedDrops {
				t.Fatalf("repeated installation duplicated rules: %s", log)
			}
			if backend == "install" && strings.Count(string(log), "apt-get install -y iptables") != 1 {
				t.Fatalf("must install iptables only when no firewall exists: %s", log)
			}
			cmd := exec.Command("/bin/sh", "pjsip-network.sh", "protect")
			cmd.Env = append(env, "PATH="+bin, "NETWORK_FAIL=yes")
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("failed firewall verification must not report success: %s", output)
			}
		})
	}
	unit, err := os.ReadFile("amerimon-pjsip-firewall.service")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Before=asterisk.service", "RequiredBy=asterisk.service", "WantedBy=multi-user.target", "pjsip-network.sh protect"} {
		if !strings.Contains(string(unit), required) {
			t.Fatalf("missing persistent boot protection: %s", required)
		}
	}
}

func mockPJSIPNetwork(t *testing.T, backend, addresses, routes string) (string, []string) {
	t.Helper()
	bin := t.TempDir()
	for _, tool := range []string{"awk", "grep", "sed", "cp"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, script string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("ip", `case "$*" in
    '-o -4 addr show up') printf '%s' "$NETWORK_ADDRESSES" ;;
    '-4 route show default') printf '%s' "$NETWORK_ROUTES" ;;
    *) exit 1 ;;
esac
`)
	iptables := `[ "$NETWORK_FAIL" != yes ] || exit 1
shift 3
operation=$1
shift
case "$operation" in
    -S) [ -f "$NETWORK_BIN/chain" ] ;;
    -N) : > "$NETWORK_BIN/chain"; printf 'iptables -N %s\n' "$*" >> "$NETWORK_BIN/log" ;;
    -C) grep -Fx -- "$*" "$NETWORK_BIN/rules" >/dev/null 2>&1 ;;
    -I) chain=$1; shift 2; printf '%s\n' "$chain $*" >> "$NETWORK_BIN/rules"; printf 'iptables -I %s %s\n' "$chain" "$*" >> "$NETWORK_BIN/log" ;;
    -A) printf '%s\n' "$*" >> "$NETWORK_BIN/rules"; printf 'iptables -A %s\n' "$*" >> "$NETWORK_BIN/log" ;;
    *) exit 1 ;;
esac
`
	write("iptables-template", iptables)
	if backend == "iptables" || backend == "firewalld" {
		write("iptables", iptables)
	}
	if strings.HasPrefix(backend, "firewalld") {
		write("firewall-cmd", `[ "$NETWORK_FAIL" != yes ] || exit 1
if [ "$*" = --state ]; then
    [ "$NETWORK_BACKEND" = firewalld ] || [ -f "$NETWORK_BIN/firewalld-active" ]
    exit $?
fi
key=$(printf '%s\n' "$*" | sed 's/--query-/--add-/g')
case "$*" in
    *--query-*) grep -Fx -- "$key" "$NETWORK_BIN/rules" >/dev/null 2>&1 ;;
    *--add-*) printf '%s\n' "$key" >> "$NETWORK_BIN/rules"; printf 'firewall-cmd %s\n' "$*" >> "$NETWORK_BIN/log" ;;
    *) exit 1 ;;
esac
`)
	}
	write("apt-get", `printf 'apt-get %s\n' "$*" >> "$NETWORK_BIN/log"
cp "$NETWORK_BIN/iptables-template" "$NETWORK_BIN/iptables"
`)
	write("systemctl", `printf 'systemctl %s\n' "$*" >> "$NETWORK_BIN/log"
: > "$NETWORK_BIN/firewalld-active"
`)
	return bin, append(os.Environ(),
		"NETWORK_BIN="+bin, "NETWORK_BACKEND="+backend, "NETWORK_ADDRESSES="+addresses, "NETWORK_ROUTES="+routes)
}
