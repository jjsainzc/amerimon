package controllers

import (
	"amerimon/internal/config"
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var allmonSearchDirs = []string{"/etc/allmon3", "/var/www/html/supermon"}

// allmonOwnPath devuelve la ruta propia configurada o la ruta predeterminada de Amerimon.
func allmonOwnPath() string {
	if p := strings.TrimSpace(config.App.Sources.AllmonINIOwnPath); p != "" {
		return p
	}
	return "/etc/amerimon/allmon.ini"
}

// allmonINIPath devuelve el allmon.ini a usar: el forzado por config, el primero legible
// en /etc/allmon3 o /var/www/html/supermon, o el propio de /etc/amerimon.
func allmonINIPath() string {
	if p := strings.TrimSpace(config.App.Sources.AllmonINIPath); p != "" {
		return p
	}
	for _, dir := range allmonSearchDirs {
		p := filepath.Join(dir, "allmon.ini")
		if f, err := os.Open(p); err == nil {
			f.Close()
			return p
		}
	}
	return allmonOwnPath()
}

type allmonInstance struct {
	Node   string
	Host   string
	Port   string
	User   string
	Secret string
}

// readAllmonInstances analiza las secciones de nodos válidas de allmon.ini y devuelve errores de lectura.
func readAllmonInstances(path string) ([]allmonInstance, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	instances := make([]allmonInstance, 0)
	seen := make(map[string]struct{})
	section := ""
	values := make(map[string]string)
	storeSection := func() {
		if !isNumericNode(section) || !strings.EqualFold(values["system"], "Nodes") || strings.EqualFold(values["menu"], "no") {
			return
		}
		host := strings.TrimSpace(values["host"])
		user := strings.TrimSpace(values["user"])
		secret := strings.TrimSpace(values["passwd"])
		if host == "" || user == "" {
			return
		}
		if _, exists := seen[section]; exists {
			return
		}
		port := strings.TrimSpace(values["port"])
		if port == "" {
			port = "5038"
		}
		if splitHost, splitPort, err := net.SplitHostPort(host); err == nil {
			host, port = splitHost, splitPort
		} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		}
		if parsedPort, err := strconv.Atoi(port); err != nil || parsedPort < 1 || parsedPort > 65535 {
			return
		}
		seen[section] = struct{}{}
		instances = append(instances, allmonInstance{
			Node: section, Host: host, Port: port, User: user, Secret: secret,
		})
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			storeSection()
			sectionEnd := strings.IndexByte(line, ']')
			if sectionEnd < 0 {
				section = ""
			} else {
				section = strings.TrimSpace(line[1:sectionEnd])
			}
			values = make(map[string]string)
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	storeSection()
	return instances, nil
}

// loadAllmonInstances carga las instancias configuradas y completa credenciales AMI compatibles; el booleano indica si allmon.ini está configurado.
func loadAllmonInstances() ([]allmonInstance, bool, error) {
	path := allmonINIPath()
	if path == "" {
		return nil, false, nil
	}
	instances, err := readAllmonInstances(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err == nil && len(instances) == 0 {
		return nil, false, nil
	}
	globalAMIPort := strings.TrimSpace(config.App.AMI.Port)
	if globalAMIPort == "" {
		globalAMIPort = "5038"
	}
	for i := range instances {
		if strings.TrimSpace(instances[i].Secret) != "" || strings.TrimSpace(config.App.AMI.Secret) == "" {
			continue
		}
		if strings.EqualFold(instances[i].Host, strings.TrimSpace(config.App.AMI.Host)) &&
			instances[i].Port == globalAMIPort &&
			strings.EqualFold(instances[i].User, strings.TrimSpace(config.App.AMI.User)) {
			instances[i].Secret = config.App.AMI.Secret
		}
	}
	if err != nil {
		return nil, true, err
	}
	instances, err = reconcileLocalAllmonInstances(instances)
	return instances, true, err
}

// reconcileLocalAllmonInstances toma los nodos locales de rpt.conf y conserva los servidores remotos de Allmon.
func reconcileLocalAllmonInstances(instances []allmonInstance) ([]allmonInstance, error) {
	ami := config.App.AMI
	path := strings.TrimSpace(ami.RPTConfigPath)
	if path == "" {
		path = "/etc/asterisk/rpt.conf"
	}
	primary, err := readPrimaryRPTNode(path)
	if err != nil {
		return nil, fmt.Errorf("leer nodo principal de rpt.conf: %w", err)
	}
	nodes, err := readLocalRPTNodes(path)
	if err != nil {
		return nil, err
	}
	if primary != "" {
		nodes = append([]string{primary}, nodes...)
	}
	port := strings.TrimSpace(ami.Port)
	if port == "" {
		port = "5038"
	}
	resolved := make([]allmonInstance, 0, len(nodes)+len(instances))
	local := make(map[string]bool)
	for _, node := range nodes {
		if local[node] {
			continue
		}
		local[node] = true
		resolved = append(resolved, allmonInstance{
			Node: node, Host: ami.Host, Port: port, User: ami.User, Secret: ami.Secret,
		})
	}
	for _, instance := range instances {
		ip := net.ParseIP(instance.Host)
		isLocal := strings.EqualFold(instance.Host, "localhost") ||
			(ip != nil && ip.IsLoopback()) ||
			strings.EqualFold(instance.Host, strings.TrimSpace(ami.Host))
		if isLocal {
			continue
		}
		if !local[instance.Node] {
			resolved = append(resolved, instance)
		}
	}
	return resolved, nil
}

// findAllmonInstance busca un nodo local y devuelve si hay configuración Allmon y cualquier error de lectura.
func findAllmonInstance(node string) (allmonInstance, bool, error) {
	instances, configured, err := loadAllmonInstances()
	if err != nil || !configured {
		return allmonInstance{}, configured, err
	}
	for _, instance := range instances {
		if instance.Node == node {
			return instance, true, nil
		}
	}
	return allmonInstance{}, true, nil
}
