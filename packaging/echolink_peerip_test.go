package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEchoLinkPeerIPPreservesPolicyAndOtherContexts(t *testing.T) {
	policy := " same => n,GotoIf($[${DB_KEYCOUNT(allowlist/${EXTEN})} = 0]?denylist)\n" +
		" same => n,GotoIf(${DB_EXISTS(allowlist/${EXTEN}/${CALLERID(num)})}?connect)\n" +
		" same => n,Hangup()\n" +
		" same => n(denylist),GotoIf(${DB_EXISTS(denylist/${EXTEN}/${CALLERID(num)})}?:connect)\n" +
		" same => n,Hangup()\n same => n(connect),Rpt(${EXTEN})\n"
	other := "[other]\nexten => 100,1,NoOp(${CHANNEL(peerip)})\n"
	content := "[radio-secure]\nexten => _XXXX!,1,NoOp(Incoming)\n" +
		" same => n,NoOp(The Channel IP is ${CHANNEL(peerip)})\n" +
		" same => n(continue),GotoIf($[\"${CHANNEL(peerip)}\" = \"127.0.0.1\"]?connect)\n" +
		policy + other
	path := filepath.Join(t.TempDir(), "extensions.conf")
	for run := 0; run < 2; run++ {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		output, err := exec.Command("awk", "-f", "echolink-peerip.awk", path).Output()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(output), policy+other) ||
			strings.Count(string(output), "n(read_peerip)") != 1 ||
			!strings.Contains(string(output), "?read_peerip:check_ip") ||
			!strings.Contains(string(output), `GotoIf($["${CHAN_IP}" = "127.0.0.1"]?connect)`) {
			t.Fatal("patch changed policy or failed to guard peerip")
		}
		if run == 1 && string(output) != content {
			t.Fatal("patch is not idempotent")
		}
		content = string(output)
	}
}

func TestEchoLinkPeerIPRejectsMissingDuplicateOrPartialStanzas(t *testing.T) {
	for _, content := range []string{
		"[other]\n",
		"[radio-secure]\n[radio-secure]\n",
		"[radio-secure]\n same => n(continue),GotoIf($[\"${CHANNEL(peerip)}\" = \"127.0.0.1\"]?connect)\n",
	} {
		path := filepath.Join(t.TempDir(), "extensions.conf")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command("awk", "-f", "echolink-peerip.awk", path).Run(); err == nil {
			t.Fatal("unsafe patch accepted")
		}
	}
}
