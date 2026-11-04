package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

var echoLinkSectionPattern = regexp.MustCompile(`^el[0-9]+$`)
var echoLinkCallPattern = regexp.MustCompile(`^[A-Z0-9]+(?:-[LR])?$`)
var echoLinkContextPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,80}$`)
var echoLinkHostPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
var echoLinkKeys = []string{
	"call", "name", "qth", "email", "node", "lat", "lon", "freq", "tone",
	"power", "height", "gain", "dir", "message", "maxstns", "rtcptimeout",
	"recfile", "context", "server1", "server2", "server3", "server4",
}

type echoLinkConfigControl struct {
	path, modulesPath string
	primary           primaryNodeControl
}

type echoLinkSection struct {
	Section     string            `json:"section"`
	Values      map[string]string `json:"values"`
	PasswordSet bool              `json:"password_set"`
}

type echoLinkRequest struct {
	Section  string            `json:"section"`
	Revision string            `json:"revision"`
	AstNode  string            `json:"astnode"`
	Values   map[string]string `json:"values"`
	Password string            `json:"password"`
}

func newEchoLinkConfigControl() echoLinkConfigControl {
	p := newPrimaryNodeControl()
	// AMI Command blacklists module load/unload; module state and changes use the local CLI.
	p.ami = func(command string) (string, error) {
		return runSystemCommand(context.Background(), 10*time.Second, "asterisk", "-rx", command)
	}
	dir := filepath.Dir(p.rptPath)
	return echoLinkConfigControl{filepath.Join(dir, "echolink.conf"), filepath.Join(dir, "modules.conf"), p}
}

func GetEchoLinkConfig(c echo.Context) error   { return newEchoLinkConfigControl().get(c) }
func GetEchoLinkPassword(c echo.Context) error { return newEchoLinkConfigControl().password(c) }
func SaveEchoLinkConfig(c echo.Context) error  { return newEchoLinkConfigControl().save(c) }
func ActivateEchoLink(c echo.Context) error    { return newEchoLinkConfigControl().activate(c) }
func CheckEchoLinkPorts(c echo.Context) error  { return newEchoLinkConfigControl().ports(c) }

func (p echoLinkConfigControl) ports(c echo.Context) error {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return echoLinkAPIError(c, 500, "ECHOLINK_READ_ERROR", err)
	}
	sections, err := p.sections(string(data))
	if err != nil {
		return echoLinkAPIError(c, 409, "ECHOLINK_STRUCTURE_ERROR", err)
	}
	servers := make(map[string]bool)
	results := make([]echo.Map, 0)
	ctx, cancel := context.WithTimeout(c.Request().Context(), 10*time.Second)
	defer cancel()
	for _, section := range sections {
		for _, key := range []string{"server1", "server2", "server3", "server4"} {
			server := section.Values[key]
			if server == "" || servers[server] {
				continue
			}
			if !echoLinkHostPattern.MatchString(server) {
				return echoLinkAPIError(c, 409, "ECHOLINK_VALIDATION_ERROR", nil)
			}
			servers[server] = true
			conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(server, "5200"))
			if err == nil {
				conn.Close()
			}
			results = append(results, echo.Map{"protocol": "TCP", "port": 5200, "server": server, "reachable": err == nil})
		}
	}
	// /proc/net exposes local listeners only; never claim public NAT reachability.
	for _, port := range []int{5198, 5199} {
		listening := false
		for _, path := range []string{"/proc/net/udp", "/proc/net/udp6"} {
			table, err := os.ReadFile(path)
			if err != nil {
				return echoLinkAPIError(c, 500, "ECHOLINK_PORTS_ERROR", err)
			}
			for _, line := range strings.Split(string(table), "\n")[1:] {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}
				_, value, ok := strings.Cut(fields[1], ":")
				number, err := strconv.ParseInt(value, 16, 32)
				if ok && err == nil && number == int64(port) {
					listening = true
				}
			}
		}
		results = append(results, echo.Map{"protocol": "UDP", "port": port, "listening": listening})
	}
	return c.JSON(200, echo.Map{"results": results, "public_reachability_verified": false})
}

func (p echoLinkConfigControl) password(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	data, err := os.ReadFile(p.path)
	if err != nil {
		return echoLinkAPIError(c, 500, "ECHOLINK_READ_ERROR", err)
	}
	if c.QueryParam("revision") != echoLinkRevision(data) {
		return echoLinkAPIError(c, 409, "ECHOLINK_STALE", nil)
	}
	if _, err := p.sections(string(data)); err != nil {
		return echoLinkAPIError(c, 409, "ECHOLINK_STRUCTURE_ERROR", err)
	}
	for _, section := range parseManagerSections(string(data)) {
		if section.name == c.QueryParam("section") && echoLinkSectionPattern.MatchString(section.name) {
			return c.JSON(200, echo.Map{"password": sipOption(section, "pwd", "")})
		}
	}
	return echoLinkAPIError(c, 400, "ECHOLINK_STRUCTURE_ERROR", nil)
}

func echoLinkRevision(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func echoLinkAPIError(c echo.Context, status int, code string, err error) error {
	if err != nil {
		// No file contents, passwords or AMI output are included in diagnostics.
		log.Printf("echolink: %s", code)
	}
	return c.JSON(status, echo.Map{"status": "error", "code": code})
}

func echoLinkSections(content string) ([]echoLinkSection, error) {
	result := make([]echoLinkSection, 0)
	seen := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return nil, errors.New("included configurations require manual editing")
		}
	}
	for _, section := range parseManagerSections(content) {
		if !echoLinkSectionPattern.MatchString(section.name) {
			continue
		}
		if seen[section.name] || !plainSIPSection(section) {
			return nil, errors.New("ambiguous EchoLink section")
		}
		seen[section.name] = true
		item := echoLinkSection{Section: section.name, Values: make(map[string]string)}
		for _, key := range echoLinkKeys {
			item.Values[key] = sipOption(section, key, "")
		}
		item.Values["astnode"] = sipOption(section, "astnode", "")
		item.PasswordSet = sipOption(section, "pwd", "") != ""
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil, errors.New("no editable EchoLink sections")
	}
	return result, nil
}

func (p echoLinkConfigControl) sections(content string) ([]echoLinkSection, error) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		match := iaxClientUsersInclude.FindStringSubmatch(line)
		if match == nil {
			return nil, errors.New("included configurations require manual editing")
		}
		target := ""
		for _, value := range match[1:] {
			if value != "" {
				target = value
				break
			}
		}
		if target == "" || strings.ContainsAny(target, "*?[]") {
			return nil, errors.New("ambiguous optional include")
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(p.path), target)
		}
		_, err := os.Stat(target)
		if !errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("optional include exists or cannot be checked")
		}
		// Asterisk ignores absent optional includes; keep them in the saved file.
		lines[i] = ""
	}
	return echoLinkSections(strings.Join(lines, "\n"))
}

func echoLinkModuleLoaded(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "Output: "))
		if len(fields) > 0 && fields[0] == "chan_echolink.so" {
			return true
		}
	}
	return false
}

func (p echoLinkConfigControl) loaded() (bool, error) {
	output, err := p.primary.ami("module show like chan_echolink.so")
	if err != nil {
		return false, err
	}
	lower := strings.ToLower(output)
	if strings.Contains(lower, "no such command") || strings.Contains(lower, "response: error") ||
		(!echoLinkModuleLoaded(output) && !strings.Contains(lower, "0 modules loaded")) {
		return false, errors.New("module status unavailable")
	}
	return echoLinkModuleLoaded(output), nil
}

func (p echoLinkConfigControl) get(c echo.Context) error {
	systemControlLock.Lock()
	defer systemControlLock.Unlock()
	c.Response().Header().Set("Cache-Control", "no-store")
	data, err := os.ReadFile(p.path)
	if err != nil {
		return echoLinkAPIError(c, 500, "ECHOLINK_READ_ERROR", err)
	}
	sections, err := p.sections(string(data))
	if err != nil {
		return echoLinkAPIError(c, 409, "ECHOLINK_STRUCTURE_ERROR", err)
	}
	node, err := p.primary.primary()
	if err != nil {
		return echoLinkAPIError(c, 409, "ECHOLINK_PRIMARY_ERROR", err)
	}
	loaded, err := p.loaded()
	if err != nil {
		return echoLinkAPIError(c, 502, "ECHOLINK_STATUS_ERROR", err)
	}
	return c.JSON(200, echo.Map{"path": p.path, "astnode": node, "sections": sections,
		"revision": echoLinkRevision(data), "loaded": loaded})
}

func validateEchoLinkValues(values map[string]string) error {
	allowed := make(map[string]bool)
	for _, key := range echoLinkKeys {
		allowed[key] = true
	}
	for key, value := range values {
		if !allowed[key] || len(value) > 2048 || !validSIPValues(value) {
			return errors.New("invalid field")
		}
	}
	if !echoLinkCallPattern.MatchString(values["call"]) || strings.TrimSpace(values["name"]) == "" ||
		!echoLinkContextPattern.MatchString(values["context"]) {
		return errors.New("invalid identity or context")
	}
	if value := values["email"]; value != "" && (!strings.Contains(value, "@") || strings.ContainsAny(value, " \t")) {
		return errors.New("invalid email")
	}
	for _, rule := range []struct {
		key      string
		min, max float64
		integer  bool
	}{
		{"node", 1, 9999999, true}, {"lat", -90, 90, false}, {"lon", -180, 180, false},
		{"freq", 0, 100000, false}, {"tone", 0, 1000, false},
		{"power", 0, 9, true}, {"height", 0, 9, true}, {"gain", 0, 9, true}, {"dir", 0, 8, true},
		{"maxstns", 1, 1000, true}, {"rtcptimeout", 1, 3600, true},
	} {
		value, err := strconv.ParseFloat(values[rule.key], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < rule.min ||
			value > rule.max || (rule.integer && math.Trunc(value) != value) {
			return errors.New("invalid numeric field")
		}
	}
	if file := values["recfile"]; file != "" && !filepath.IsAbs(file) {
		return errors.New("recording path must be absolute")
	}
	serverCount := 0
	for _, key := range []string{"server1", "server2", "server3", "server4"} {
		if value := values[key]; value != "" {
			if !echoLinkHostPattern.MatchString(value) || strings.Contains(value, "..") {
				return errors.New("invalid server")
			}
			serverCount++
		}
	}
	if serverCount == 0 {
		return errors.New("at least one server required")
	}
	return nil
}

func (p echoLinkConfigControl) save(c echo.Context) error {
	var req echoLinkRequest
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, 32<<10)
	if err := c.Bind(&req); err != nil {
		return echoLinkAPIError(c, 400, "ECHOLINK_VALIDATION_ERROR", err)
	}
	if err := validateEchoLinkValues(req.Values); err != nil ||
		!validSIPValues(req.Password) || len(req.Password) > 128 {
		return echoLinkAPIError(c, 400, "ECHOLINK_VALIDATION_ERROR", errors.New("invalid request"))
	}
	if !systemControlLock.TryLock() {
		return echoLinkAPIError(c, 409, "ECHOLINK_BUSY", nil)
	}
	defer systemControlLock.Unlock()
	node, err := p.primary.primary()
	if err != nil {
		return echoLinkAPIError(c, 409, "ECHOLINK_PRIMARY_ERROR", err)
	}
	if req.AstNode != node {
		return echoLinkAPIError(c, 409, "ECHOLINK_STALE", nil)
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return echoLinkAPIError(c, 500, "ECHOLINK_READ_ERROR", err)
	}
	if echoLinkRevision(data) != req.Revision {
		return echoLinkAPIError(c, 409, "ECHOLINK_STALE", nil)
	}
	if _, err := p.sections(string(data)); err != nil {
		return echoLinkAPIError(c, 409, "ECHOLINK_STRUCTURE_ERROR", err)
	}
	index := -1
	for i, section := range parseManagerSections(string(data)) {
		if section.name == req.Section {
			index = i
			if req.Password == "" && sipOption(section, "pwd", "") == "" {
				return echoLinkAPIError(c, 400, "ECHOLINK_PASSWORD_REQUIRED", nil)
			}
		}
	}
	if index < 0 || !echoLinkSectionPattern.MatchString(req.Section) {
		return echoLinkAPIError(c, 400, "ECHOLINK_STRUCTURE_ERROR", nil)
	}
	options := []sipUserOption{{key: "astnode", values: []string{node}}}
	for _, key := range echoLinkKeys {
		options = append(options, sipUserOption{key: key, values: []string{req.Values[key]}})
	}
	if req.Password != "" {
		options = append(options, sipUserOption{key: "pwd", values: []string{req.Password}})
	}
	updated := rewriteSIPSections(string(data), map[int][]sipUserOption{index: options}, nil)
	if err := writePrimaryBackup(p.path, string(data)); err != nil {
		return echoLinkAPIError(c, 500, "ECHOLINK_BACKUP_ERROR", err)
	}
	if err := writePrimaryFile(p.path, updated); err != nil {
		return echoLinkAPIError(c, 500, "ECHOLINK_SAVE_ERROR", err)
	}
	log.Printf("echolink: configuración guardada sección=%s astnode=%s", req.Section, node)
	return c.JSON(200, echo.Map{"status": "ok", "revision": echoLinkRevision([]byte(updated))})
}

func (p echoLinkConfigControl) activate(c echo.Context) error {
	var req struct {
		Confirmation string `json:"confirmation"`
		Persist      bool   `json:"persist"`
		Action       string `json:"action"`
	}
	if err := c.Bind(&req); err != nil || req.Confirmation != "APPLY ECHOLINK" ||
		(req.Action != "enable" && req.Action != "disable" && req.Action != "reload") ||
		(req.Action == "reload" && req.Persist) {
		return echoLinkAPIError(c, 400, "ECHOLINK_CONFIRM_REQUIRED", nil)
	}
	if !systemControlLock.TryLock() {
		return echoLinkAPIError(c, 409, "ECHOLINK_BUSY", nil)
	}
	defer systemControlLock.Unlock()
	loaded, err := p.loaded()
	if err != nil {
		return echoLinkAPIError(c, 502, "ECHOLINK_STATUS_ERROR", err)
	}
	if req.Persist {
		data, err := os.ReadFile(p.modulesPath)
		if err != nil {
			return echoLinkAPIError(c, 500, "ECHOLINK_MODULES_ERROR", err)
		}
		updated, err := echoLinkModuleConfig(string(data), req.Action == "enable")
		if err != nil {
			return echoLinkAPIError(c, 409, "ECHOLINK_MODULES_ERROR", err)
		}
		if updated != string(data) {
			if err := writePrimaryBackup(p.modulesPath, string(data)); err != nil {
				return echoLinkAPIError(c, 500, "ECHOLINK_BACKUP_ERROR", err)
			}
			if err := writePrimaryFile(p.modulesPath, updated); err != nil {
				return echoLinkAPIError(c, 500, "ECHOLINK_MODULES_ERROR", err)
			}
		}
	}
	command := "module load chan_echolink.so"
	switch req.Action {
	case "disable":
		command = "module unload -f chan_echolink.so"
	case "reload":
		if !loaded {
			return echoLinkAPIError(c, 409, "ECHOLINK_NOT_LOADED", nil)
		}
		command = "module reload chan_echolink.so"
	}
	wantLoaded := req.Action != "disable"
	if loaded != wantLoaded || req.Action == "reload" {
		output, err := p.primary.ami(command)
		lower := strings.ToLower(output)
		confirmed := strings.Contains(lower, "successfully")
		if req.Action == "disable" || req.Action == "enable" {
			expected := "Loaded chan_echolink.so"
			if req.Action == "disable" {
				expected = "Unloaded chan_echolink.so"
			}
			for _, line := range strings.Split(output, "\n") {
				if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Output: ")) == expected {
					confirmed = true
				}
			}
		}
		if err != nil || strings.Contains(lower, "unable") || strings.Contains(lower, "failed") ||
			strings.Contains(lower, "no such") || strings.Contains(lower, "not support") ||
			!confirmed {
			return echoLinkAPIError(c, 502, "ECHOLINK_ACTIVATE_ERROR", errors.New("module change unconfirmed"))
		}
	}
	active, err := p.loaded()
	if err != nil || active != wantLoaded {
		return echoLinkAPIError(c, 502, "ECHOLINK_ACTIVATE_ERROR", errors.New("module status mismatch"))
	}
	log.Printf("echolink: módulo action=%s permanente=%t", req.Action, req.Persist)
	return c.JSON(200, echo.Map{"status": "ok", "loaded": active})
}

func echoLinkModuleConfig(content string, enabled bool) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return "", errors.New("module includes need manual review")
		}
	}
	sections := parseManagerSections(content)
	index := -1
	for i, section := range sections {
		if section.name != "modules" {
			continue
		}
		if index >= 0 || !plainSIPSection(section) {
			return "", errors.New("ambiguous modules section")
		}
		index = i
	}
	if index < 0 {
		return "", errors.New("missing modules section")
	}
	key := "noload"
	if enabled {
		key = "load"
	}
	lines := sections[index].lines
	updated := make([]string, 0, len(lines)+1)
	found := false
	for i, line := range lines {
		if i > sections[index].start && i < sections[index].end {
			match := managerOptionPattern.FindStringSubmatchIndex(line)
			if match != nil {
				option := strings.ToLower(line[match[2]:match[3]])
				value := strings.TrimSpace(line[match[4]:match[5]])
				if (option == "load" || option == "noload") && strings.EqualFold(value, "chan_echolink.so") {
					if !found {
						updated = append(updated, line[:match[2]]+key+line[match[3]:])
						found = true
					}
					continue
				}
			}
		}
		updated = append(updated, line)
	}
	if !found {
		// With no existing directive, insert one immediately after the section header.
		at := sections[index].start + 1
		updated = append(updated[:at], append([]string{key + " => chan_echolink.so"}, updated[at:]...)...)
	}
	output := strings.Join(updated, "\n")
	if strings.Contains(content, "\r\n") {
		output = strings.ReplaceAll(output, "\n", "\r\n")
	}
	return output, nil
}
