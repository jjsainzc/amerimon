package controllers

// Este archivo contiene pruebas para el almacenamiento seguro de credenciales de terminal por destino.

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

func TestTerminalCredentialHandlersEncryptAndRetrievePerTarget(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "amerimon.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	previousDB, previousPath := config.App.DB, config.App.DBPath
	config.App.DB, config.App.DBPath = database, filepath.Join(dir, "amerimon.db")
	defer func() {
		config.App.DB, config.App.DBPath = previousDB, previousPath
	}()
	t.Setenv("AMERIMON_TERMINAL_CREDENTIALS_KEY_FILE", filepath.Join(dir, "credentials.key"))

	target := terminalCredentialTarget{
		Host:       "Server.Internal",
		Port:       22,
		Username:   "operator",
		AuthMethod: "password",
	}
	savePayload, err := json.Marshal(terminalCredentialSaveRequest{
		terminalCredentialTarget: target,
		terminalCredentialData: terminalCredentialData{
			Password: "test-password-not-plaintext",
		},
	})
	if err != nil {
		t.Fatalf("encode save request: %v", err)
	}
	recorder := httptest.NewRecorder()
	context := echo.New().NewContext(
		httptest.NewRequest(http.MethodPost, "/mon/terminal/credentials/save", bytes.NewReader(savePayload)),
		recorder,
	)
	if err := SaveTerminalCredentials(context); err != nil {
		t.Fatalf("save credentials: %v", err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("save status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}

	var encrypted []byte
	key, err := terminalCredentialTargetKey(target)
	if err != nil {
		t.Fatalf("create credential target key: %v", err)
	}
	if err := database.QueryRow(`SELECT encrypted_data FROM terminal_credentials WHERE target_key = ?`, key).Scan(&encrypted); err != nil {
		t.Fatalf("read stored ciphertext: %v", err)
	}
	if bytes.Contains(encrypted, []byte("test-password-not-plaintext")) {
		t.Fatal("database contains the password in plaintext")
	}

	getPayload, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("encode get request: %v", err)
	}
	recorder = httptest.NewRecorder()
	context = echo.New().NewContext(
		httptest.NewRequest(http.MethodPost, "/mon/terminal/credentials/get", bytes.NewReader(getPayload)),
		recorder,
	)
	if err := GetTerminalCredentials(context); err != nil {
		t.Fatalf("get credentials: %v", err)
	}
	var response struct {
		Found    bool   `json:"found"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode credential response: %v", err)
	}
	if !response.Found || response.Password != "test-password-not-plaintext" {
		t.Fatalf("unexpected credential response: found=%v password=%q", response.Found, response.Password)
	}

	target.Port++
	otherKey, err := terminalCredentialTargetKey(target)
	if err != nil {
		t.Fatalf("create other credential target key: %v", err)
	}
	if otherKey == key {
		t.Fatal("credentials for a different port share the same target key")
	}
}

func TestTerminalCredentialSaveRequiresConfirmationToReplaceExistingCredentials(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "amerimon.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	previousDB, previousPath := config.App.DB, config.App.DBPath
	config.App.DB, config.App.DBPath = database, filepath.Join(dir, "amerimon.db")
	defer func() {
		config.App.DB, config.App.DBPath = previousDB, previousPath
	}()
	t.Setenv("AMERIMON_TERMINAL_CREDENTIALS_KEY_FILE", filepath.Join(dir, "credentials.key"))

	target := terminalCredentialTarget{
		Host:       "server.internal",
		Port:       22,
		Username:   "operator",
		AuthMethod: "password",
	}
	save := func(payload string) error {
		recorder := httptest.NewRecorder()
		context := echo.New().NewContext(
			httptest.NewRequest(http.MethodPost, "/mon/terminal/credentials/save", strings.NewReader(payload)),
			recorder,
		)
		return SaveTerminalCredentials(context)
	}

	if err := save(`{"host":"server.internal","port":22,"username":"operator","authMethod":"password","password":"old-password"}`); err != nil {
		t.Fatalf("save initial credentials: %v", err)
	}
	err = save(`{"host":"server.internal","port":22,"username":"operator","authMethod":"password","password":"new-password"}`)
	var httpErr *echo.HTTPError
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("unconfirmed replacement error = %#v, want terminal_credentials_exist conflict", err)
	}
	message, ok := httpErr.Message.(map[string]any)
	if !ok || message["code"] != "terminal_credentials_exist" {
		t.Fatalf("unconfirmed replacement response = %#v, want terminal_credentials_exist", httpErr.Message)
	}

	targetKey, err := terminalCredentialTargetKey(target)
	if err != nil {
		t.Fatalf("create credential target key: %v", err)
	}
	var encrypted []byte
	if err := database.QueryRow(`SELECT encrypted_data FROM terminal_credentials WHERE target_key = ?`, targetKey).Scan(&encrypted); err != nil {
		t.Fatalf("read stored credentials: %v", err)
	}
	stored, err := decryptTerminalCredentials(targetKey, encrypted)
	if err != nil {
		t.Fatalf("decrypt stored credentials: %v", err)
	}
	if stored.Password != "old-password" {
		t.Fatalf("unconfirmed replacement changed stored credentials to %q", stored.Password)
	}

	if err := save(`{"host":"server.internal","port":22,"username":"operator","authMethod":"password","password":"new-password","replaceExisting":true}`); err != nil {
		t.Fatalf("save confirmed replacement: %v", err)
	}
	if err := database.QueryRow(`SELECT encrypted_data FROM terminal_credentials WHERE target_key = ?`, targetKey).Scan(&encrypted); err != nil {
		t.Fatalf("read replaced credentials: %v", err)
	}
	stored, err = decryptTerminalCredentials(targetKey, encrypted)
	if err != nil {
		t.Fatalf("decrypt replaced credentials: %v", err)
	}
	if stored.Password != "new-password" {
		t.Fatalf("confirmed replacement stored password = %q, want new-password", stored.Password)
	}
}

func TestTerminalCredentialHandlersSaveAndRetrievePublicKey(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "amerimon.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	previousDB, previousPath := config.App.DB, config.App.DBPath
	config.App.DB, config.App.DBPath = database, filepath.Join(dir, "amerimon.db")
	defer func() {
		config.App.DB, config.App.DBPath = previousDB, previousPath
	}()
	t.Setenv("AMERIMON_TERMINAL_CREDENTIALS_KEY_FILE", filepath.Join(dir, "credentials.key"))

	payload := `{"host":"server.internal","port":2222,"username":"operator","authMethod":"publickey","privateKey":"private-key-material","keyPassphrase":"private-key-passphrase"}`
	recorder := httptest.NewRecorder()
	context := echo.New().NewContext(
		httptest.NewRequest(http.MethodPost, "/mon/terminal/credentials/save", strings.NewReader(payload)),
		recorder,
	)
	if err := SaveTerminalCredentials(context); err != nil {
		t.Fatalf("save public-key credentials: %v", err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("save status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}

	getPayload := `{"host":"server.internal","port":2222,"username":"operator","authMethod":"publickey"}`
	recorder = httptest.NewRecorder()
	context = echo.New().NewContext(
		httptest.NewRequest(http.MethodPost, "/mon/terminal/credentials/get", strings.NewReader(getPayload)),
		recorder,
	)
	if err := GetTerminalCredentials(context); err != nil {
		t.Fatalf("get public-key credentials: %v", err)
	}
	var response struct {
		Found         bool   `json:"found"`
		PrivateKey    string `json:"privateKey"`
		KeyPassphrase string `json:"keyPassphrase"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode public-key credential response: %v", err)
	}
	if !response.Found || response.PrivateKey != "private-key-material" ||
		response.KeyPassphrase != "private-key-passphrase" {
		t.Fatalf("public-key credentials were not saved and restored: found=%v", response.Found)
	}

	var encrypted []byte
	target := terminalCredentialTarget{
		Host:       "server.internal",
		Port:       2222,
		Username:   "operator",
		AuthMethod: "publickey",
	}
	targetKey, err := terminalCredentialTargetKey(target)
	if err != nil {
		t.Fatalf("create public-key target key: %v", err)
	}
	if err := database.QueryRow(
		`SELECT encrypted_data FROM terminal_credentials WHERE target_key = ?`, targetKey,
	).Scan(&encrypted); err != nil {
		t.Fatalf("read stored public-key ciphertext: %v", err)
	}
	if bytes.Contains(encrypted, []byte("private-key-material")) ||
		bytes.Contains(encrypted, []byte("private-key-passphrase")) {
		t.Fatal("database contains public-key credentials in plaintext")
	}
}

func TestClearAdminPasswordAlsoClearsTerminalCredentials(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "amerimon.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()
	if _, err := database.Exec(`INSERT INTO admin (id, password_hash) VALUES (1, 'hash')`); err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO terminal_credentials (target_key, encrypted_data) VALUES ('target', x'0102')`); err != nil {
		t.Fatalf("insert terminal credential: %v", err)
	}
	if err := ClearAdminPassword(database); err != nil {
		t.Fatalf("clear admin password: %v", err)
	}
	for _, table := range []string{"admin", "terminal_credentials"} {
		var count int
		if err := database.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count rows in %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s still has %d records after password reset", table, count)
		}
	}
}

func TestTerminalCredentialKeyFileHasPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.key")
	t.Setenv("AMERIMON_TERMINAL_CREDENTIALS_KEY_FILE", path)
	key, err := loadOrCreateTerminalCredentialKey()
	if err != nil {
		t.Fatalf("create credential key: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d, want 32", len(key))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat credential key: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("key permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestTerminalCredentialTargetScopeIncludesMethodAndUsername(t *testing.T) {
	target := terminalCredentialTarget{Host: "server", Port: 22, Username: "one", AuthMethod: "password"}
	first, err := terminalCredentialTargetKey(target)
	if err != nil {
		t.Fatal(err)
	}
	target.Username = "two"
	second, err := terminalCredentialTargetKey(target)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("different usernames share a credential scope")
	}
	target.Username = "one"
	target.AuthMethod = "publickey"
	third, err := terminalCredentialTargetKey(target)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("different authentication methods share a credential scope")
	}
}

func TestTerminalCredentialEncryptionUsesAuthenticatedCiphertext(t *testing.T) {
	t.Setenv("AMERIMON_TERMINAL_CREDENTIALS_KEY_FILE", filepath.Join(t.TempDir(), "credentials.key"))
	credentials := terminalCredentialData{PrivateKey: "private-key-material", KeyPassphrase: "passphrase"}
	ciphertext, err := encryptTerminalCredentials("scope", credentials)
	if err != nil {
		t.Fatalf("encrypt credentials: %v", err)
	}
	if strings.Contains(string(ciphertext), credentials.PrivateKey) {
		t.Fatal("private key is visible in ciphertext")
	}
	decoded, err := decryptTerminalCredentials("scope", ciphertext)
	if err != nil {
		t.Fatalf("decrypt credentials: %v", err)
	}
	if decoded != credentials {
		t.Fatalf("decrypted credentials = %#v, want %#v", decoded, credentials)
	}
	if _, err := decryptTerminalCredentials("other-scope", ciphertext); err == nil {
		t.Fatal("ciphertext was accepted for a different target")
	}
}
