package config

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDialAMI(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", " localhost "} {
		t.Run(host, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			_, port, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			conn, err := dialAMI(host, port, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if conn.RemoteAddr().String() != listener.Addr().String() {
				t.Fatalf("expected %s, got %s", listener.Addr(), conn.RemoteAddr())
			}
		})
	}
}

func TestDialAMIIPv6(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"::1", "[::1]"} {
		t.Run(host, func(t *testing.T) {
			conn, err := dialAMI(host, port, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if conn.RemoteAddr().String() != listener.Addr().String() {
				t.Fatalf("expected %s, got %s", listener.Addr(), conn.RemoteAddr())
			}
		})
	}
}

func TestDialAMIErrors(t *testing.T) {
	for _, tc := range []struct {
		host    string
		wantErr string
	}{
		{" ", "host AMI vacío"},
		{"invalid host", `resolver host AMI "invalid host"`},
	} {
		t.Run(tc.host, func(t *testing.T) {
			conn, err := dialAMI(tc.host, "5038", time.Second)
			if conn != nil {
				conn.Close()
				t.Fatal("unexpected connection")
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDialAMITimeout(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			start := time.Now()
			conn, err := dialAMI(host, "5038", -time.Second)
			if conn != nil {
				conn.Close()
				t.Fatal("unexpected connection with expired timeout")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				var netErr net.Error
				if !errors.As(err, &netErr) || !netErr.Timeout() {
					t.Fatalf("expected timeout, got %v", err)
				}
			}
			if elapsed := time.Since(start); elapsed > time.Second {
				t.Fatalf("expired connection took %v", elapsed)
			}
		})
	}
}

func TestExecuteAMIOriginate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		login    string
		response string
		wantErr  string
	}{
		{"accepted", "Success", "Success", ""},
		{"rejected-login", "Error", "", "login AMI rechazado"},
		{"rejected-originate", "Success", "Error", "originate rechazado"},
		{"closed-connection", "Success", "", "error leyendo respuesta de Originate: EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				reader := bufio.NewReader(conn)
				fmt.Fprint(conn, "Asterisk Call Manager/11.0.0\r\n")
				if _, err := readAMIMessage(reader); err != nil {
					done <- err
					return
				}
				fmt.Fprintf(conn, "Response: %s\r\nMessage: Login result\r\n\r\n", tc.login)
				if tc.login != "Success" {
					// El cliente debe cerrar sin enviar Originate tras un login rechazado.
					message, err := reader.ReadString('\n')
					if err == nil || message != "" {
						done <- fmt.Errorf("Originate enviado tras login rechazado: %q", message)
						return
					}
					done <- nil
					return
				}
				message, err := readAMIMessage(reader)
				if err != nil {
					done <- err
					return
				}
				actionID := ""
				for _, line := range strings.Split(message, "\n") {
					if value, ok := strings.CutPrefix(strings.TrimSpace(line), "ActionID: "); ok {
						actionID = value
					}
				}
				if actionID == "" || !strings.Contains(message, "Action: Originate") {
					done <- fmt.Errorf("Originate sin ActionID: %q", message)
					return
				}
				if !strings.Contains(message, "Variable: __AMERIMON_RPT_CONFIG=/etc/asterisk/rpt.conf\r\n") {
					done <- fmt.Errorf("missing inherited config path in Originate")
					return
				}
				if tc.response != "" {
					fmt.Fprint(conn, "Event: Unrelated\r\nActionID: other\r\n\r\n")
					fmt.Fprintf(conn, "Response: %s\r\nActionID: %s\r\nMessage: Originate result\r\n\r\n", tc.response, actionID)
				}
				done <- nil
			}()
			host, port, _ := net.SplitHostPort(listener.Addr().String())
			err = ExecuteAMIOriginate(host, port, "test-user", "test-secret", "Local/nodo@listen-live-spy/n", "listen-live-eagi", "test", 2*time.Second,
				AMIOriginateVariable{Name: "__AMERIMON_RPT_CONFIG", Value: "/etc/asterisk/rpt.conf"})
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}

			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExecuteAMIOriginateRejectsVariableInjection(t *testing.T) {
	for _, variable := range []AMIOriginateVariable{
		{Name: "NAME\r\nAction: Hangup", Value: "value"},
		{Name: "NAME", Value: "value\r\nAction: Hangup"},
	} {
		err := ExecuteAMIOriginate("", "", "", "", "", "", "", time.Second, variable)
		if err == nil || !strings.Contains(err.Error(), "variable Originate inválida") {
			t.Fatalf("expected variable validation before connecting, got %v", err)
		}
	}
}
