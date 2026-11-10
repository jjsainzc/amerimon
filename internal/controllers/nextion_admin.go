package controllers

// Este archivo implementa la lectura y administración de la configuración Nextion.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

// nextionBaudRates, nextionPortName y nextionModelName son expresiones regulares y mapas utilizados para validar y enumerar los puertos y modelos Nextion disponibles.
var (
	nextionBaudRates = map[int]uint32{
		9600: syscall.B9600, 19200: syscall.B19200, 38400: syscall.B38400,
		57600: syscall.B57600, 115200: syscall.B115200,
	}
	nextionPortName = regexp.MustCompile(`^/dev/tty(?:USB|ACM)[0-9]+$`)
	// El firmware es único para todos los idiomas; los nombres con sufijo -idioma quedan excluidos.
	nextionModelName = regexp.MustCompile(`^AMERINET_Nextion_([A-Za-z0-9._]+)$`)
)

// nextionSettings representa la configuración de un dispositivo Nextion, incluyendo el puerto, la velocidad de transmisión, la interfaz de red, el nodo favorito y si se deben enviar etiquetas internacionalizadas.
type nextionSettings struct {
	Enabled      bool   `json:"enabled"`
	Port         string `json:"port"`
	BaudRate     int    `json:"baud_rate"`
	Interface    string `json:"interface"`
	FavoriteNode string `json:"favorite_node"`
	// SendLabels envía las etiquetas internacionalizadas; desactívalo para
	// conservar los textos de un diseño HMI propio y recibir solo los valores.
	SendLabels bool `json:"send_labels"`
}

// nextionNetworkInterface representa una interfaz de red disponible para la configuración Nextion, incluyendo su nombre, etiqueta descriptiva y dirección IPv4.
type nextionNetworkInterface struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	IPv4  string `json:"ipv4,omitempty"`
}

// nextionPort representa un puerto serie disponible para la configuración Nextion, incluyendo información del fabricante, producto, IDs y número de serie.
type nextionPort struct {
	Path         string `json:"path"`
	Label        string `json:"label"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Product      string `json:"product,omitempty"`
	VendorID     string `json:"vendor_id,omitempty"`
	ProductID    string `json:"product_id,omitempty"`
	Serial       string `json:"serial,omitempty"`
}

// discoverNextionInterfaces busca y devuelve las interfaces de red disponibles que están activas y no son de bucle invertido, incluyendo su dirección IPv4 y una etiqueta descriptiva.
func discoverNextionInterfaces() []nextionNetworkInterface {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("nextion: no se pudieron enumerar interfaces de red: %v", err)
		return []nextionNetworkInterface{}
	}
	result := make([]nextionNetworkInterface, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			log.Printf("nextion: no se pudieron consultar direcciones de %s: %v", iface.Name, err)
		}
		ipv4 := nextionInterfaceIPv4(addresses)
		label := iface.Name
		if ipv4 != "" {
			label += " — " + ipv4
		}
		if iface.HardwareAddr != nil {
			label += " — " + iface.HardwareAddr.String()
		}
		result = append(result, nextionNetworkInterface{Name: iface.Name, Label: label, IPv4: ipv4})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// nextionInterfaceIPv4 obtiene la primera dirección IPv4 global unicast de la lista de direcciones proporcionada, ignorando las direcciones de bucle invertido y locales, y evita duplicados.
func nextionInterfaceIPv4(addresses []net.Addr) string {
	var values []string
	seen := make(map[string]bool)
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || ip.To4() == nil || !ip.IsGlobalUnicast() ||
			ip.IsLoopback() || ip.IsLinkLocalUnicast() || seen[ip.String()] {
			continue
		}
		seen[ip.String()] = true
		values = append(values, ip.String())
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

// hasNetworkInterface verifica si la interfaz de red especificada existe y está activa, ignorando las interfaces de bucle invertido.
func hasNetworkInterface(name string) bool {
	iface, err := net.InterfaceByName(name)
	return err == nil && iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0
}

// nextionModel representa un modelo de pantalla Nextion disponible, incluyendo su nombre, modelo, URL de vista previa y si el código fuente está disponible.
type nextionModel struct {
	Name            string `json:"name"`
	Model           string `json:"model"`
	PreviewURL      string `json:"preview_url"`
	SourceAvailable bool   `json:"source_available"`
}

// nextionRuntimeStatus representa el estado de ejecución del servicio Nextion, incluyendo si está en ejecución, el puerto, el nodo local y cualquier error.
type nextionRuntimeStatus struct {
	Running   bool   `json:"running"`
	Port      string `json:"port"`
	LocalNode string `json:"local_node,omitempty"`
	Error     string `json:"error,omitempty"`
}

// nextionSettingsRequest representa la estructura de entrada para actualizar la configuración de un dispositivo Nextion.
type nextionSettingsRequest struct {
	Enabled      bool   `json:"enabled"`
	Port         string `json:"port"`
	BaudRate     int    `json:"baud_rate"`
	Interface    string `json:"interface"`
	FavoriteNode string `json:"favorite_node"`
	SendLabels   *bool  `json:"send_labels"`
}

// nextionPortTestRequest representa la estructura de entrada para probar un puerto serie de un dispositivo Nextion.
type nextionPortTestRequest struct {
	Port     string `json:"port"`
	BaudRate int    `json:"baud_rate"`
}

// nextionService representa el servicio Nextion en ejecución, incluyendo su estado, puerto, nodo local, último error, revisión, función de cancelación y canales de notificación.
type nextionService struct {
	mu       sync.Mutex
	running  bool
	port     string
	local    string
	lastErr  string
	revision string
	cancel   context.CancelFunc
	done     chan struct{}
	notify   chan struct{}
}

// nextionRuntime es la instancia global del servicio Nextion, utilizada para rastrear su estado y notificaciones.
var nextionRuntime = &nextionService{notify: make(chan struct{}, 1)}

// readNextionSettings lee la configuración de Nextion desde la base de datos, proporcionando valores predeterminados y manejando errores de lectura.
func readNextionSettings(ctx context.Context) (nextionSettings, error) {
	settings := nextionSettings{BaudRate: 9600, SendLabels: true}
	if config.App.DB == nil {
		return settings, errors.New("base de datos no disponible")
	}
	values := map[string]*string{
		"nextion_enabled": nil, "nextion_port": &settings.Port,
		"nextion_baud_rate": nil, "nextion_interface": &settings.Interface,
		"nextion_favorite_node": &settings.FavoriteNode,
		"nextion_send_labels":   nil,
	}
	for key, target := range values {
		var value string
		err := config.App.DB.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return settings, err
		}
		if target != nil {
			*target = value
			continue
		}
		switch key {
		case "nextion_enabled":
			settings.Enabled = value == "1"
		case "nextion_send_labels":
			settings.SendLabels = value != "0"
		case "nextion_baud_rate":
			rate, err := strconv.Atoi(value)
			if err != nil {
				return settings, fmt.Errorf("velocidad serie guardada inválida: %w", err)
			}
			settings.BaudRate = rate
		}
	}
	return settings, nil
}

// validateNextionSettings valida la configuración de Nextion, asegurándose de que los valores sean correctos y que el nodo favorito sea distinto del nodo local.
func validateNextionSettings(settings nextionSettings) error {
	settings.Port = strings.TrimSpace(settings.Port)
	settings.FavoriteNode = strings.TrimSpace(settings.FavoriteNode)
	if !settings.Enabled {
		return nil
	}
	if settings.Port != "" && !nextionPortName.MatchString(settings.Port) {
		return errors.New("el puerto debe ser un dispositivo /dev/ttyUSBx o /dev/ttyACMx")
	}
	if settings.Interface != "" && !hasNetworkInterface(settings.Interface) {
		return errors.New("la interfaz de red seleccionada no está disponible")
	}
	if settings.BaudRate != 0 {
		if _, ok := nextionBaudRates[settings.BaudRate]; !ok {
			return errors.New("velocidad serie no compatible")
		}
	}
	if settings.Port == "" || settings.BaudRate == 0 || !nodeNumber.MatchString(settings.FavoriteNode) {
		return errors.New("para activar Nextion selecciona un puerto, una velocidad y un nodo favorito válidos")
	}
	local, err := newPrimaryNodeControl().primary()
	if err != nil {
		return fmt.Errorf("no se pudo determinar el nodo local: %w", err)
	}
	if settings.FavoriteNode == local {
		return errors.New("el nodo favorito debe ser distinto del nodo local")
	}
	ami := config.CurrentAMI()
	if ami.Host == "" || ami.Port == "" || ami.User == "" || ami.Secret == "" {
		return errors.New("la configuración AMI existente está incompleta")
	}
	return nil
}

// GetNextionSettings maneja la solicitud para obtener la configuración de Nextion, incluyendo los puertos disponibles, modelos, estado de ejecución y configuración de red actual.
func GetNextionSettings(c echo.Context) error {
	settings, err := readNextionSettings(c.Request().Context())
	if err != nil {
		log.Printf("nextion: no se pudo leer la configuración: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudo leer la configuración Nextion")
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	nextionRuntime.mu.Lock()
	runtime := nextionRuntimeStatus{
		Running: nextionRuntime.running, Port: nextionRuntime.port,
		LocalNode: nextionRuntime.local, Error: nextionRuntime.lastErr,
	}
	nextionRuntime.mu.Unlock()
	ami := config.CurrentAMI()
	return c.JSON(http.StatusOK, echo.Map{
		"settings": settings, "ports": discoverNextionPorts(),
		"models": listNextionModels(), "runtime": runtime,
		"interfaces":   discoverNextionInterfaces(),
		"current_ipv4": ipAddress(settings.Interface),
		"local_node":   configuredLocalNode(), "ami_configured": ami.Host != "" && ami.Port != "" && ami.User != "" && ami.Secret != "",
	})
}

// SaveNextionSettings maneja la solicitud para guardar la configuración de Nextion, validando los datos y actualizando la base de datos.
func SaveNextionSettings(c echo.Context) error {
	var request nextionSettingsRequest
	if err := c.Bind(&request); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud Nextion inválida")
	}
	settings := nextionSettings{
		Enabled: request.Enabled, Port: strings.TrimSpace(request.Port),
		BaudRate: request.BaudRate, Interface: strings.TrimSpace(request.Interface),
		FavoriteNode: strings.TrimSpace(request.FavoriteNode), SendLabels: true,
	}
	if request.SendLabels != nil {
		settings.SendLabels = *request.SendLabels
	}
	if err := validateNextionSettings(settings); err != nil {
		return apiError(http.StatusBadRequest, err.Error())
	}
	if config.App.DB == nil {
		return apiError(http.StatusServiceUnavailable, "La base de datos no está disponible")
	}
	values := map[string]string{
		"nextion_enabled": "0", "nextion_port": settings.Port,
		"nextion_baud_rate":     strconv.Itoa(settings.BaudRate),
		"nextion_interface":     settings.Interface,
		"nextion_favorite_node": settings.FavoriteNode,
		"nextion_send_labels":   "0",
	}
	if settings.Enabled {
		values["nextion_enabled"] = "1"
	}
	if settings.SendLabels {
		values["nextion_send_labels"] = "1"
	}
	tx, err := config.App.DB.BeginTx(c.Request().Context(), nil)
	if err != nil {
		log.Printf("nextion: no se pudo iniciar la transacción de configuración: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudo guardar la configuración Nextion")
	}
	defer tx.Rollback()
	for key, value := range values {
		if _, err := tx.ExecContext(c.Request().Context(),
			`INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			key, value); err != nil {
			log.Printf("nextion: no se pudo guardar %s: %v", key, err)
			return apiError(http.StatusInternalServerError, "No se pudo guardar la configuración Nextion")
		}
	}
	if err := tx.Commit(); err != nil {
		log.Printf("nextion: no se pudo confirmar la configuración: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudo guardar la configuración Nextion")
	}
	nextionRuntime.signalChange()
	return GetNextionSettings(c)
}

// TestNextionPort maneja la solicitud para probar un puerto serie de Nextion, verificando la validez del puerto y la velocidad de baudios, y asegurando que el puerto pueda abrirse y cerrarse correctamente.
func TestNextionPort(c echo.Context) error {
	var request nextionPortTestRequest
	if err := c.Bind(&request); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud de prueba inválida")
	}
	request.Port = strings.TrimSpace(request.Port)
	if !nextionPortName.MatchString(request.Port) {
		return apiError(http.StatusBadRequest, "El puerto debe ser un dispositivo /dev/ttyUSBx o /dev/ttyACMx")
	}
	if _, ok := nextionBaudRates[request.BaudRate]; !ok {
		return apiError(http.StatusBadRequest, "Velocidad serie no compatible")
	}
	settings, err := readNextionSettings(c.Request().Context())
	if err != nil {
		log.Printf("nextion: no se pudo leer la configuración al probar el puerto: %v", err)
		return apiError(http.StatusInternalServerError, "No se pudo leer la configuración Nextion")
	}
	nextionRuntime.mu.Lock()
	alreadyOpen := nextionRuntime.running && nextionRuntime.port == request.Port
	nextionRuntime.mu.Unlock()
	if alreadyOpen && settings.Enabled && settings.Port == request.Port && settings.BaudRate == request.BaudRate {
		return c.JSON(http.StatusOK, echo.Map{"status": "ok", "message": "El controlador ya está conectado a este puerto"})
	}
	port, err := openSerial(request.Port, request.BaudRate)
	if err != nil {
		return c.JSON(http.StatusConflict, echo.Map{"status": "error", "message": "No se pudo abrir el puerto serie: " + err.Error()})
	}
	if err := port.Close(); err != nil {
		log.Printf("nextion: error cerrando puerto después de probarlo: %v", err)
		return apiError(http.StatusInternalServerError, "El puerto respondió, pero no se pudo cerrar correctamente")
	}
	return c.JSON(http.StatusOK, echo.Map{"status": "ok"})
}

// signalChange notifica al supervisor de Nextion que debe reconciliar la configuración, enviando una señal a través del canal de notificación.
func (s *nextionService) signalChange() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// StartNextionSupervisor inicia el supervisor de Nextion en un contexto dado, ejecutando la supervisión en una goroutine separada.
func StartNextionSupervisor(ctx context.Context) {
	go nextionRuntime.supervise(ctx)
}

// supervise ejecuta el bucle de supervisión del servicio Nextion, reconciliando la configuración periódicamente y respondiendo a señales de cambio.
func (s *nextionService) supervise(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		s.reconcile(ctx)
		select {
		case <-ctx.Done():
			s.stopWorker()
			return
		case <-ticker.C:
		case <-s.notify:
		}
	}
}

// reconcile sincroniza la configuración del servicio Nextion con la configuración actual, deteniendo el trabajador si es necesario y validando los ajustes antes de iniciar un nuevo trabajador.
func (s *nextionService) reconcile(ctx context.Context) {
	settings, err := readNextionSettings(ctx)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err.Error()
		s.mu.Unlock()
		log.Printf("nextion: no se pudo leer la configuración para sincronizar el controlador: %v", err)
		return
	}
	language := readAppLanguage(ctx)
	revision := fmt.Sprintf("%t|%s|%d|%s|%s|%s|%t", settings.Enabled, settings.Port, settings.BaudRate, settings.Interface, settings.FavoriteNode, language, settings.SendLabels)
	s.mu.Lock()
	if revision == s.revision && (!settings.Enabled || s.cancel != nil) {
		s.mu.Unlock()
		return
	}
	s.revision = revision
	s.mu.Unlock()

	s.stopWorker()
	if !settings.Enabled {
		s.setRuntime(false, "", "", "")
		return
	}
	if err := validateNextionSettings(settings); err != nil {
		s.setRuntime(false, settings.Port, "", err.Error())
		log.Printf("nextion: configuración activada no válida: %v", err)
		return
	}
	local, err := newPrimaryNodeControl().primary()
	if err != nil {
		s.setRuntime(false, settings.Port, "", err.Error())
		return
	}
	amiConfig := config.CurrentAMI()
	radio := &ami{host: amiConfig.Host, port: amiConfig.Port, username: amiConfig.User, secret: amiConfig.Secret, node: local}
	p := &panel{local: local, favorite: settings.FavoriteNode, network: settings.Interface, radio: radio,
		language: language, sendLabels: settings.SendLabels, executeSystem: executeSystem}
	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.mu.Lock()
	s.cancel, s.done = cancel, done
	s.port, s.local, s.running, s.lastErr = settings.Port, local, false, ""
	s.mu.Unlock()
	go s.runWorker(workerCtx, p, settings, done)
}

// runWorker ejecuta el trabajador de Nextion, gestionando la conexión serial, la ejecución del panel y el manejo de errores, reiniciando el proceso cuando es necesario.
func (s *nextionService) runWorker(ctx context.Context, p *panel, settings nextionSettings, done chan struct{}) {
	defer close(done)
	for {
		if ctx.Err() != nil {
			return
		}
		port, err := openSerial(settings.Port, settings.BaudRate)
		if err != nil {
			s.setRuntime(false, settings.Port, p.local, err.Error())
			log.Printf("nextion: no se pudo abrir %s: %v; reintento en 2 segundos", settings.Port, err)
		} else {
			s.setRuntime(true, settings.Port, p.local, "")
			log.Printf("nextion: conectado a %s a %d baudios", settings.Port, settings.BaudRate)
			runDone := make(chan error, 1)
			go func() { runDone <- p.run(port) }()
			select {
			case <-ctx.Done():
				_ = port.Close()
				<-runDone
				p.radio.close()
				return
			case err = <-runDone:
				_ = port.Close()
				p.radio.close()
				if ctx.Err() != nil {
					return
				}
				s.setRuntime(false, settings.Port, p.local, err.Error())
				log.Printf("nextion: comunicación interrumpida: %v; reintento en 2 segundos", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// stopWorker detiene el trabajador de Nextion, cancelando su contexto y esperando a que finalice.
func (s *nextionService) stopWorker() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done = nil, nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

// setRuntime actualiza el estado de ejecución del servicio Nextion, incluyendo el puerto, el nodo local y el último error.
func (s *nextionService) setRuntime(running bool, port, local, lastErr string) {
	s.mu.Lock()
	s.running, s.port, s.local, s.lastErr = running, port, local, lastErr
	s.mu.Unlock()
}

// configuredLocalNode devuelve el nodo local configurado actualmente, o una cadena vacía si no se puede determinar.
func configuredLocalNode() string {
	node, err := newPrimaryNodeControl().primary()
	if err != nil {
		return ""
	}
	return node
}

// discoverNextionPorts busca y devuelve una lista de puertos serie disponibles que podrían estar conectados a dispositivos Nextion.
func discoverNextionPorts() []nextionPort {
	paths, err := filepath.Glob("/dev/ttyUSB*")
	if err != nil {
		log.Printf("nextion: no se pudieron enumerar puertos USB: %v", err)
		return []nextionPort{}
	}
	acm, err := filepath.Glob("/dev/ttyACM*")
	if err != nil {
		log.Printf("nextion: no se pudieron enumerar puertos ACM: %v", err)
	}
	paths = append(paths, acm...)
	sort.Strings(paths)
	ports := make([]nextionPort, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			continue
		}
		base := filepath.Base(path)
		sysfs := filepath.Join("/sys/class/tty", base, "device")
		if resolved, err := filepath.EvalSymlinks(sysfs); err == nil {
			sysfs = resolved
		}
		port := nextionPort{Path: path, Label: path}
		for current := sysfs; current != "/" && current != "."; current = filepath.Dir(current) {
			vendorID := readSysfsString(current, "idVendor", "")
			productID := readSysfsString(current, "idProduct", "")
			if vendorID == "" || productID == "" {
				continue
			}
			port.VendorID, port.ProductID = vendorID, productID
			port.Manufacturer = readSysfsString(current, "manufacturer", "")
			port.Product = readSysfsString(current, "product", "")
			port.Serial = readSysfsString(current, "serial", "")
			break
		}
		if strings.EqualFold(port.VendorID, "10c4") && strings.EqualFold(port.ProductID, "ea60") {
			port.Label += " — Silicon Labs CP210x"
		} else if port.Product != "" {
			port.Label += " — " + port.Product
		}
		if port.VendorID != "" && port.ProductID != "" {
			port.Label += fmt.Sprintf(" (%s:%s)", port.VendorID, port.ProductID)
		}
		ports = append(ports, port)
	}
	return ports
}

// readSysfsString lee un valor de cadena desde el sistema de archivos sysfs, devolviendo un valor de respaldo si ocurre un error.
func readSysfsString(dir, name, fallback string) string {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return fallback
	}
	return strings.TrimSpace(string(data))
}

// listNextionModels devuelve una lista de modelos de Nextion disponibles, incluyendo información sobre la disponibilidad de la fuente y la URL de vista previa.
func listNextionModels() []nextionModel {
	dirs := nextionFirmwareDirs()
	models := make([]nextionModel, 0)
	seen := make(map[string]bool)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Printf("nextion: no se pudo leer el directorio de firmware %s: %v", dir, err)
			}
			continue
		}
		for _, entry := range entries {
			extension := strings.ToLower(filepath.Ext(entry.Name()))
			if entry.IsDir() || extension != ".tft" {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			match := nextionModelName.FindStringSubmatch(name)
			if match == nil || seen[name] {
				continue
			}
			seen[name] = true
			models = append(models, nextionModel{
				Name: name, Model: match[1],
				PreviewURL: "assets/images/" + name + ".png",
			})
		}
	}
	for i := range models {
		for _, dir := range dirs {
			if _, err := os.Stat(filepath.Join(dir, models[i].Name+".HMI")); err == nil {
				models[i].SourceAvailable = true
				break
			}
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	return models
}

// nextionFirmwareDirs devuelve una lista de directorios donde se pueden encontrar los archivos de firmware de Nextion, incluyendo directorios predeterminados, configurados y relativos al directorio de trabajo actual.
func nextionFirmwareDirs() []string {
	dirs := []string{"/usr/share/amerimon/nextion"}
	if configured := os.Getenv("AMERIMON_NEXTION_DIR"); configured != "" {
		dirs = append([]string{configured}, dirs...)
	}
	if cwd, err := os.Getwd(); err == nil {
		for current := cwd; ; current = filepath.Dir(current) {
			dirs = append(dirs, filepath.Join(current, "public", "assets", "firmware"))
			if filepath.Dir(current) == current {
				break
			}
		}
	}
	return dirs
}

// DownloadNextionFile maneja la descarga de archivos de firmware de Nextion, validando el modelo y la extensión, y sirviendo el archivo correspondiente si existe.
func DownloadNextionFile(c echo.Context) error {
	name := c.Param("model")
	if !nextionModelName.MatchString(name) {
		return apiError(http.StatusBadRequest, "Modelo Nextion inválido")
	}
	extension := strings.ToLower(c.Param("extension"))
	if extension != "tft" && extension != "hmi" {
		return apiError(http.StatusBadRequest, "Tipo de archivo Nextion inválido")
	}
	filename := name + "." + extension
	for _, dir := range nextionFirmwareDirs() {
		path := filepath.Join(dir, filename)
		if extension == "hmi" {
			path = filepath.Join(dir, name+".HMI")
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			continue
		}
		c.Response().Header().Set("Content-Type", "application/octet-stream")
		c.Response().Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
		return c.File(path)
	}
	return apiError(http.StatusNotFound, "No se encontró el archivo Nextion solicitado")
}
