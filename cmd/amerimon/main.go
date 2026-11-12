package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"amerimon/internal/config"
	"amerimon/internal/controllers"
	"amerimon/internal/db"
	"amerimon/internal/routes"
	"amerimon/internal/web"

	"github.com/labstack/echo/v4"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// env devuelve el valor no vacío de una variable de entorno o, si no existe, def.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func ObtenerContenidoURL(url string) (string, error) {
	// 1. Realizar la petición HTTP GET
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("error al realizar la petición: %w", err)
	}
	defer resp.Body.Close()

	// 2. Verificar que el código de estado sea exitoso (200 OK)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error en la respuesta HTTP: estado %d", resp.StatusCode)
	}

	// 3. Leer el contenido del cuerpo
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error al leer el contenido: %w", err)
	}

	// 4. Convertir los bytes a string y retornar
	return string(bodyBytes), nil
}

// SortNU procesa un texto multilínea en memoria simulando "sort -nu"
func SortNU(input string) (string, error) {
	// 1. Dividir el texto en líneas
	lines := strings.Split(input, "\n")

	// Mapa para almacenar números únicos (evita duplicados de forma eficiente)
	uniqueNumbers := make(map[int]bool)
	var nonNumericLines []string // Por si hay líneas que no son números puros

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue // Omitir líneas vacías si es necesario
		}

		// Intentar convertir la línea a entero (equivalente al orden numérico -n)
		num, err := strconv.Atoi(trimmed)
		if err != nil {
			// Si la línea no es un número (ej. texto plano o metadatos),
			// puedes decidir guardarla o ignorarla. Aquí la guardamos al final.
			nonNumericLines = append(nonNumericLines, trimmed)
			continue
		}

		uniqueNumbers[num] = true
	}
	// 2. Extraer las claves del mapa y ordenarlas numéricamente
	var sortedNums []int
	for num := range uniqueNumbers {
		sortedNums = append(sortedNums, num)
	}
	sort.Ints(sortedNums)

	// 3. Reconstruir el texto de salida
	var resultBuilder strings.Builder
	for _, num := range sortedNums {
		resultBuilder.WriteString(strconv.Itoa(num))
		resultBuilder.WriteByte('\n')
	}

	// (Opcional) Si tenías líneas no numéricas, las puedes agregar al final
	for _, line := range nonNumericLines {
		resultBuilder.WriteString(line)
		resultBuilder.WriteByte('\n')
	}

	return resultBuilder.String(), nil
}

// main configura la aplicación, inicia el servidor HTTP y coordina su apagado ordenado.
func main() {
	addr := flag.String("addr", env("AMERIMON_ADDR", "127.0.0.1:8082"), "listen address")
	dbPath := flag.String("db", env("AMERIMON_DB", "amerimon.db"), "sqlite database path")
	cfgPath := flag.String("config", env("AMERIMON_CONFIG", "/etc/amerimon/amerimon.yaml"), "yaml config (ami, sources)")
	showVersion := flag.Bool("version", false, "print version and exit")
	genToken := flag.Bool("gen-token", false, "print the admin token (no expiry) and exit")
	resetAdmin := flag.Bool("reset-admin", false, "clear the admin password (the web asks for a new one) and exit")
	flag.Parse()

	if *genToken {
		tok, err := controllers.GenerateAdminToken()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(tok)
		return
	}

	if *showVersion {
		fmt.Printf("amerimon %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	if err := config.Load(*cfgPath); err != nil {
		log.Fatalf("config: %v", err)
	}
	config.App.Version = version
	if err := controllers.EnsurePrivateNodesFile(); err != nil {
		log.Fatalf("private nodes: %v", err)
	}
	if len(controllers.JWTSecret()) == 0 {
		log.Printf("secreto JWT no encontrado: las rutas privadas rechazarán todos los tokens")
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()
	config.App.DB = database
	config.App.DBPath = *dbPath
	config.App.UseDB = true
	if err := controllers.RecoverAudioRecordings(); err != nil {
		log.Fatalf("audio recordings recovery: %v", err)
	}

	if *resetAdmin {
		if err := controllers.ClearAdminPassword(database); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Contraseña de admin y credenciales SSH guardadas borradas: defina una nueva contraseña desde la web.")
		return
	}
	if ok, err := controllers.AdminConfigured(database); err != nil {
		log.Fatalf("admin: %v", err)
	} else if !ok {
		log.Printf("contraseña de admin sin definir: se pedirá al primer ingreso desde la web")
	}

	// iniciar el programador de actividad de todos los nodos StarLink
	reconnectionCtx, stopReconnections := context.WithCancel(context.Background())
	reconnectionsDone := controllers.StartNodeReconnectionScheduler(reconnectionCtx)
	defer func() {
		stopReconnections()
		<-reconnectionsDone
	}()
	controllers.StartAllStarLinkActivityScheduler()
	controllers.StartNextionSupervisor(reconnectionCtx)

	// iniciar el listener de audio
	audioManager := config.NewAudioStreamManager()
	go config.StartAudioListener(fmt.Sprintf("127.0.0.1:%d", config.CurrentAMI().ListenPort), audioManager)
	controllers.StartAudioPlaybackScheduler(audioManager)

	/** Chequear que el file astdb exista en el path correcto y si no crearlo */
	rutaArchivoSupermon := "/var/www/html/supermon/astdb.txt"
	rutaArchivo := config.App.Sources.AstDBPath
	pass := false
	if info, err := os.Stat(rutaArchivo); err != nil {
		if (info != nil && info.IsDir()) || os.IsNotExist(err) {

			// buscar si el file esta en la ruta por defecto de supermon y crear un symlink si es necesario
			if rutaArchivo != rutaArchivoSupermon {
				if info, err := os.Stat(rutaArchivoSupermon); err == nil && !info.IsDir() {
					if err := os.Symlink(rutaArchivoSupermon, rutaArchivo); err != nil {
						log.Print(err.Error())
					} else {
						pass = true
					}
				}
			}

			if !pass {
				// bajar el contenido del sitio de allstarlink
				if config.App.Sources.AstDBUrl == "" {
					config.App.Sources.AstDBUrl = "https://allmondb.allstarlink.org/allmondb.php"
				}
				contenido, _ := ObtenerContenidoURL(config.App.Sources.AstDBUrl)
				if resultado, err := SortNU(contenido); err == nil {
					if err := os.WriteFile(rutaArchivo, []byte(resultado), 0644); err != nil {
						log.Print(err.Error())
					}
				}
			}
		}
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	routes.Setup(e, audioManager)
	e.Any("/*", echo.WrapHandler(web.Handler()))

	srv := &http.Server{
		Addr:              *addr,
		Handler:           e,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stopReconnections()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("amerimon %s listening on %s (db: %s)", version, *addr, *dbPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
