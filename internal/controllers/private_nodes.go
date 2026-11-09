package controllers

// Este archivo implementa la lectura, validación y actualización del catálogo de nodos privados.

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

const defaultPrivateNodesPath = "/var/lib/asterisk/privatenodes.txt"

const (
	maxPrivateNodesLines = 10000
	maxPrivateNodesBytes = 1024 * 1024
	maxPrivateNodeLine   = 512
)

type privateNodesRequest struct {
	Lines []string `json:"lines"`
}

// EnsurePrivateNodesFile prepara la ruta predeterminada sin reemplazar un archivo configurado existente.
func EnsurePrivateNodesFile() error {
	_, err := preparePrivateNodesFile(config.App.Sources.PrivateNodesPath, defaultPrivateNodesPath, allmonSearchDirs)
	return err
}

// preparePrivateNodesFile reutiliza archivos de Allmon/Supermon si la ruta predeterminada
// no existe o está vacía; nunca reemplaza datos existentes ni una ruta explícita distinta.
func preparePrivateNodesFile(configuredPath, defaultPath string, searchDirs []string) (string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath != "" && filepath.Clean(configuredPath) != filepath.Clean(defaultPath) {
		return ensurePrivateNodesFile(configuredPath, defaultPath)
	}

	info, err := os.Lstat(defaultPath)
	switch {
	case err == nil:
		if !info.Mode().IsRegular() || info.Size() > 0 {
			return ensurePrivateNodesFile(configuredPath, defaultPath)
		}
	case !errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("comprobar %s: %w", defaultPath, err)
	}

	for _, dir := range searchDirs {
		candidate := filepath.Join(dir, "privatenodes.txt")
		if filepath.Clean(candidate) == filepath.Clean(defaultPath) {
			continue
		}
		exists, err := regularFileExists(candidate)
		if err != nil {
			return "", fmt.Errorf("comprobar %s: %w", candidate, err)
		}
		if !exists {
			continue
		}
		candidateInfo, err := os.Stat(candidate)
		if err != nil {
			return "", fmt.Errorf("comprobar %s: %w", candidate, err)
		}
		if candidateInfo.Size() == 0 {
			continue
		}
		// Solo se retira el archivo vacío creado anteriormente, nunca un archivo con datos.
		if info != nil {
			current, err := os.Lstat(defaultPath)
			if err != nil {
				return "", fmt.Errorf("comprobar %s: %w", defaultPath, err)
			}
			if !os.SameFile(info, current) || !current.Mode().IsRegular() || current.Size() != 0 {
				return "", fmt.Errorf("%s cambió durante la búsqueda de nodos privados", defaultPath)
			}
			if err := os.Remove(defaultPath); err != nil {
				return "", fmt.Errorf("reemplazar archivo vacío %s: %w", defaultPath, err)
			}
		}
		path, err := ensurePrivateNodesFile(candidate, defaultPath)
		if err == nil {
			log.Printf("private nodes: reutilizando %s mediante %s", candidate, defaultPath)
		}
		return path, err
	}
	return ensurePrivateNodesFile(configuredPath, defaultPath)
}

// ensurePrivateNodesFile prepara el archivo predeterminado o su enlace a la ruta configurada y devuelve la ruta efectiva o un error de sistema de archivos.
func ensurePrivateNodesFile(configuredPath, defaultPath string) (string, error) {
	configuredPath = strings.TrimSpace(configuredPath)
	if configuredPath == "" {
		configuredPath = defaultPath
	}
	configuredExists, err := regularFileExists(configuredPath)
	if err != nil {
		return "", fmt.Errorf("comprobar %s: %w", configuredPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(defaultPath), 0o755); err != nil {
		return "", fmt.Errorf("crear directorio %s: %w", filepath.Dir(defaultPath), err)
	}

	defaultExists, err := regularFileExists(defaultPath)
	if err != nil {
		return "", fmt.Errorf("comprobar %s: %w", defaultPath, err)
	}

	if configuredExists && filepath.Clean(configuredPath) != filepath.Clean(defaultPath) && !defaultExists {
		target, err := filepath.Abs(configuredPath)
		if err != nil {
			return "", fmt.Errorf("resolver %s: %w", configuredPath, err)
		}
		if err := os.Symlink(target, defaultPath); err != nil && !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("crear enlace %s -> %s: %w", defaultPath, target, err)
		}
		defaultExists, err = regularFileExists(defaultPath)
		if err != nil {
			return "", fmt.Errorf("comprobar %s: %w", defaultPath, err)
		}
	}

	if !configuredExists && !defaultExists {
		file, err := os.OpenFile(defaultPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("crear %s: %w", defaultPath, err)
		}
		if file != nil {
			if err := file.Close(); err != nil {
				return "", fmt.Errorf("cerrar %s: %w", defaultPath, err)
			}
		}
	}

	if configuredExists {
		return configuredPath, nil
	}
	if _, err := regularFileExists(defaultPath); err != nil {
		return "", fmt.Errorf("comprobar %s: %w", defaultPath, err)
	}
	return defaultPath, nil
}

// regularFileExists comprueba si path apunta a un archivo regular y devuelve errores para rutas inválidas o enlaces rotos.
func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if _, linkErr := os.Lstat(path); linkErr == nil {
			return false, fmt.Errorf("el enlace simbólico %s no tiene destino", path)
		} else if !errors.Is(linkErr, os.ErrNotExist) {
			return false, linkErr
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s no es un archivo regular", path)
	}
	return true, nil
}

// privateNodesFilePath devuelve la ruta configurada si existe; en caso contrario, usa la ruta predeterminada.
func privateNodesFilePath() string {
	path := strings.TrimSpace(config.App.Sources.PrivateNodesPath)
	if path == "" {
		return defaultPrivateNodesPath
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return defaultPrivateNodesPath
}

// GetPrivateNodesConfig devuelve a los administradores las líneas editables de nodos privados.
func GetPrivateNodesConfig(c echo.Context) error {
	path, err := preparePrivateNodesFile(config.App.Sources.PrivateNodesPath, defaultPrivateNodesPath, allmonSearchDirs)
	if err != nil {
		log.Printf("private nodes: no se pudo preparar el archivo: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudo preparar el archivo de nodos privados: "+err.Error())
	}
	lines, err := readPrivateNodeLines(path)
	if err != nil {
		log.Printf("private nodes: no se pudo leer %s: %v", path, err)
		return apiError(http.StatusInternalServerError, "No se pudo leer el archivo de nodos privados")
	}
	return c.JSON(http.StatusOK, echo.Map{
		"status":   "ok",
		"path":     path,
		"writable": privateNodesWritable(path),
		"lines":    lines,
	})
}

// SavePrivateNodesConfig valida y guarda atómicamente las líneas de nodos privados.
func SavePrivateNodesConfig(c echo.Context) error {
	var request privateNodesRequest
	if err := c.Bind(&request); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	if request.Lines == nil {
		return apiError(http.StatusBadRequest, "Falta la lista de líneas")
	}
	if len(request.Lines) > maxPrivateNodesLines {
		return apiError(http.StatusBadRequest, "Demasiadas líneas")
	}

	lines := make([]string, 0, len(request.Lines))
	seen := make(map[string]struct{}, len(request.Lines))
	totalBytes := 0
	for index, line := range request.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > maxPrivateNodeLine || strings.ContainsAny(line, "\r\n\x00") {
			return apiError(http.StatusBadRequest, fmt.Sprintf("La línea %d supera el límite permitido o contiene caracteres inválidos", index+1))
		}
		if !validPrivateNodeLine(line) {
			return apiError(http.StatusBadRequest, fmt.Sprintf("La línea %d debe tener formato nodo|indicativo[|descripción[|ubicación]]", index+1))
		}
		fields := strings.Split(line, "|")
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			lines = append(lines, line)
			totalBytes += len(line) + 1
			continue
		}
		if _, exists := seen[fields[0]]; exists {
			return apiError(http.StatusBadRequest, "El nodo "+fields[0]+" aparece más de una vez")
		}
		seen[fields[0]] = struct{}{}
		lines = append(lines, line)
		totalBytes += len(line) + 1
		if totalBytes > maxPrivateNodesBytes {
			return apiError(http.StatusBadRequest, "El archivo supera el tamaño máximo permitido")
		}
	}
	if totalBytes > maxPrivateNodesBytes {
		return apiError(http.StatusBadRequest, "El archivo supera el tamaño máximo permitido")
	}

	path, err := preparePrivateNodesFile(config.App.Sources.PrivateNodesPath, defaultPrivateNodesPath, allmonSearchDirs)
	if err != nil {
		log.Printf("private nodes: no se pudo preparar el archivo: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudo preparar el archivo de nodos privados")
	}
	if err := writePrivateNodeLines(path, lines); err != nil {
		log.Printf("private nodes: no se pudo guardar %s: %v", path, err)
		return apiError(http.StatusInternalServerError, "No se pudo guardar el archivo de nodos privados")
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "ok", "path": path})
}

// validPrivateNodeLine acepta comentarios o registros de nodo con formato y campos válidos.
func validPrivateNodeLine(line string) bool {
	if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return true
	}
	fields := strings.Split(line, "|")
	if len(fields) < 2 || len(fields) > 4 || len(fields[0]) > 7 ||
		!isNumericNode(fields[0]) || strings.TrimSpace(fields[1]) == "" {
		return false
	}
	for _, field := range fields {
		if strings.TrimSpace(field) != field {
			return false
		}
	}
	return true
}

// privateNodesWritable indica si puede abrirse el archivo para escritura o si aún no existe.
func privateNodesWritable(path string) bool {
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		_ = file.Close()
		return true
	}
	return os.IsNotExist(err)
}

// readPrivateNodeLines devuelve las líneas no vacías del archivo o un error de lectura o límite excedido.
func readPrivateNodeLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	lines := make([]string, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxPrivateNodeLine*2)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
			if len(lines) > maxPrivateNodesLines {
				return nil, fmt.Errorf("demasiadas líneas en %s", path)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// writePrivateNodeLines reemplaza el contenido atómicamente conservando permisos y grupo
// del destino; conserva también su propietario si el proceso tiene privilegios para hacerlo.
func writePrivateNodeLines(path string, lines []string) error {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(resolvedPath), ".privatenodes-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if owner, ok := info.Sys().(*syscall.Stat_t); ok {
		uid := int(owner.Uid)
		if os.Geteuid() != 0 && os.Geteuid() != uid {
			uid = os.Geteuid()
			log.Printf("private nodes: guardado sin privilegios en %s conservará el grupo %d, pero usará el propietario %d en lugar de %d", resolvedPath, owner.Gid, uid, owner.Uid)
		}
		if err := temp.Chown(uid, int(owner.Gid)); err != nil {
			temp.Close()
			return err
		}
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		temp.Close()
		return err
	}
	writer := bufio.NewWriter(temp)
	for _, line := range lines {
		if _, err := writer.WriteString(line + "\n"); err != nil {
			temp.Close()
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, resolvedPath); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(resolvedPath))
}

// syncDirectory sincroniza los cambios de metadatos del directorio y devuelve cualquier error de sistema de archivos.
func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return err
	}
	return nil
}
