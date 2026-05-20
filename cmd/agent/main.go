package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dreamreflex/service-edge/internal/agent"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", "/opt/service-edge/agent.yaml", "path to agent.yaml")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("service-edge-agent", version)
		return
	}

	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))

	cfg, err := agent.LoadConfig(*cfgPath)
	if err != nil {
		slog.Error("failed to load agent config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		slog.Info("signal received, shutting down agent")
		cancel()
	}()

	agent.NewRunner(cfg).Run(ctx)
}
