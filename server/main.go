package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"ts/tlsutil"
)

const version = "v1.0.0"

type ServerConfig struct {
	TunnelPort string `json:"tunnel_port"`
	HTTPPort   string `json:"http_port"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	TimeoutSec int    `json:"timeout_sec"`
	LogLevel   string `json:"log_level"`
	Password   string `json:"password"`
}

func main() {
	configFile := flag.String("config", "server_config.json", "path to server config file (json)")
	flag.Parse()

	// read config file (required)
	data, err := os.ReadFile(*configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read config failed:", err)
		os.Exit(1)
	}
	var cfg ServerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "parse config failed:", err)
		os.Exit(1)
	}

	// apply defaults
	if cfg.TunnelPort == "" {
		cfg.TunnelPort = "9000"
	}
	if cfg.HTTPPort == "" {
		cfg.HTTPPort = "9001"
	}
	if cfg.TimeoutSec == 0 {
		cfg.TimeoutSec = 60
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	logger := newLogger(cfg.LogLevel)
	logger.Info("ts-server starting", "version", version, "tunnel_port", cfg.TunnelPort, "http_port", cfg.HTTPPort)

	certPEM, keyPEM, err := loadOrGenerateCert(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	tlsCfg, err := tlsutil.ServerConfig(certPEM, keyPEM)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	pool := &Pool{}
	var reqIDGen uint32
	timeout := time.Duration(cfg.TimeoutSec) * time.Second

	go func() {
		if err := StartTunnel(":"+cfg.TunnelPort, tlsCfg, pool, logger, cfg.Password); err != nil {
			logger.Error("tunnel stopped", "err", err)
			os.Exit(1)
		}
	}()

	if err := StartProxy(":"+cfg.HTTPPort, pool, timeout, &reqIDGen, logger); err != nil {
		logger.Error("proxy stopped", "err", err)
		os.Exit(1)
	}
}

func loadOrGenerateCert(certFile, keyFile string) ([]byte, []byte, error) {
	if certFile == "" && keyFile == "" {
		return tlsutil.GenerateSelfSigned()
	}
	if certFile == "" || keyFile == "" {
		return nil, nil, fmt.Errorf("--cert and --key must be set together")
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read cert: %w", err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("read key: %w", err)
	}
	return certPEM, keyPEM, nil
}

func newLogger(level string) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
