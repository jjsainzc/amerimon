package controllers

import (
	"amerimon/internal/config"
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type pendingRPTLink struct {
	server, local, remote string
}

var rptConnectionSafety = struct {
	sync.Mutex
	pending map[pendingRPTLink]time.Time
}{pending: make(map[pendingRPTLink]time.Time)}

type rptLinkGraph map[string]map[string]bool

func (g rptLinkGraph) add(local, remote string) {
	if g[local] == nil {
		g[local] = make(map[string]bool)
	}
	if g[remote] == nil {
		g[remote] = make(map[string]bool)
	}
	g[local][remote], g[remote][local] = true, true
}

func (g rptLinkGraph) path(origin, destination string) []string {
	queue := []string{origin}
	parents := map[string]string{origin: ""}
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if node == destination {
			path := []string{node}
			for node != origin {
				node = parents[node]
				path = append(path, node)
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		neighbors := make([]string, 0, len(g[node]))
		for neighbor := range g[node] {
			neighbors = append(neighbors, neighbor)
		}
		sort.Strings(neighbors)
		for _, neighbor := range neighbors {
			if _, seen := parents[neighbor]; !seen {
				parents[neighbor] = node
				queue = append(queue, neighbor)
			}
		}
	}
	return nil
}

func addRPTVisibleLinks(graph rptLinkGraph, local, raw string) error {
	hasStatus := false
	for _, line := range config.ParseAMIOutputToLines(raw) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "Conn:":
			if len(fields) < 2 || !isValidRemoteNodeID(fields[1]) {
				return fmt.Errorf("XStat contiene un enlace inválido en %s", local)
			}
			hasStatus = true
		case "LinkedNodes:":
			hasStatus = true
			if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "LinkedNodes:")) == "<NONE>" {
				continue
			}
			for _, linked := range strings.Split(strings.Join(fields[1:], ""), ",") {
				if linked == "" {
					continue
				}
				if len(linked) < 2 || !isValidRemoteNodeID(linked[1:]) {
					return fmt.Errorf("XStat contiene una ruta inválida en %s", local)
				}
				// LinkedNodes anuncia alcanzabilidad, no necesariamente un salto físico.
				graph.add(local, linked[1:])
			}
		}
	}
	if !hasStatus {
		return fmt.Errorf("XStat no devolvió la lista de enlaces de %s", local)
	}
	for _, link := range parseRPTConnections(raw, "") {
		if link.Link == "ESTABLISHED" || link.Link == "CONNECTING" {
			graph.add(local, link.Node)
		}
	}
	return nil
}

// El llamador mantiene rptConnectionSafety bloqueado hasta terminar la operación AMI.
func validateRPTConnection(ctx context.Context, selected allmonInstance, remote string) (int, error) {
	if selected.Node == remote {
		return http.StatusConflict, fmt.Errorf("Conexión rechazada: un nodo no puede enlazarse consigo mismo")
	}
	instances, configured, err := loadAllmonInstances()
	if err == nil && !configured {
		instances, err = reconcileLocalAllmonInstances(nil)
	}
	if err != nil {
		return http.StatusServiceUnavailable, fmt.Errorf("No se pudo validar la topología visible: %w", err)
	}
	// También se comprueba el origen cuando se usa la AMI global sin Allmon.
	byNode := map[string]allmonInstance{selected.Node: selected}
	for _, instance := range instances {
		byNode[instance.Node] = instance
	}
	graph := make(rptLinkGraph)
	type snapshot struct {
		instance allmonInstance
		raw      string
		err      error
	}
	snapshots := make(chan snapshot, len(byNode))
	for _, instance := range byNode {
		go func(instance allmonInstance) {
			if err := ctx.Err(); err != nil {
				snapshots <- snapshot{instance: instance, err: err}
				return
			}
			raw, err := config.ExecuteAMIRptStatus(instance.Host, instance.Port, instance.User, instance.Secret, instance.Node, "XStat", 5*time.Second)
			snapshots <- snapshot{instance, raw, err}
		}(instance)
	}
	observed := make(map[pendingRPTLink]bool)
	for range byNode {
		var result snapshot
		select {
		case <-ctx.Done():
			return http.StatusServiceUnavailable, fmt.Errorf("No se pudo completar la validación de topología: %w", ctx.Err())
		case result = <-snapshots:
		}
		if result.err != nil {
			log.Printf("asterisk: no se pudo consultar XStat para validar el nodo %s: %v", result.instance.Node, result.err)
			return http.StatusServiceUnavailable, fmt.Errorf("No se pudo validar la topología: no se pudieron consultar los enlaces del nodo %s", result.instance.Node)
		}
		if err := addRPTVisibleLinks(graph, result.instance.Node, result.raw); err != nil {
			return http.StatusServiceUnavailable, err
		}
		for _, link := range parseRPTConnections(result.raw, "") {
			if link.Link == "ESTABLISHED" || link.Link == "CONNECTING" {
				observed[pendingRPTLink{rptServerKey(result.instance), result.instance.Node, link.Node}] = true
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return http.StatusServiceUnavailable, fmt.Errorf("No se pudo completar la validación de topología: %w", err)
	}
	now := time.Now()
	for link, expires := range rptConnectionSafety.pending {
		instance, visible := byNode[link.local]
		if !visible || link.server != rptServerKey(instance) || !expires.After(now) || observed[link] {
			delete(rptConnectionSafety.pending, link)
			continue
		}
		graph.add(link.local, link.remote)
	}
	if path := graph.path(selected.Node, remote); len(path) > 0 {
		return http.StatusConflict, fmt.Errorf("Conexión rechazada para evitar un bucle: ya existe una ruta visible o pendiente entre los nodos (%s)", strings.Join(path, " -> "))
	}
	return 0, nil
}

func rptServerKey(instance allmonInstance) string {
	return instance.Host + ":" + instance.Port
}

func rememberPendingRPTLink(instance allmonInstance, remote string, timeout time.Duration) {
	rptConnectionSafety.pending[pendingRPTLink{rptServerKey(instance), instance.Node, remote}] = time.Now().Add(timeout)
}

func forgetPendingRPTLinks(local, remote string) {
	for link := range rptConnectionSafety.pending {
		if (link.local == local && (remote == "" || link.remote == remote)) ||
			(link.remote == local && (remote == "" || link.local == remote)) {
			delete(rptConnectionSafety.pending, link)
		}
	}
}
