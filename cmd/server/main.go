package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dreamreflex/service-edge/internal/api"
	"github.com/dreamreflex/service-edge/internal/api/handler"
	"github.com/dreamreflex/service-edge/internal/api/middleware"
	"github.com/dreamreflex/service-edge/internal/config"
	"github.com/dreamreflex/service-edge/internal/pki"
	"github.com/dreamreflex/service-edge/internal/service"
	"github.com/dreamreflex/service-edge/internal/store"
	"github.com/dreamreflex/service-edge/internal/web"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "gen-ca" {
		runGenCA(os.Args[2:])
		return
	}

	cfgPath := flag.String("config", "config.yaml", "path to config file")
	agentDist := flag.String("agent-dist", "", "directory of agent binaries to serve under /download")
	flag.Parse()

	setupLogging()
	slog.Info("starting service-edge control plane", "version", version)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		panic(fmt.Sprintf("load config: %v", err))
	}

	// Strict startup CA validation: refuse to start on any failure.
	ca, err := pki.LoadCA(cfg.PKI.CACert, cfg.PKI.CAKey)
	if err != nil {
		panic(fmt.Sprintf("CA validation failed (refusing to start): %v", err))
	}
	slog.Info("CA loaded and validated")

	st, err := store.Open(cfg.Database.Path)
	if err != nil {
		panic(fmt.Sprintf("open store: %v", err))
	}

	svc := service.New(st, ca, cfg)
	if err := svc.BootstrapAdmin(cfg.BootstrapAdmin.Username, cfg.BootstrapAdmin.Password); err != nil {
		panic(fmt.Sprintf("bootstrap admin: %v", err))
	}

	jwt := middleware.NewJWTManager(cfg.JWTSecret, 12*time.Hour)
	h := handler.New(svc, jwt)

	r := api.NewRouter(api.Options{
		Handler:   h,
		JWT:       jwt,
		Cfg:       cfg,
		StaticFS:  web.FS(),
		AgentDist: *agentDist,
	})

	srv := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		// No global write timeout: the long-poll config endpoint holds for 30s.
	}

	go func() {
		slog.Info("listening", "addr", cfg.Server.Listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(fmt.Sprintf("server error: %v", err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func runGenCA(args []string) {
	fs := flag.NewFlagSet("gen-ca", flag.ExitOnError)
	out := fs.String("out", "dev", "output directory for ca.crt/ca.key")
	_ = fs.Parse(args)
	certPath, keyPath, err := pki.GenerateCA(*out)
	if err != nil {
		panic(fmt.Sprintf("generate CA: %v", err))
	}
	fmt.Printf("CA generated:\n  cert: %s\n  key:  %s\n", certPath, keyPath)
}

func setupLogging() {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
}
