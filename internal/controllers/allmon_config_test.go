package controllers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAllmonInstancesPreservesTrackingDisabledNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allmon.ini")
	content := "[123456]\nsystem=Nodes\nhost=192.0.2.1\nuser=admin\npasswd=secret\namerimon_tracking=disabled\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	instances, err := readAllmonInstances(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 {
		t.Fatalf("got %d nodes, want the disabled node to remain in the config list", len(instances))
	}
	if !instances[0].TrackingDisabled {
		t.Fatal("node tracking should be disabled")
	}
}

func TestFilterDisabledAllmonInstances(t *testing.T) {
	instances := []allmonInstance{
		{Node: "123456"},
		{Node: "123457", TrackingDisabled: true},
	}

	active := filterDisabledAllmonInstances(instances)
	if len(active) != 1 || active[0].Node != "123456" {
		t.Fatalf("got active nodes %+v, want only node 123456", active)
	}
}

func TestRewriteAllmonNodesTogglesTrackingWithoutChangingCustomSettings(t *testing.T) {
	content := "[123456]\nsystem=Nodes\nhost=192.0.2.1\nuser=admin\npasswd=secret\ncustom_setting=keep\n"
	existing := map[string]bool{"123456": true}

	disabled := rewriteAllmonNodes(content, []allmonNodeIn{{
		Node: "123456", Host: "192.0.2.1", User: "admin", Passwd: "secret", TrackingDisabled: true,
	}}, existing)
	if !strings.Contains(disabled, "amerimon_tracking=disabled\n") {
		t.Fatal("disabled tracking setting was not written")
	}
	if count := strings.Count(disabled, "amerimon_tracking=disabled\n"); count != 1 {
		t.Fatalf("tracking setting written %d times, want once", count)
	}
	if !strings.Contains(disabled, "custom_setting=keep\n") {
		t.Fatal("unrelated custom setting was not preserved")
	}

	enabled := rewriteAllmonNodes(disabled, []allmonNodeIn{{
		Node: "123456", Host: "192.0.2.1", User: "admin", Passwd: "secret",
	}}, existing)
	if strings.Contains(enabled, "amerimon_tracking=disabled") {
		t.Fatal("disabled tracking setting remained after re-enabling the node")
	}
	if !strings.Contains(enabled, "custom_setting=keep\n") {
		t.Fatal("unrelated custom setting was not preserved after re-enabling")
	}
}
