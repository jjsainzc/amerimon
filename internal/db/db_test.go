package db

import (
	"path/filepath"
	"testing"
)

// Simula una actualización: la base ya tiene datos y falta registrar una migración.
func TestMigrateKeepsDataAndBacksUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")

	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO favoritos (numero, descripcion) VALUES ('448511', 'HI3ACS')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`DELETE FROM schema_migrations WHERE name = '0005_favoritos.sql'`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	var n int
	if err := d.QueryRow(`SELECT COUNT(1) FROM favoritos WHERE numero = '448511'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("se perdieron datos del cliente: n=%d err=%v", n, err)
	}
	if err := d.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE name = '0005_favoritos.sql'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("la migración no se volvió a registrar: n=%d err=%v", n, err)
	}
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 1 {
		t.Fatalf("se esperaba 1 respaldo, hay %d", len(backups))
	}
}
