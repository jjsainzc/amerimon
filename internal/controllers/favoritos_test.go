package controllers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuscarCoincidenciasAstDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "astdb.txt")
	content := "700002|HC1LNM|Nodo secundario|Quito\n700001|HC1LNM|Nodo principal|Quito\n700003|OTRO|Sin coincidencia|Lima\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{"hc1lnm", "700001", "secundario"} {
		results, err := buscarCoincidenciasAstDB(path, query)
		if err != nil {
			t.Fatal(err)
		}
		if query == "hc1lnm" {
			if len(results) != 2 {
				t.Fatalf("got %d matches for callsign, want 2", len(results))
			}
			if !reflect.DeepEqual([]string{results[0].Nodo, results[1].Nodo}, []string{"700001", "700002"}) {
				t.Fatalf("got nodes %v, want sorted matches [700001 700002]", results)
			}
			continue
		}
		if len(results) != 1 {
			t.Fatalf("query %q: got %d matches, want 1", query, len(results))
		}
	}
}

func TestBuscarCoincidenciasAstDBReturnsReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-astdb.txt")
	if _, err := buscarCoincidenciasAstDB(path, "HC1LNM"); err == nil {
		t.Fatal("expected an error when AstDB cannot be read")
	}
}
