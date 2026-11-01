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
	protected := "[other-context]\nexten => 100,1,NoOp(Keep this)\n"
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
			!strings.Contains(string(result), "DB_EXISTS(blacklist/${CALLSIGN})") ||
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
