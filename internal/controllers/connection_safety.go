package controllers

import (
	"amerimon/internal/config"
	"context"
	"database/sql"
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
func validateRPTConnection(ctx context.Context, selected allmonInstance, remote string, desired ...SupermonCommandRequest) (int, error) {
	if selected.Node == remote {
		return http.StatusConflict, fmt.Errorf("Conexión rechazada: un nodo no puede enlazarse consigo mismo")
	}
	instances, configured, err := loadAllmonInstances()
	if err == nil && !configured {
		instances, err = reconcileLocalAllmonInstances(nil)
	}
	if err != nil {
		log.Printf("asterisk: topología incompleta; se comprobarán los enlaces disponibles: %v", err)
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
	var existing *rptConnection
	for range byNode {
		var result snapshot
		select {
		case <-ctx.Done():
			return http.StatusServiceUnavailable, fmt.Errorf("No se pudo completar la validación de topología: %w", ctx.Err())
		case result = <-snapshots:
		}
		if result.err != nil {
			log.Printf("asterisk: no se pudo consultar XStat para validar el nodo %s: %v", result.instance.Node, result.err)
			continue
		}
		links := make(rptLinkGraph)
		if err := addRPTVisibleLinks(links, result.instance.Node, result.raw); err != nil {
			log.Printf("asterisk: se omite XStat no interpretable del nodo %s: %v", result.instance.Node, err)
			continue
		}
		for local, neighbors := range links {
			for linked := range neighbors {
				graph.add(local, linked)
			}
		}
		for _, link := range parseRPTConnections(result.raw, "") {
			if link.Link == "ESTABLISHED" || link.Link == "CONNECTING" {
				observed[pendingRPTLink{rptServerKey(result.instance), result.instance.Node, link.Node}] = true
				if result.instance.Node == selected.Node && link.Node == remote {
					copy := link
					existing = &copy
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return http.StatusServiceUnavailable, fmt.Errorf("No se pudo completar la validación de topología: %w", err)
	}
	if existing != nil && len(desired) > 0 {
		request := desired[0]
		same, known, err := matchesRecordedRPTConnection(ctx, selected.Node, *existing, request)
		if err != nil {
			return http.StatusServiceUnavailable, fmt.Errorf("No se pudo verificar la modalidad del enlace existente: %w", err)
		}
		if same {
			return http.StatusConflict, fmt.Errorf("El nodo %s ya está conectado o conectándose en el servidor %s con el mismo modo y la misma permanencia solicitados. No se modificó el enlace", remote, selected.Node)
		}
		if !known {
			log.Printf("asterisk: permanencia no verificable local=%s remoto=%s; se reconectará con la modalidad solicitada", selected.Node, remote)
		}
		started := time.Now()
		if err := disconnectRPTConnections(ctx, selected, 15*time.Second, remote); err != nil {
			recordAsteriskOperation(ctx, "disconnect", selected.Node, remote, "", false, "", "error", err.Error(), started)
			return http.StatusBadGateway, fmt.Errorf("No se inició la nueva conexión: %w", err)
		}
		forgetPendingRPTLinks(selected.Node, remote)
		recordAsteriskOperation(ctx, "disconnect", selected.Node, remote, "", false,
			fmt.Sprintf("rpt cmd %s ilink 11 %s", selected.Node, remote), "success", "Cambio de modalidad: XStat confirmó la desconexión del enlace anterior", started)
		// Tras retirar el enlace anterior, se valida de nuevo la topología real completa.
		return validateRPTConnection(ctx, selected, remote)
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

func matchesRecordedRPTConnection(ctx context.Context, local string, existing rptConnection, request SupermonCommandRequest) (same, known bool, err error) {
	if config.App.DB == nil {
		return false, false, fmt.Errorf("base de datos no disponible")
	}
	var command string
	err = config.App.DB.QueryRowContext(ctx, `
		SELECT command FROM asterisk_operation_logs
		WHERE local_node = ? AND (remote_node = ? OR action = 'disconnect_before_connect')
		  AND result IN ('success', 'not_confirmed')
		  AND action IN ('connect', 'monitor', 'localmonitor', 'add_connection', 'disconnect', 'disconnect_before_connect')
		ORDER BY id DESC LIMIT 1`, local, existing.Node).Scan(&command)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	for action, mode := range map[string]string{"connect": "T", "monitor": "R", "localmonitor": "L"} {
		for _, permanent := range []bool{false, true} {
			expected := fmt.Sprintf("rpt cmd %s ilink %d %s", local, supermonConnectionCode(action, permanent), existing.Node)
			if command == expected && (existing.Mode == mode || existing.Mode == "C") && existing.Direction == "OUT" {
				return action == request.Action && permanent == request.Permanent, true, nil
			}
		}
	}
	return false, false, nil
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
