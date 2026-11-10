package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, keeps CGO disabled for cross builds
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open abre (o crea, si es necesario) la base de datos SQLite situada en la ruta indicada y aplica las migraciones pendientes.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	d, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	if err := migrate(d, path); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

// migrate aplica las migraciones pendientes en orden, cada una en su transacción.
// Las ya aplicadas (schema_migrations) no se repiten, así que los datos del cliente se conservan:
// para cambiar el esquema se agrega un archivo nuevo (ALTER TABLE / CREATE TABLE IF NOT EXISTS)
// y nunca se edita uno ya publicado. Antes de migrar una base existente se guarda una copia.
func migrate(d *sql.DB, path string) error {
	if _, err := d.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	applied := map[string]bool{}
	rows, err := d.Query(`SELECT name FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return err
		}
		applied[n] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	pending := 0
	for _, name := range names {
		if !applied[name] {
			pending++
		}
	}
	if pending > 0 && len(applied) > 0 {
		if err := backupBeforeMigrate(d, path); err != nil {
			return fmt.Errorf("respaldo previo a la migración: %w", err)
		}
	}

	for _, name := range names {
		if applied[name] {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := d.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// backupBeforeMigrate copia la base a <path>.bak-<fecha> y conserva solo las 3 últimas.
func backupBeforeMigrate(d *sql.DB, path string) error {
	dest := fmt.Sprintf("%s.bak-%s", path, time.Now().Format("20060102-150405"))
	_ = os.Remove(dest)
	if _, err := d.Exec("VACUUM INTO '" + strings.ReplaceAll(dest, "'", "''") + "'"); err != nil {
		return err
	}
	_ = os.Chmod(dest, 0o600)

	old, _ := filepath.Glob(path + ".bak-*")
	sort.Strings(old)
	for len(old) > 3 {
		_ = os.Remove(old[0])
		old = old[1:]
	}
	return nil
}
