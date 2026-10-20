package controllers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPrimaryRPTNode(t *testing.T) {
	config := `[nodes]
	616261 = radio@192.168.1.10/616261,NONE
	616262 = radio@127.0.0.1/616262,NONE ; primary node
	616263 = radio@192.168.1.10/616263,NONE
`
	path := filepath.Join(t.TempDir(), "rpt.conf")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	node, err := readPrimaryRPTNode(path)
	if err != nil {
		t.Fatal(err)
	}
	if node != "616262" {
		t.Fatalf("expected primary node 616262, got %q", node)
	}
}

func TestReadPrimaryRPTNodeNotConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rpt.conf")
	if err := os.WriteFile(path, []byte("[nodes]\n616263 = radio@192.168.1.10/616263,NONE\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	node, err := readPrimaryRPTNode(path)
	if err != nil {
		t.Fatal(err)
	}
	if node != "" {
		t.Fatalf("expected no primary node, got %q", node)
	}
}
