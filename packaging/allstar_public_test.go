package packaging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllstarPublicPatchReplacesOnlyItsStanzaIdempotently(t *testing.T) {
	merger, err := filepath.Abs("merge-allstar-public.awk")
	if err != nil {
		t.Fatal(err)
	}
	patch, err := filepath.Abs("extensions-allstar-public-patch.conf")
	if err != nil {
		t.Fatal(err)
	}
	protected := "[radio-secure]\nexten => _XXXX!,1,Set(NODENUM=${CALLERID(num)})\n" +
		"same => n,GotoIf(${DB_EXISTS(allowlist/${EXTEN}/${NODENUM})}?connect)\n" +
		"[other-context]\nexten => 100,1,NoOp(Keep this)\n"
	initial := "[allstar-public]\nexten => s,1,Old implementation\n" + protected
	dir := t.TempDir()
	path := filepath.Join(dir, "extensions.conf")
	if err := os.WriteFile(path, []byte(initial), 0o640); err != nil {
		t.Fatal(err)
	}

	var first string
	for run := 0; run < 2; run++ {
		cmd := exec.Command("awk", "-v", "patch="+patch, "-f", merger, path)
		result, err := cmd.Output()
		if err != nil {
			t.Fatalf("merge failed: %v", err)
		}
		if !strings.Contains(string(result), "DB_KEYCOUNT(whitelist)") ||
			!strings.Contains(string(result), "DB_EXISTS(blacklist/${CALLSIGN_BASE})") ||
			!strings.Contains(string(result), protected) ||
			strings.Contains(string(result), "Old implementation") {
			t.Fatalf("patch output is incomplete or changed other contexts:\n%s", result)
		}
		if run == 0 {
			first = string(result)
		} else if string(result) != first {
			t.Fatal("patch is not idempotent")
		}
		if err := os.WriteFile(path, result, 0o640); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAllstarPublicPatchReportsMissingStanza(t *testing.T) {
	merger, err := filepath.Abs("merge-allstar-public.awk")
	if err != nil {
		t.Fatal(err)
	}
	patch, err := filepath.Abs("extensions-allstar-public-patch.conf")
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "extensions.conf")
	if err := os.WriteFile(config, []byte("[other-context]\nexten => 100,1,NoOp()\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("awk", "-v", "patch="+patch, "-f", merger, config)
	if output, err := cmd.Output(); err == nil {
		t.Fatalf("missing stanza should fail, output=%s", output)
	}
}

func TestAllstarPublicPatchFallsBackToGlobalCallsignAccess(t *testing.T) {
	data, err := os.ReadFile("extensions-allstar-public-patch.conf")
	if err != nil {
		t.Fatal(err)
	}
	patch := string(data)
	required := []string{
		"Set(NODENUM=${CALLERID(num)})",
		"GotoIf(${DB_EXISTS(allowlist/${NODENUM}/${CALLSIGN_BASE})}?connect)",
		"GotoIf(${DB_EXISTS(denylist/${NODENUM}/${CALLSIGN_BASE})}?node_denied:check_callsign)",
		"n(node_denied),NoOp(${CALLSIGN} is in denylist/${NODENUM}, Hangup)\n        same => n,Hangup",
		"n(check_callsign),Set(WL_COUNT=$[${DB_KEYCOUNT(whitelist)} + 0])",
		"Set(BL_COUNT=$[${DB_KEYCOUNT(blacklist)} + 0])",
		"GotoIf($[$[${WL_COUNT} = 0] & $[${BL_COUNT} = 0]]?connect)",
		"GotoIf(${DB_EXISTS(whitelist/${CALLSIGN_BASE})}?connect)",
		"GotoIf(${DB_EXISTS(blacklist/${CALLSIGN_BASE})}?blacked)",
		"Playback(rpt/connected-to&rpt/node,noanswer)",
		"Rpt(${NODENUM},X)",
	}

	for _, line := range required {
		if !strings.Contains(patch, line) {
			t.Fatalf("missing callsign access rule: %s", line)
		}
	}
	nodeRules, globalRules, found := strings.Cut(patch, "same => n(check_callsign)")
	if !found || strings.Contains(nodeRules, "DB_KEYCOUNT(allowlist/") {
		t.Fatal("node list population must not prevent fallback for an unmatched callsign")
	}
	if strings.Contains(globalRules, "allowlist/") || strings.Contains(globalRules, "denylist/") {
		t.Fatal("global callsign rules must remain separate from node-scoped rules")
	}
	if strings.Index(nodeRules, "DB_EXISTS(allowlist/") > strings.Index(nodeRules, "DB_EXISTS(denylist/") {
		t.Fatal("node allowlist must take precedence over node denylist")
	}
}

func TestAllstarPublicPatchNormalizesOnlyAccessCallsign(t *testing.T) {
	data, err := os.ReadFile("extensions-allstar-public-patch.conf")
	if err != nil {
		t.Fatal(err)
	}
	patch := string(data)
	normalization := "Set(CALLSIGN_BASE=${CUT(CALLSIGN,-,1)})\n" +
		"        same => n,Set(CALLSIGN_BASE=${CUT(CALLSIGN_BASE,/,1)})\n" +
		"        same => n,Set(CALLSIGN_BASE=${TOUPPER(${CALLSIGN_BASE})})\n" +
		"        same => n,GotoIf($[\"${CALLSIGN_BASE}\" = \"\"]?hangit)"
	if !strings.Contains(patch, normalization) {
		t.Fatal("access callsign must strip hyphen and slash suffixes, uppercase, and reject an empty base")
	}
	if strings.Index(patch, normalization) < strings.Index(patch, "Set(CALLSIGN=${RESP:5})") ||
		strings.Index(patch, normalization) > strings.Index(patch, "DB_EXISTS(") {
		t.Fatal("normalization must occur after authentication and before access lookups")
	}
	if strings.Contains(patch, "/${CALLSIGN})") {
		t.Fatal("access lookups must not use the unnormalized callsign")
	}
	if !strings.Contains(patch, "Set(CALLERID(name)=${CALLSIGN})") {
		t.Fatal("connected caller identity must retain the authenticated callsign")
	}
}
