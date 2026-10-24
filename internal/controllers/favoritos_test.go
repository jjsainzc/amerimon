package controllers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"amerimon/internal/config"
	"amerimon/internal/db"

	"github.com/labstack/echo/v4"
)

func TestBuscarCoincidenciasAstDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "astdb.txt")
	content := "700002|HC1LNM|Nodo secundario|Quito\n700001|HC1LNM|Nodo principal|Quito\n700003|OTRO|Sin coincidencia|Lima\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, query := range []string{"hc1lnm", "700001", "secundario"} {
		results, err := buscarCoincidenciasAstDB(path, query)
		if err != nil {
			t.Fatal(err)
		}
		if query == "hc1lnm" {
			if len(results) != 2 {
				t.Fatalf("got %d matches for callsign, want 2", len(results))
			}
			if !reflect.DeepEqual([]string{results[0].Nodo, results[1].Nodo}, []string{"700001", "700002"}) {
				t.Fatalf("got nodes %v, want sorted matches [700001 700002]", results)
			}
			continue
		}
		if len(results) != 1 {
			t.Fatalf("query %q: got %d matches, want 1", query, len(results))
		}
	}
}

func TestBuscarCoincidenciasAstDBReturnsReadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-astdb.txt")
	if _, err := buscarCoincidenciasAstDB(path, "HC1LNM"); err == nil {
		t.Fatal("expected an error when AstDB cannot be read")
	}
}

func TestReadEchoLinkFavoritos(t *testing.T) {
	for _, test := range []struct {
		name        string
		module      string
		dump        string
		failing     string
		wantDump    bool
		wantError   bool
		wantEntries int
	}{
		{name: "module absent", module: "0 modules loaded"},
		{name: "module query fails", failing: "module show like chan_echolink.so", wantError: true},
		{name: "empty directory", module: "chan_echolink.so EchoLink Running", wantDump: true},
		{name: "valid directory", module: "chan_echolink.so EchoLink Running", wantDump: true, wantEntries: 2,
			dump: "996546|W2LRC-R|165.227.99.201\r\nOutput: 12345|HC1LNM-L|192.0.2.1\r\n996546|W2LRC-R|165.227.99.201"},
		{name: "malformed directory", module: "chan_echolink.so EchoLink Running", wantDump: true, wantError: true,
			dump: "invalid|CALL|192.0.2.1\n123||192.0.2.1\n124|CALL|bad-ip\n125|CALL|192.0.2.1|extra"},
		{name: "dump fails", module: "chan_echolink.so EchoLink Running", wantDump: true,
			failing: "echolink dbdump", wantError: true},
		{name: "unknown CLI command", module: "chan_echolink.so EchoLink Running", wantDump: true,
			dump: "No such command 'echolink dbdump'", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var commands []string
			entries, err := readEchoLinkFavoritos(func(command string) (string, error) {
				commands = append(commands, command)
				if command == test.failing {
					return "", errors.New("AMI unavailable")
				}
				if command == "module show like chan_echolink.so" {
					return "Response: Follows\r\nOutput: " + test.module + "\r\n--END COMMAND--\r\n", nil
				}
				return "Response: Follows\r\nOutput: " + test.dump + "\r\n--END COMMAND--\r\n", nil
			})
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, want error %v", err, test.wantError)
			}
			wantCommands := []string{"module show like chan_echolink.so"}
			if test.wantDump {
				wantCommands = append(wantCommands, "echolink dbdump")
			}
			if !reflect.DeepEqual(commands, wantCommands) {
				t.Fatalf("commands = %v, want %v", commands, wantCommands)
			}
			if len(entries) != test.wantEntries {
				t.Fatalf("entries = %v, want %d", entries, test.wantEntries)
			}
			if test.wantEntries > 0 {
				want := AstDBEntry{Nodo: "3996546", Callsign: "W2LRC-R", Descripcion: "EchoLink", Ubicacion: "165.227.99.201"}
				if entries[want.Nodo] != want {
					t.Fatalf("entry = %+v, want %+v", entries[want.Nodo], want)
				}
			}
		})
	}
}

func TestFavoritosEchoLinkSearchAndDescriptions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "astdb.txt")
	if err := os.WriteFile(path, []byte("700001|W2LRC-R|AllStar|Quito\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	previous := config.App
	defer func() { config.App = previous }()
	config.App.DB = database
	config.App.Sources.AstDBPath = path
	host, port := mockConnectionAMI(t, func(fields map[string]string) string {
		switch fields["Command"] {
		case "module show like chan_echolink.so":
			return "Response: Follows\r\nOutput: chan_echolink.so EchoLink Running\r\n--END COMMAND--\r\n\r\n"
		case "echolink dbdump":
			return "Response: Follows\r\nOutput: 996546|W2LRC-R|165.227.99.201\r\n--END COMMAND--\r\n\r\n"
		default:
			return "Response: Error\r\nMessage: Unexpected command\r\n\r\n"
		}
	})
	config.App.AMI.Host, config.App.AMI.Port = host, port
	config.App.AMI.User, config.App.AMI.Secret = "test", "test"

	e := echo.New()
	for _, query := range []string{"w2lrc-r", "996546", "3996546", "EchoLink", "nothing"} {
		rec := httptest.NewRecorder()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/?q="+query, nil), rec)
		if err := BuscarAstdbFavorito(c); err != nil {
			t.Fatal(err)
		}
		var entries []AstDBEntry
		if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
			t.Fatal(err)
		}
		wantCount := 1
		if query == "w2lrc-r" {
			wantCount = 2
		} else if query == "nothing" {
			wantCount = 0
			if strings.TrimSpace(rec.Body.String()) != "[]" {
				t.Fatalf("empty response = %s, want []", rec.Body.String())
			}
		}
		if len(entries) != wantCount || (wantCount > 0 && entries[0].Nodo != "3996546") {
			t.Fatalf("query %q: unexpected results %+v", query, entries)
		}
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
	c.SetParamNames("numero")
	c.SetParamValues("3996546")
	if err := AstdbFavorito(c); err != nil {
		t.Fatal(err)
	}
	var suggestion struct {
		Descripcion string `json:"descripcion"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &suggestion); err != nil {
		t.Fatal(err)
	}
	if suggestion.Descripcion != "W2LRC-R EchoLink 165.227.99.201" {
		t.Fatalf("unexpected suggestion: %+v", suggestion)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"numero":"3996546","descripcion":""}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if err := CreateFavorito(e.NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	if err := ListFavoritos(e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)); err != nil {
		t.Fatal(err)
	}
	var items []favorito
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Numero != "3996546" || items[0].DescripcionDB != suggestion.Descripcion {
		t.Fatalf("unexpected saved favorite: %+v", items)
	}

	config.App.AMI.Host, config.App.AMI.Port = mockConnectionAMI(t, func(map[string]string) string {
		return "Response: Error\r\nMessage: EchoLink unavailable\r\n\r\n"
	})
	rec = httptest.NewRecorder()
	if err := BuscarAstdbFavorito(e.NewContext(httptest.NewRequest(http.MethodGet, "/?q=W2LRC", nil), rec)); err != nil {
		t.Fatal(err)
	}
	var entries []AstDBEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Nodo != "700001" {
		t.Fatalf("AstDB search must still work without EchoLink: %+v", entries)
	}
}
