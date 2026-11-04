package controllers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"amerimon/internal/config"

	"github.com/labstack/echo/v4"
)

var amiSecretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,128}$`)
var amiUserPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

type primaryAMIConfig struct {
	User        string   `json:"user"`
	Secret      string   `json:"secret"`
	SecretSet   bool     `json:"secret_set"`
	BindAddress string   `json:"bind_address"`
	Permit      []string `json:"permit"`
	Deny        []string `json:"deny"`
}

type primaryAMIRequest struct {
	Secret      string   `json:"secret"`
	BindAddress string   `json:"bind_address"`
	Permit      []string `json:"permit"`
	Deny        []string `json:"deny"`
}

type amiConfigFile struct {
	path     string
	original string
	updated  string
}

func GetPrimaryNodeAMIConfig(c echo.Context) error {
	return newPrimaryNodeControl().getAMIConfig(c)
}

func SavePrimaryNodeAMIConfig(c echo.Context) error {
	return newPrimaryNodeControl().saveAMIConfig(c)
}

func (p primaryNodeControl) getAMIConfig(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	c.Response().Header().Set("Cache-Control", "no-store")
	ami := config.CurrentAMI()
	if !amiUserPattern.MatchString(ami.User) {
		return apiError(http.StatusServiceUnavailable, "No hay un usuario AMI configurado en amerimon.yaml")
	}
	data, err := os.ReadFile(p.managerPath)
	if err != nil {
		return primaryNodeError("No se pudo leer manager.conf", err)
	}
	sections := parseManagerSections(string(data))
	bindAddress := "0.0.0.0"
	userCount, generalCount := 0, 0
	result := primaryAMIConfig{
		User:   ami.User,
		Secret: ami.Secret,
	}
	for _, section := range sections {
		switch {
		case strings.EqualFold(section.name, "general"):
			generalCount++
			if value, ok := section.last("bindaddr"); ok {
				bindAddress = value
			}
		case strings.EqualFold(section.name, result.User):
			userCount++
			if value, ok := section.last("secret"); ok {
				result.Secret = value
				result.SecretSet = value != ""
			}
			result.Permit = append(result.Permit, section.values("permit")...)
			result.Deny = append(result.Deny, section.values("deny")...)
		}
	}
	if userCount != 1 || generalCount != 1 {
		return apiError(http.StatusConflict, "manager.conf debe contener una sola sección [general] y una stanza del usuario AMI configurado")
	}
	result.BindAddress = bindAddress
	return c.JSON(http.StatusOK, result)
}

func (p primaryNodeControl) saveAMIConfig(c echo.Context) error {
	var req primaryAMIRequest
	if err := c.Bind(&req); err != nil {
		return apiError(http.StatusBadRequest, "Solicitud inválida")
	}
	req.Secret = strings.TrimSpace(req.Secret)
	req.BindAddress = strings.TrimSpace(req.BindAddress)
	if !amiSecretPattern.MatchString(req.Secret) {
		return apiError(http.StatusBadRequest, "El secret AMI debe tener de 6 a 128 letras, números, _ o -")
	}
	bindIP, err := netip.ParseAddr(req.BindAddress)
	if err != nil || bindIP.Zone() != "" || bindIP.IsMulticast() {
		return apiError(http.StatusBadRequest, "El bind address debe ser una dirección IPv4 o IPv6 válida")
	}
	ami := config.CurrentAMI()
	if !amiUserPattern.MatchString(ami.User) || strings.TrimSpace(ami.Host) == "" ||
		strings.TrimSpace(ami.Port) == "" || config.App.Path == "" || config.App.EnvPath == "" {
		return apiError(http.StatusServiceUnavailable, "La conexión AMI, amerimon.yaml o AMERIMON_ENV_FILE no están configurados")
	}
	permit, err := normalizeManagerNetworks(req.Permit)
	if err != nil {
		return apiError(http.StatusBadRequest, "Red permitida inválida: "+err.Error())
	}
	deny, err := normalizeManagerNetworks(req.Deny)
	if err != nil {
		return apiError(http.StatusBadRequest, "Red denegada inválida: "+err.Error())
	}
	if !systemControlLock.TryLock() {
		return apiError(http.StatusConflict, "Hay otra operación de configuración o mantenimiento en curso")
	}
	defer systemControlLock.Unlock()
	ami = config.CurrentAMI()

	managerData, err := os.ReadFile(p.managerPath)
	if err != nil {
		return primaryNodeError("No se pudo leer manager.conf", err)
	}
	yamlPath := config.App.Path
	envPath := config.App.EnvPath
	yamlData, err := os.ReadFile(yamlPath)
	if err != nil {
		return primaryNodeError("No se pudo leer amerimon.yaml", err)
	}
	envData, err := os.ReadFile(envPath)
	if err != nil {
		return primaryNodeError("No se pudo leer amerimon.env", err)
	}
	sections := parseManagerSections(string(managerData))
	userCount, generalCount := 0, 0
	var existingDeny, existingPermit []string
	for _, section := range sections {
		if strings.EqualFold(section.name, ami.User) {
			userCount++
			existingDeny = append(existingDeny, section.values("deny")...)
			existingPermit = append(existingPermit, section.values("permit")...)
		}
		if strings.EqualFold(section.name, "general") {
			generalCount++
		}
	}
	if userCount != 1 || generalCount != 1 {
		return apiError(http.StatusConflict, "manager.conf debe contener [general] y la stanza del usuario AMI configurado")
	}
	newHost := strings.TrimSpace(ami.Host)
	if !bindIP.IsUnspecified() {
		newHost = bindIP.String()
	}
	protected := []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1")}
	for _, host := range []string{ami.Host, newHost} {
		host = strings.Trim(host, "[]")
		if addr, parseErr := netip.ParseAddr(host); parseErr == nil {
			protected = append(protected, addr)
			continue
		}
		resolved, lookupErr := resolveProtectedAMIHost(host)
		if lookupErr == nil {
			protected = append(protected, resolved...)
		} else if len(existingPermit)+len(existingDeny)+len(permit)+len(deny) > 0 {
			return apiError(http.StatusConflict, "No se pudo comprobar qué red usa el backend AMI; no se aplicaron reglas de red")
		}
	}
	finalDeny := append(append([]string{}, existingDeny...), deny...)
	for _, rule := range finalDeny {
		prefix, parseErr := managerPrefix(rule)
		if parseErr != nil {
			return apiError(http.StatusConflict, "No se puede verificar una regla deny existente en manager.conf; revísala antes de continuar")
		}
		for _, addr := range protected {
			if prefix.Contains(addr) {
				return apiError(http.StatusConflict, "La regla deny "+rule+" bloquearía el acceso AMI local de Amerimon; no se aplicó ningún cambio")
			}
		}
	}
	updatedManager, err := updateManagerConfig(string(managerData), ami.User, req.Secret, req.BindAddress, permit, deny, protected)
	if err != nil {
		return primaryNodeError("No se pudo preparar manager.conf", err)
	}
	updatedYAML, err := updateAMIYAML(string(yamlData), newHost, ami.User)
	if err != nil {
		return primaryNodeError("No se pudo preparar amerimon.yaml", err)
	}
	updatedEnv, err := updateAMIEnvironment(string(envData), ami.User, req.Secret)
	if err != nil {
		return primaryNodeError("No se pudo preparar amerimon.env", err)
	}
	changes := []amiConfigFile{
		{p.managerPath, string(managerData), updatedManager},
		{yamlPath, string(yamlData), updatedYAML},
		{envPath, string(envData), updatedEnv},
	}
	for _, change := range changes {
		if change.original == change.updated {
			continue
		}
		if err := writePrimaryBackup(change.path, change.original); err != nil {
			return primaryNodeError("No se pudo crear respaldo de "+filepath.Base(change.path), err)
		}
	}
	written := make([]amiConfigFile, 0, len(changes))
	managerTouched := false
	rollback := func(cause error) error {
		var failures []string
		for i := len(written) - 1; i >= 0; i-- {
			if err := writePrimaryFile(written[i].path, written[i].original); err != nil {
				failures = append(failures, filepath.Base(written[i].path))
			}
		}
		if managerTouched {
			reloaded := false
			if _, err := p.amiLogin(newHost, ami.Port, ami.User, req.Secret, "manager reload"); err == nil {
				reloaded = true
			} else if _, fallbackErr := p.ami("manager reload"); fallbackErr == nil {
				reloaded = true
			}
			if !reloaded {
				failures = append(failures, "recarga de la configuración AMI anterior")
			} else if _, err := p.amiLogin(ami.Host, ami.Port, ami.User, ami.Secret, "manager show settings"); err != nil {
				failures = append(failures, "verificación de la conexión AMI anterior")
			}
		}
		if len(failures) > 0 {
			return primaryNodeError("Falló el cambio AMI y no se pudo completar la restauración de "+strings.Join(failures, ", "), cause)
		}
		return primaryNodeError("No se aplicaron los cambios AMI; se restauró la configuración anterior", cause)
	}
	for _, change := range changes {
		if change.original == change.updated {
			continue
		}
		current, err := os.ReadFile(change.path)
		if err != nil || string(current) != change.original {
			return rollback(errors.New("un archivo cambió durante el guardado"))
		}
		written = append(written, change)
		if err := writePrimaryFile(change.path, change.updated); err != nil {
			return rollback(err)
		}
		if change.path == p.managerPath {
			managerTouched = true
		}
	}
	managerTouched = true
	if _, err := p.ami("manager reload"); err != nil {
		return rollback(err)
	}
	if _, err := p.amiLogin(newHost, ami.Port, ami.User, req.Secret, "manager show settings"); err != nil {
		return rollback(err)
	}
	ami.Host = newHost
	ami.Secret = req.Secret
	config.SetAMI(ami)
	return c.JSON(http.StatusOK, echo.Map{
		"status":  "ok",
		"message": "Configuración AMI guardada y verificada. El backend conserva acceso local; Asterisk ejecutó manager reload.",
	})
}

func resolveProtectedAMIHost(host string) ([]netip.Addr, error) {
	if strings.EqualFold(host, "localhost") {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("::1")}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func normalizeManagerNetworks(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := managerPrefix(value)
		if err != nil {
			return nil, fmt.Errorf("%q no es una IP/CIDR válida", value)
		}
		result = append(result, prefix.Masked().String())
	}
	return result, nil
}

func managerPrefix(value string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix, nil
	}
	if address, mask, ok := strings.Cut(value, "/"); ok {
		addr, addressErr := netip.ParseAddr(strings.Trim(address, "[]"))
		if addressErr == nil && addr.Is4() {
			maskIP := net.ParseIP(mask)
			if maskIP != nil {
				ones, bits := net.IPMask(maskIP.To4()).Size()
				if bits == 32 && ones >= 0 {
					return netip.PrefixFrom(addr, ones), nil
				}
			}
		}
	}
	addr, err := netip.ParseAddr(strings.Trim(value, "[]"))
	if err != nil {
		return netip.Prefix{}, err
	}
	bits := 128
	if addr.Is4() {
		bits = 32
	}
	return netip.PrefixFrom(addr, bits), nil
}

func hasMatchingNetwork(networks []string, addr netip.Addr) bool {
	for _, network := range networks {
		prefix, err := managerPrefix(network)
		if err == nil && prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func hasEquivalentNetwork(networks []string, candidate string) bool {
	prefix, err := managerPrefix(candidate)
	if err != nil {
		return false
	}
	prefix = prefix.Masked()
	for _, network := range networks {
		existing, err := managerPrefix(network)
		if err == nil && existing.Masked() == prefix {
			return true
		}
	}
	return false
}

type managerSection struct {
	name  string
	start int
	end   int
	lines []string
}

var managerHeaderPattern = regexp.MustCompile(`^\s*\[([^\]]+)\]`)
var managerOptionPattern = regexp.MustCompile(`^\s*([A-Za-z0-9_]+)\s*(?:=>|=)\s*([^;#]*?)\s*(?:[;#].*)?$`)

func parseManagerSections(content string) []managerSection {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var sections []managerSection
	for i, line := range lines {
		header := managerHeaderPattern.FindStringSubmatch(line)
		if header == nil || strings.HasPrefix(strings.TrimSpace(line), ";") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if len(sections) > 0 {
			sections[len(sections)-1].end = i
		}
		sections = append(sections, managerSection{name: strings.TrimSpace(header[1]), start: i, end: len(lines), lines: lines})
	}
	return sections
}

func (s managerSection) options(key string) []string {
	var values []string
	for _, line := range s.lines[s.start+1 : s.end] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		match := managerOptionPattern.FindStringSubmatch(line)
		if match != nil && strings.EqualFold(match[1], key) {
			values = append(values, strings.TrimSpace(match[2]))
		}
	}
	return values
}

func (s managerSection) values(key string) []string { return s.options(key) }

func (s managerSection) last(key string) (string, bool) {
	values := s.options(key)
	if len(values) == 0 {
		return "", false
	}
	return values[len(values)-1], true
}

func updateManagerConfig(content, user, secret, bindAddress string, permits, denies []string, protected []netip.Addr) (string, error) {
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	sections := parseManagerSections(content)
	var general, userSection *managerSection
	generalCount, userCount := 0, 0
	for i := range sections {
		if strings.EqualFold(sections[i].name, "general") {
			generalCount++
			general = &sections[i]
		}
		if strings.EqualFold(sections[i].name, user) {
			userCount++
			userSection = &sections[i]
		}
	}
	if generalCount != 1 || userCount != 1 || general == nil || userSection == nil {
		return "", errors.New("manager.conf debe contener una sola sección [general] y una stanza del usuario AMI")
	}
	lines, general, err := replaceManagerOption(lines, *general, "bindaddr", bindAddress)
	if err != nil {
		return "", err
	}
	sections = parseManagerSections(strings.Join(lines, "\n"))
	general, userSection = nil, nil
	for i := range sections {
		if strings.EqualFold(sections[i].name, "general") {
			general = &sections[i]
		}
		if strings.EqualFold(sections[i].name, user) {
			userSection = &sections[i]
		}
	}
	if general == nil || userSection == nil {
		return "", errors.New("no se pudo volver a localizar la configuración AMI")
	}
	lines, _, err = replaceManagerOption(lines, *userSection, "secret", secret)
	if err != nil {
		return "", err
	}
	_ = protected
	sections = parseManagerSections(strings.Join(lines, "\n"))
	userSection = nil
	for i := range sections {
		if strings.EqualFold(sections[i].name, user) {
			userSection = &sections[i]
			break
		}
	}
	if userSection == nil {
		return "", errors.New("no se encontró la stanza del usuario AMI")
	}
	existingPermit, existingDeny := userSection.values("permit"), userSection.values("deny")
	newRules := make([]string, 0, len(permits)+len(denies))
	for _, candidate := range permits {
		if !hasEquivalentNetwork(existingPermit, candidate) && !hasEquivalentNetworkOption(newRules, "permit", candidate) {
			newRules = append(newRules, formatManagerACL("permit", candidate))
		}
	}
	for _, candidate := range denies {
		if !hasEquivalentNetwork(existingDeny, candidate) && !hasEquivalentNetworkOption(newRules, "deny", candidate) {
			newRules = append(newRules, formatManagerACL("deny", candidate))
		}
	}
	if len(existingPermit)+len(permits) > 0 {
		for _, addr := range protected {
			allowed := false
			for _, rule := range append(append([]string{}, existingPermit...), permits...) {
				prefix, err := managerPrefix(rule)
				if err == nil && prefix.Contains(addr) {
					allowed = true
					break
				}
			}
			if !allowed {
				if addr.Is4() {
					if !hasEquivalentNetworkOption(newRules, "permit", addr.String()+"/32") {
						newRules = append(newRules, formatManagerACL("permit", addr.String()+"/32"))
					}
				} else {
					if !hasEquivalentNetworkOption(newRules, "permit", addr.String()+"/128") {
						newRules = append(newRules, formatManagerACL("permit", addr.String()+"/128"))
					}
				}
			}
		}
	}
	if len(newRules) > 0 {
		insert := append([]string{""}, newRules...)
		lines = append(lines[:userSection.end], append(insert, lines[userSection.end:]...)...)
	}

	return strings.ReplaceAll(strings.Join(lines, "\n"), "\n", eol), nil
}

func hasEquivalentNetworkOption(options []string, key, candidate string) bool {
	prefix, err := managerPrefix(candidate)
	if err != nil {
		return false
	}
	for _, option := range options {
		name, value, ok := strings.Cut(option, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), key) {
			continue
		}
		existing, err := managerPrefix(strings.TrimSpace(value))
		if err == nil && existing.Masked() == prefix.Masked() {
			return true
		}
	}
	return false
}

func formatManagerACL(key, value string) string {
	prefix, err := managerPrefix(value)
	if err != nil {
		return key + "=" + value
	}
	prefix = prefix.Masked()
	if prefix.Addr().Is4() {
		mask := net.IP(net.CIDRMask(prefix.Bits(), 32)).String()
		return key + "=" + prefix.Addr().String() + "/" + mask
	}
	return key + "=" + prefix.String()
}

func replaceManagerOption(lines []string, section managerSection, key, value string) ([]string, *managerSection, error) {
	found := false
	pattern := regexp.MustCompile(`(?i)^(\s*)(` + regexp.QuoteMeta(key) + `\s*(?:=|=>)\s*)([^;#]*?)(\s*(?:[;#].*)?)$`)
	for i := section.start + 1; i < section.end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		match := pattern.FindStringSubmatch(lines[i])
		if match != nil {
			lines[i] = match[1] + match[2] + value + match[4]
			found = true
		}
	}
	if !found {
		lines = append(lines[:section.end], append([]string{key + " = " + value}, lines[section.end:]...)...)
	}
	updated := parseManagerSections(strings.Join(lines, "\n"))
	for i := range updated {
		if updated[i].name == section.name {
			return lines, &updated[i], nil
		}
	}
	return nil, nil, errors.New("no se pudo actualizar una opción de manager.conf")
}

func updateAMIYAML(content, host, user string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	sectionStart, sectionEnd := -1, len(lines)
	for i, line := range lines {
		if strings.TrimSpace(line) == "ami:" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			sectionStart = i
			continue
		}
		if sectionStart >= 0 && line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			sectionEnd = i
			break
		}
	}
	if sectionStart < 0 {
		return "", errors.New("no se encontró la sección ami en amerimon.yaml")
	}
	values := map[string]string{"host": host, "user": user, "secret": "${AMI_SECRET}"}
	found := make(map[string]bool)
	for i := sectionStart + 1; i < sectionEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		key, _, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value, exists := values[key]
		if !exists {
			continue
		}
		comment := ""
		if _, after, hasComment := strings.Cut(lines[i], " #"); hasComment {
			comment = " #" + after
		}
		indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
		lines[i] = indent + key + ": " + strconv.Quote(value) + comment
		found[key] = true
	}
	missing := make([]string, 0, len(values))
	for _, key := range []string{"host", "user", "secret"} {
		if !found[key] {
			missing = append(missing, "  "+key+": "+strconv.Quote(values[key]))
		}
	}
	if len(missing) > 0 {
		lines = append(lines[:sectionEnd], append(missing, lines[sectionEnd:]...)...)
	}
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	return strings.ReplaceAll(strings.Join(lines, "\n"), "\n", eol), nil
}

func updateAMIEnvironment(content, user, secret string) (string, error) {
	values := map[string]string{"AMI_USER": user, "AMI_SECRET": secret}
	found := make(map[string]bool)
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	pattern := regexp.MustCompile(`^(\s*(?:export\s+)?)(AMI_USER|AMI_SECRET)(\s*=\s*)(.*?)(\s+#.*)?$`)
	for i, line := range lines {
		match := pattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		key := match[2]
		lines[i] = match[1] + match[2] + match[3] + strconv.Quote(values[key]) + match[5]
		found[key] = true
	}
	for _, key := range []string{"AMI_USER", "AMI_SECRET"} {
		if !found[key] {
			lines = append(lines, key+"="+strconv.Quote(values[key]))
		}
	}
	eol := "\n"
	if strings.Contains(content, "\r\n") {
		eol = "\r\n"
	}
	return strings.ReplaceAll(strings.Join(lines, "\n"), "\n", eol), nil
}
