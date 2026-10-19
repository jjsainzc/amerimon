package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

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
	if len(controllers.JWTSecret()) == 0 {
		log.Printf("secreto JWT no encontrado: las rutas privadas rechazarán todos los tokens")
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()
	config.App.DB = database
	config.App.UseDB = true

	if *resetAdmin {
		if err := controllers.ClearAdminPassword(database); err != nil {
			log.Fatal(err)
		}
		fmt.Println("Contraseña de admin borrada: defina una nueva desde la web.")
		return
	}
	if ok, err := controllers.AdminConfigured(database); err != nil {
		log.Fatalf("admin: %v", err)
	} else if !ok {
		log.Printf("contraseña de admin sin definir: se pedirá al primer ingreso desde la web")
	}

	controllers.StartAllStarLinkActivityScheduler()

	audioManager := config.NewAudioStreamManager()
	go config.StartAudioListener(fmt.Sprintf("127.0.0.1:%d", config.App.AMI.ListenPort), audioManager)

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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("amerimon %s listening on %s (db: %s)", version, *addr, *dbPath)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
