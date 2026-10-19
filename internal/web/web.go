package web

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Solo se acepta un prefijo simple (p. ej. /amerimon) para no inyectar nada en el HTML.
var rePrefijo = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)+$`)

// Handler sirve el build de Angular embebido; las rutas de la SPA devuelven index.html.
// Si el proxy envía X-Forwarded-Prefix (p. ej. /amerimon), se ajusta <base href> a ese prefijo.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServerFS(sub)
	index, _ := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		_, statErr := fs.Stat(sub, p)
		if len(index) > 0 && (p == "" || p == "index.html" || statErr != nil) {
			body := index
			if prefix := strings.TrimRight(r.Header.Get("X-Forwarded-Prefix"), "/"); rePrefijo.MatchString(prefix) {
				body = bytes.Replace(index, []byte(`<base href="/">`), []byte(`<base href="`+prefix+`/">`), 1)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(body)
			return
		}
		files.ServeHTTP(w, r)
	})
}
