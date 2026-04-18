// Command forge is the entry point for the DavzyVault.
//
// Usage:
//   forge --config=configs/dev.yaml
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/DMT123/davzy-vault/internal/config"
	"github.com/DMT123/davzy-vault/internal/server"
	"github.com/DMT123/davzy-vault/internal/sources"
	"github.com/DMT123/davzy-vault/internal/sources/memories"
	"github.com/DMT123/davzy-vault/internal/sources/obsidian"
	"github.com/DMT123/davzy-vault/internal/sources/workspace"
)

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", "configs/dev.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		slog.Error("config load failed", slog.Any("err", err))
		os.Exit(1)
	}

	// Wire logger
	var logLevel slog.Level
	_ = logLevel.UnmarshalText([]byte(cfg.Logging.Level))
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	// Build sources from config
	var srcs []sources.Source
	for _, sc := range cfg.Sources {
		if !sc.Enabled {
			continue
		}
		switch sc.Type {
		case "workspace":
			root := sc.Options["root"]
			if root == "" {
				logger.Warn("workspace source missing 'root' option", slog.String("name", sc.Name))
				continue
			}
			src, err := workspace.New(root)
			if err != nil {
				logger.Warn("workspace source init failed", slog.String("name", sc.Name), slog.Any("err", err))
				continue
			}
			srcs = append(srcs, src)
			logger.Info("mounted source", slog.String("name", sc.Name), slog.String("type", sc.Type))
		case "obsidian":
			vault := sc.Options["vault"]
			if vault == "" {
				logger.Warn("obsidian source missing 'vault' option", slog.String("name", sc.Name))
				continue
			}
			src, err := obsidian.New(sc.Name, vault)
			if err != nil {
				logger.Warn("obsidian source init failed", slog.String("name", sc.Name), slog.Any("err", err))
				continue
			}
			srcs = append(srcs, src)
			logger.Info("mounted source", slog.String("name", sc.Name), slog.String("type", sc.Type))
		case "memories":
			root := sc.Options["root"]
			if root == "" {
				logger.Warn("memories source missing 'root' option", slog.String("name", sc.Name))
				continue
			}
			src, err := memories.New(sc.Name, root)
			if err != nil {
				logger.Warn("memories source init failed", slog.String("name", sc.Name), slog.Any("err", err))
				continue
			}
			srcs = append(srcs, src)
			logger.Info("mounted source", slog.String("name", sc.Name), slog.String("type", sc.Type))
		default:
			logger.Warn("unknown source type", slog.String("name", sc.Name), slog.String("type", sc.Type))
		}
	}

	if len(srcs) == 0 {
		logger.Error("no sources enabled — refusing to start")
		os.Exit(1)
	}

	// Graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	forge := server.New(cfg, srcs, logger)
	logger.Info("DavzyVault starting",
		slog.String("transport", cfg.Server.Transport),
		slog.Int("sources", len(srcs)),
	)
	if err := forge.Run(ctx); err != nil {
		logger.Error("server exited with error", slog.Any("err", err))
		os.Exit(1)
	}
}
