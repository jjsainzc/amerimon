package controllers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

const (
	terminalCredentialMaxRequest = 96 * 1024
	terminalCredentialMaxKey     = 48 * 1024
)

var terminalCredentialKeyMu sync.Mutex

type terminalCredentialTarget struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	AuthMethod string `json:"authMethod"`
}

type terminalCredentialData struct {
	Password      string `json:"password,omitempty"`
	PrivateKey    string `json:"privateKey,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
}

type terminalCredentialSaveRequest struct {
	terminalCredentialTarget
	terminalCredentialData
}

// SaveTerminalCredentials stores credentials encrypted for one SSH destination.
func SaveTerminalCredentials(c echo.Context) error {
	var request terminalCredentialSaveRequest
	if err := decodeTerminalCredentialRequest(c, &request); err != nil {
		return apiError(http.StatusBadRequest, "La solicitud de credenciales SSH no es válida.")
	}
	targetKey, err := terminalCredentialTargetKey(request.terminalCredentialTarget)
	if err != nil {
		return apiError(http.StatusBadRequest, err.Error())
	}
	if err := validateTerminalCredentialData(request.terminalCredentialTarget, request.terminalCredentialData); err != nil {
		return apiError(http.StatusBadRequest, err.Error())
	}
	if config.App.DB == nil {
		return apiError(http.StatusServiceUnavailable, "La base de datos no está disponible.")
	}

	encrypted, err := encryptTerminalCredentials(targetKey, request.terminalCredentialData)
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudieron cifrar las credenciales SSH en el servidor.")
	}
	if _, err := config.App.DB.ExecContext(c.Request().Context(),
		`INSERT INTO terminal_credentials (target_key, encrypted_data, updated_at)
		 VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(target_key) DO UPDATE SET encrypted_data = excluded.encrypted_data, updated_at = CURRENT_TIMESTAMP`,
		targetKey, encrypted); err != nil {
		return apiError(http.StatusInternalServerError, "No se pudieron guardar las credenciales SSH.")
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// GetTerminalCredentials returns decrypted credentials only to the authenticated admin.
func GetTerminalCredentials(c echo.Context) error {
	var target terminalCredentialTarget
	if err := decodeTerminalCredentialRequest(c, &target); err != nil {
		return apiError(http.StatusBadRequest, "El destino SSH no es válido.")
	}
	targetKey, err := terminalCredentialTargetKey(target)
	if err != nil {
		return apiError(http.StatusBadRequest, err.Error())
	}
	if config.App.DB == nil {
		return apiError(http.StatusServiceUnavailable, "La base de datos no está disponible.")
	}

	var encrypted []byte
	err = config.App.DB.QueryRowContext(c.Request().Context(),
		`SELECT encrypted_data FROM terminal_credentials WHERE target_key = ?`, targetKey,
	).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusOK, map[string]any{"found": false})
	}
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudieron leer las credenciales SSH.")
	}
	credentials, err := decryptTerminalCredentials(targetKey, encrypted)
	if err != nil {
		return apiError(http.StatusInternalServerError, "No se pudieron descifrar las credenciales SSH; vuelve a guardarlas.")
	}
	return c.JSON(http.StatusOK, map[string]any{
		"found":         true,
		"password":      credentials.Password,
		"privateKey":    credentials.PrivateKey,
		"keyPassphrase": credentials.KeyPassphrase,
	})
}

func decodeTerminalCredentialRequest(c echo.Context, target any) error {
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, terminalCredentialMaxRequest)
	return json.NewDecoder(c.Request().Body).Decode(target)
}

func terminalCredentialTargetKey(target terminalCredentialTarget) (string, error) {
	target.Host = strings.TrimSpace(target.Host)
	target.Username = strings.TrimSpace(target.Username)
	if target.Host == "" || len(target.Host) > 253 ||
		strings.ContainsAny(target.Host, " \t\r\n/") {
		return "", errors.New("El host SSH no es válido.")
	}
	if target.Port < 1 || target.Port > 65535 {
		return "", errors.New("El puerto SSH debe estar entre 1 y 65535.")
	}
	if target.Username == "" || len(target.Username) > 64 ||
		strings.ContainsAny(target.Username, " \t\r\n") {
		return "", errors.New("El usuario SSH no es válido.")
	}
	if target.AuthMethod != "password" && target.AuthMethod != "publickey" {
		return "", errors.New("El método de autenticación SSH no es válido.")
	}
	scope := strings.ToLower(target.Host) + "\x00" + strconv.Itoa(target.Port) +
		"\x00" + target.Username + "\x00" + target.AuthMethod
	digest := sha256.Sum256([]byte(scope))
	return fmt.Sprintf("%x", digest[:]), nil
}

func validateTerminalCredentialData(target terminalCredentialTarget, credentials terminalCredentialData) error {
	switch target.AuthMethod {
	case "password":
		if credentials.Password == "" || len(credentials.Password) > 4096 ||
			credentials.PrivateKey != "" || credentials.KeyPassphrase != "" {
			return errors.New("Indica una contraseña SSH válida para guardar.")
		}
	case "publickey":
		if strings.TrimSpace(credentials.PrivateKey) == "" ||
			len(credentials.PrivateKey) > terminalCredentialMaxKey ||
			len(credentials.KeyPassphrase) > 4096 || credentials.Password != "" {
			return errors.New("Selecciona una clave privada SSH válida para guardar.")
		}
	}
	return nil
}

func terminalCredentialKeyPath() (string, error) {
	if path := os.Getenv("AMERIMON_TERMINAL_CREDENTIALS_KEY_FILE"); path != "" {
		return path, nil
	}
	if config.App.DBPath == "" {
		return "", errors.New("no está configurada la ruta de la base de datos")
	}
	return filepath.Join(filepath.Dir(config.App.DBPath), "terminal-credentials.key"), nil
}

func loadOrCreateTerminalCredentialKey() ([]byte, error) {
	terminalCredentialKeyMu.Lock()
	defer terminalCredentialKeyMu.Unlock()

	path, err := terminalCredentialKeyPath()
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			file.Close()
			os.Remove(path)
			return nil, err
		}
		if _, err := file.Write(key); err != nil {
			file.Close()
			os.Remove(path)
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrExist) {
		return nil, err
	}

	return readTerminalCredentialKey(path)
}

func loadTerminalCredentialKey() ([]byte, error) {
	terminalCredentialKeyMu.Lock()
	defer terminalCredentialKeyMu.Unlock()

	path, err := terminalCredentialKeyPath()
	if err != nil {
		return nil, err
	}
	return readTerminalCredentialKey(path)
}

func readTerminalCredentialKey(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("la clave maestra debe ser un archivo regular con permisos privados")
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("la clave maestra tiene un tamaño inválido")
	}
	return key, nil
}

func encryptTerminalCredentials(targetKey string, credentials terminalCredentialData) ([]byte, error) {
	key, err := loadOrCreateTerminalCredentialKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return append(nonce, aead.Seal(nil, nonce, plaintext, []byte(targetKey))...), nil
}

func decryptTerminalCredentials(targetKey string, encrypted []byte) (terminalCredentialData, error) {
	var credentials terminalCredentialData
	key, err := loadTerminalCredentialKey()
	if err != nil {
		return credentials, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return credentials, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return credentials, err
	}
	if len(encrypted) < aead.NonceSize() {
		return credentials, errors.New("datos cifrados incompletos")
	}
	nonce, ciphertext := encrypted[:aead.NonceSize()], encrypted[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte(targetKey))
	if err != nil {
		return credentials, err
	}
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return credentials, err
	}
	return credentials, nil
}
