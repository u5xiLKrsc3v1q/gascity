// Package main is the entry point for the gascity application.
// gascity is a fork of gastownhall/gascity, providing gas price tracking
// and analysis for EVM-compatible blockchain networks.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gascity/gascity/internal/config"
	"github.com/gascity/gascity/internal/server"
)

var (
	// Version is set at build time via -ldflags.
	Version = "dev"
	// Commit is the git commit hash set at build time.
	Commit = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting gascity", "version", Version, "commit", Commit)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer cancel()

	srv, err := server.New(cfg, logger)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	if err := srv.Start(ctx); err != nil {
		slog.Error("server exited with error", "error", err)
		os.Exit(1)
	}

	fmt.Println("gascity shutdown complete")
}
