package controllers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestRestartAsteriskRecovery(t *testing.T) {
	for _, tc := range []struct {
		name         string
		restartFails bool
		needsKill    bool
		startFails   bool
		amiFails     bool
	}{
		{name: "normal"},
		{name: "term-recovery", restartFails: true},
		{name: "kill-recovery", restartFails: true, needsKill: true},
		{name: "failed-start", restartFails: true, startFails: true},
		{name: "ami-failure", amiFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var commands []string
			stopped := false
			control := systemControl{
				pause: func(time.Duration) {},
				ami: func() error {
					if tc.amiFails {
						return errors.New("AMI unavailable")
					}
					return nil
				},
				run: func(_ context.Context, _ time.Duration, name string, args ...string) (string, error) {
					command := name + " " + strings.Join(args, " ")
					commands = append(commands, command)
					switch command {
					case "systemctl restart asterisk.service":
						if tc.restartFails {
							return "", errors.New("restart failed")
						}
					case "systemctl kill --kill-whom=all --signal=SIGTERM asterisk.service":
						stopped = !tc.needsKill
					case "systemctl kill --kill-whom=all --signal=SIGKILL asterisk.service":
						stopped = true
					case "systemctl show asterisk.service --property=ActiveState --value":
						if stopped {
							return "inactive", nil
						}
						return "deactivating", nil
					case "systemctl start asterisk.service":
						if !stopped {
							t.Fatal("start called before old service stopped")
						}
						if tc.startFails {
							return "", errors.New("start failed")
						}
					case "systemctl is-active asterisk.service":
						return "active", nil
					}
					return "", nil
				},
			}
			result := systemControlResult{}
			err := control.restartAsterisk(context.Background(), &result)
			if (err != nil) != (tc.startFails || tc.amiFails) {
				t.Fatalf("unexpected error: %v", err)
			}
			all := strings.Join(commands, "\n")
			if strings.Contains(all, "SIGKILL") != tc.needsKill {
				t.Fatalf("unexpected forced kill: %s", all)
			}
			if strings.Contains(all, "SIGTERM") != tc.restartFails {
				t.Fatalf("unexpected recovery: %s", all)
			}
			if result.Recovered != (tc.restartFails && !tc.startFails) {
				t.Fatalf("unexpected recovered flag: %+v", result)
			}
		})
	}
}

func TestSystemControlAuthorizationAndConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, role, body string
		want             int
	}{
		{"anonymous", "", `{"action":"linux_reboot","confirmation":"REINICIAR LINUX"}`, 401},
		{"non-admin", "U", `{"action":"linux_reboot","confirmation":"REINICIAR LINUX"}`, 403},
		{"unconfirmed", "A", `{"action":"linux_reboot"}`, 400},
		{"arbitrary-command", "A", `{"action":"rm","confirmation":"rm"}`, 400},
		{"invalid-json", "A", `{`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control := systemControl{
				run: func(context.Context, time.Duration, string, ...string) (string, error) {
					t.Fatal("unauthorized or invalid operation executed")
					return "", nil
				},
			}
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/mon/system/control", strings.NewReader(tc.body))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if tc.role != "" {
				c.Set("claims", &CustomClaims{Role: tc.role})
			}
			if err := control.handle(c); err != nil {
				e.HTTPErrorHandler(err, c)
			}
			if rec.Code != tc.want {
				t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestLinuxActionsAreScheduledAndFailuresReported(t *testing.T) {
	for _, action := range []string{"linux_reboot", "linux_poweroff"} {
		for _, fail := range []bool{false, true} {
			t.Run(action+map[bool]string{false: "-ok", true: "-error"}[fail], func(t *testing.T) {
				confirmation, flag := "REINICIAR LINUX", "-r"
				if action == "linux_poweroff" {
					confirmation, flag = "APAGAR LINUX", "-P"
				}
				control := systemControl{
					run: func(_ context.Context, _ time.Duration, name string, args ...string) (string, error) {
						if name != "shutdown" || strings.Join(args, " ") != flag+" +1 Mantenimiento solicitado desde Amerimon" {
							t.Fatalf("unexpected command %s %v", name, args)
						}
						if fail {
							return "", errors.New("permission denied")
						}
						return "", nil
					},
				}
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"action":"`+action+`","confirmation":"`+confirmation+`"}`))
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
				rec := httptest.NewRecorder()
				c := echo.New().NewContext(req, rec)
				c.Set("claims", &CustomClaims{Role: "A"})
				if err := control.handle(c); err != nil {
					t.Fatal(err)
				}
				want := http.StatusAccepted
				if fail {
					want = http.StatusBadGateway
				}
				if rec.Code != want || strings.Contains(rec.Body.String(), `"scheduled":true`) == fail {
					t.Fatalf("wrong scheduled outcome: %d %s", rec.Code, rec.Body.String())
				}
			})
		}
	}
}

func TestSystemControlReportsJournalWarnings(t *testing.T) {
	control := systemControl{
		ami: func() error { return nil }, pause: func(time.Duration) {},
		run: func(_ context.Context, _ time.Duration, name string, args ...string) (string, error) {
			if name == "journalctl" {
				return "ERROR: No matching USB radio device", nil
			}
			if args[0] == "is-active" {
				return "active", nil
			}
			return "", nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"action":"asterisk_restart","confirmation":"REINICIAR ASTERISK"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("claims", &CustomClaims{Role: "A"})
	if err := control.handle(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "No matching USB radio device") {
		t.Fatalf("startup warning lost: %d %s", rec.Code, rec.Body.String())
	}
}
