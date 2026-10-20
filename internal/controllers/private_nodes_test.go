package controllers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnsurePrivateNodesFileUsesConfiguredExistingFile(t *testing.T) {
	root := t.TempDir()
	configured := filepath.Join(root, "supermon", "privatenodes.txt")
	defaultPath := filepath.Join(root, "asterisk", "privatenodes.txt")
	if err := os.MkdirAll(filepath.Dir(configured), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte("1234|CALL|Descripción\n")
	if err := os.WriteFile(configured, want, 0o600); err != nil {
		t.Fatal(err)
	}

	path, err := ensurePrivateNodesFile(configured, defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if path != configured {
		t.Fatalf("expected configured path %q, got %q", configured, path)
	}
	target, err := os.Readlink(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if target != configured {
		t.Fatalf("expected default symlink target %q, got %q", configured, target)
	}
	got, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("existing file content changed: got %q", got)
	}
}

func TestEnsurePrivateNodesFileCreatesDefaultWhenConfiguredFileIsMissing(t *testing.T) {
	root := t.TempDir()
	defaultPath := filepath.Join(root, "asterisk", "privatenodes.txt")
	path, err := ensurePrivateNodesFile(filepath.Join(root, "missing", "privatenodes.txt"), defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if path != defaultPath {
		t.Fatalf("expected default path %q, got %q", defaultPath, path)
	}
	if info, err := os.Stat(defaultPath); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("expected default file to be created, stat=%v err=%v", info, err)
	}
}

func TestWritePrivateNodeLinesPreservesSymlinkAndReplacesContents(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "source", "privatenodes.txt")
	link := filepath.Join(root, "privatenodes.txt")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("1234|CALL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	want := []string{"1234|CALL|Updated"}
	if err := writePrivateNodeLines(link, want); err != nil {
		t.Fatal(err)
	}
	got, err := readPrivateNodeLines(link)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected lines %#v, got %#v", want, got)
	}
	if targetAfter, err := os.Readlink(link); err != nil || targetAfter != target {
		t.Fatalf("symlink changed: target=%q err=%v", targetAfter, err)
	}
}

func TestValidPrivateNodeLine(t *testing.T) {
	for _, line := range []string{
		"1234|CALL",
		"1234567|CALL|Descripción|Ubicación",
		"# comentario",
		"; comentario",
	} {
		if !validPrivateNodeLine(line) {
			t.Errorf("expected valid private node line %q", line)
		}
	}
	for _, line := range []string{
		"node|CALL",
		"1234",
		"1234|",
		"1234|CALL|desc|location|extra",
		" 1234|CALL",
	} {
		if validPrivateNodeLine(line) {
			t.Errorf("expected invalid private node line %q", line)
		}
	}
}

func TestPreparePrivateNodesFileDiscoversExistingNodes(t *testing.T) {
	for _, emptyDefault := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing-default", true: "empty-default"}[emptyDefault], func(t *testing.T) {
			root := t.TempDir()
			defaultPath := filepath.Join(root, "asterisk", "privatenodes.txt")
			sourceDir := filepath.Join(root, "supermon")
			source := filepath.Join(sourceDir, "privatenodes.txt")
			if err := os.MkdirAll(sourceDir, 0o755); err != nil {
				t.Fatal(err)
			}
			want := "1994|#74002|EC-ECUADOR|YSF Reflector\n1995|#74001|EC-QUITO|YSF Reflector\n"
			if err := os.WriteFile(source, []byte(want), 0o644); err != nil {
				t.Fatal(err)
			}
			if emptyDefault {
				if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(defaultPath, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			path, err := preparePrivateNodesFile(defaultPath, defaultPath, []string{sourceDir})
			if err != nil {
				t.Fatal(err)
			}
			if path != source {
				t.Fatalf("expected existing source %q, got %q", source, path)
			}
			if target, err := os.Readlink(defaultPath); err != nil || target != source {
				t.Fatalf("expected symlink to %q, got %q, %v", source, target, err)
			}
			lines, err := readPrivateNodeLines(defaultPath)
			if err != nil || len(lines) != 2 {
				t.Fatalf("expected two editable existing nodes, got %#v, %v", lines, err)
			}
			if err := writePrivateNodeLines(defaultPath, []string{"1994|#74002|Editado"}); err != nil {
				t.Fatal(err)
			}
			updated, err := readPrivateNodeLines(source)
			if err != nil || !reflect.DeepEqual(updated, []string{"1994|#74002|Editado"}) {
				t.Fatalf("editing symlink did not update source: %#v, %v", updated, err)
			}
			info, err := os.Stat(source)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o644 {
				t.Fatalf("shared source permissions changed: %v", info.Mode().Perm())
			}
		})
	}
}

func TestPreparePrivateNodesFilePreservesDefaultDataAndExplicitPaths(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "populated-default", true: "explicit-empty-file"}[explicit], func(t *testing.T) {
			root := t.TempDir()
			defaultPath := filepath.Join(root, "privatenodes.txt")
			configured := defaultPath
			want := "1234|KEEP\n"
			if explicit {
				configured = filepath.Join(root, "configured.txt")
				want = ""
			}
			if err := os.WriteFile(configured, []byte(want), 0o600); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, "supermon")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "privatenodes.txt"), []byte("1994|OTHER\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			path, err := preparePrivateNodesFile(configured, defaultPath, []string{dir})
			if err != nil || path != configured {
				t.Fatalf("configured file not preserved: %q, %v", path, err)
			}
			got, err := os.ReadFile(configured)
			if err != nil || string(got) != want {
				t.Fatalf("existing contents changed: %q, %v", got, err)
			}
		})
	}
}
