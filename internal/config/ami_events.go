package config

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"
)

// StreamAMIEvents mantiene una sesión de eventos con heartbeat y cierre al cancelar.
func StreamAMIEvents(ctx context.Context, host, port, user, secret string, receive func(map[string]string)) error {
	conn, err := dialAMI(host, port, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	if _, err := reader.ReadString('\n'); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(conn, "Action: Login\r\nUsername: %s\r\nSecret: %s\r\nEvents: call,dialplan\r\n\r\n", user, secret); err != nil {
		return err
	}
	response, err := readAMIMessage(reader)
	if err != nil {
		return err
	}
	if !strings.Contains(response, "Response: Success") {
		return fmt.Errorf("login AMI de eventos rechazado")
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
					conn.Close()
					return
				}
				if _, err := fmt.Fprint(conn, "Action: Ping\r\nActionID: sip-activity-ping\r\n\r\n"); err != nil {
					conn.Close()
					return
				}
			}
		}
	}()
	for {
		if err := conn.SetReadDeadline(time.Now().Add(35 * time.Second)); err != nil {
			return err
		}
		message, err := readAMIMessage(reader)
		if err != nil {
			return err
		}
		fields := make(map[string]string)
		for _, line := range strings.Split(message, "\n") {
			key, value, found := strings.Cut(line, ":")
			if found {
				fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
		if fields["Event"] != "" {
			receive(fields)
		}
	}
}
